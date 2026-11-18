package diff

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/pandusatrianura/driftmock/internal/model"
)

// DiffOptions configures semantic diffing behavior.
type DiffOptions struct {
	IgnoreVolatileKeys bool            // ignores dynamic timestamps like updated_at, created_at
	CustomIgnoredKeys  map[string]bool // user-specified field names to skip
	CheckEnumDrift     bool            // flags categorical value shifts as WARNING
}

// DefaultDiffOptions returns standard configuration for contract drift detection.
func DefaultDiffOptions() DiffOptions {
	return DiffOptions{
		IgnoreVolatileKeys: true,
		CustomIgnoredKeys:  nil,
		CheckEnumDrift:     true,
	}
}

// Differ performs semantic tree diffs on JSON payloads.
type Differ struct {
	options DiffOptions
}

// NewDiffer creates an initialized Differ.
func NewDiffer(opts DiffOptions) *Differ {
	return &Differ{options: opts}
}

// Diff compares an expected baseline object with a received object recursively.
func (d *Differ) Diff(expected, received any) []model.DiffItem {
	var items []model.DiffItem
	d.diffRecursive("", expected, received, &items)
	return items
}

// DiffBytes parses raw JSON bytes and returns semantic diff items.
func (d *Differ) DiffBytes(expectedBytes, receivedBytes []byte) ([]model.DiffItem, error) {
	if len(expectedBytes) == 0 && len(receivedBytes) == 0 {
		return nil, nil
	}

	var expRoot any
	if len(expectedBytes) > 0 {
		if err := json.Unmarshal(expectedBytes, &expRoot); err != nil {
			return nil, fmt.Errorf("failed to parse expected JSON: %w", err)
		}
	}

	var recRoot any
	if len(receivedBytes) > 0 {
		if err := json.Unmarshal(receivedBytes, &recRoot); err != nil {
			return nil, fmt.Errorf("failed to parse received JSON: %w", err)
		}
	}

	return d.Diff(expRoot, recRoot), nil
}

// CompareStub evaluates a live HTTP response against a recorded Stub baseline.
func (d *Differ) CompareStub(stub *model.Stub, liveStatusCode int, liveBodyBytes []byte) (*model.DiffReport, error) {
	if stub == nil {
		return nil, fmt.Errorf("stub cannot be nil")
	}

	report := &model.DiffReport{
		Method: stub.Request.Method,
		Path:   stub.Request.Path,
		StubID: stub.ID,
		Items:  make([]model.DiffItem, 0),
	}

	// 1. Status Code Comparison
	if stub.Response.StatusCode != 0 && liveStatusCode != 0 && stub.Response.StatusCode != liveStatusCode {
		report.Items = append(report.Items, NewBreakingDiff(
			"status_code",
			fmt.Sprintf("status code mismatch: expected %d, received %d", stub.Response.StatusCode, liveStatusCode),
			stub.Response.StatusCode,
			liveStatusCode,
		))
	}

	// 2. Response Body Comparison
	if len(liveBodyBytes) == 0 && stub.Response.Body == nil {
		return report, nil
	}

	var liveBody any
	if len(liveBodyBytes) > 0 {
		if err := json.Unmarshal(liveBodyBytes, &liveBody); err != nil {
			// Non-JSON response received
			liveBody = string(liveBodyBytes)
		}
	}

	bodyDiffs := d.Diff(stub.Response.Body, liveBody)
	report.Items = append(report.Items, bodyDiffs...)

	return report, nil
}

func (d *Differ) shouldIgnore(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	if d.options.CustomIgnoredKeys != nil && d.options.CustomIgnoredKeys[lower] {
		return true
	}
	if d.options.IgnoreVolatileKeys && IsVolatileKey(lower) {
		return true
	}
	return false
}

func (d *Differ) diffRecursive(currentPath string, expected, received any, items *[]model.DiffItem) {
	// 1. Both nil: perfectly equal
	if expected == nil && received == nil {
		return
	}

	// 2. Expected value, received nil (Nullability shift / missing non-null)
	if expected != nil && received == nil {
		*items = append(*items, NewBreakingDiff(
			currentPath,
			fmt.Sprintf("expected %s, received null", DescribeType(expected)),
			expected,
			nil,
		))
		return
	}

	// 3. Expected nil, received value (Null field now populated: non-breaking addition)
	if expected == nil && received != nil {
		*items = append(*items, NewAdditionDiff(
			currentPath,
			fmt.Sprintf("field populated (expected null, received %s)", DescribeType(received)),
			received,
		))
		return
	}

	expType := DescribeType(expected)
	recType := DescribeType(received)

	// 4. Type mismatch: BREAKING
	if expType != recType {
		*items = append(*items, NewBreakingDiff(
			currentPath,
			fmt.Sprintf("type mismatch: expected %s, received %s", expType, recType),
			expType,
			recType,
		))
		return
	}

	// 5. Object (map[string]any) comparison
	if expType == "object" {
		expMap := expected.(map[string]any)
		recMap := received.(map[string]any)

		// Collect and sort expected keys for deterministic comparison order
		expKeys := make([]string, 0, len(expMap))
		for k := range expMap {
			expKeys = append(expKeys, k)
		}
		sort.Strings(expKeys)

		// Check for missing fields in received
		for _, k := range expKeys {
			if d.shouldIgnore(k) {
				continue
			}

			expVal := expMap[k]
			childPath := BuildFieldPath(currentPath, k)
			recVal, exists := recMap[k]
			if !exists {
				*items = append(*items, NewBreakingDiff(
					childPath,
					fmt.Sprintf("field '%s' is missing (expected %s)", childPath, DescribeType(expVal)),
					DescribeType(expVal),
					"missing",
				))
			} else {
				d.diffRecursive(childPath, expVal, recVal, items)
			}
		}

		// Collect and sort received keys to deterministically find newly added fields
		recKeys := make([]string, 0, len(recMap))
		for k := range recMap {
			recKeys = append(recKeys, k)
		}
		sort.Strings(recKeys)

		// Check for newly added fields in received
		for _, k := range recKeys {
			if d.shouldIgnore(k) {
				continue
			}

			if _, exists := expMap[k]; !exists {
				recVal := recMap[k]
				childPath := BuildFieldPath(currentPath, k)
				*items = append(*items, NewAdditionDiff(
					childPath,
					fmt.Sprintf("new field '%s' added (%s)", childPath, DescribeType(recVal)),
					DescribeType(recVal),
				))
			}
		}
		return
	}

	// 6. Array ([]any) comparison
	if expType == "array" {
		expArr := expected.([]any)
		recArr := received.([]any)

		if len(expArr) == 0 && len(recArr) == 0 {
			return
		}

		if len(expArr) == 0 && len(recArr) > 0 {
			// Array was empty in baseline, now has items
			*items = append(*items, NewAdditionDiff(
				currentPath,
				fmt.Sprintf("array '%s' populated with items", currentPath),
				DescribeType(recArr[0]),
			))
			return
		}

		if len(expArr) > 0 && len(recArr) == 0 {
			// Array had elements in baseline, now empty
			*items = append(*items, NewWarningDiff(
				currentPath,
				fmt.Sprintf("array '%s' is empty in live response (schema cannot be verified)", currentPath),
				len(expArr),
				0,
			))
			return
		}

		// Compare element schema using the first item
		childPath := currentPath + "[0]"
		if currentPath == "" {
			childPath = "[0]"
		}
		d.diffRecursive(childPath, expArr[0], recArr[0], items)
		return
	}

	// 7. Scalar Value Checks (Enum / Status Drift)
	if d.options.CheckEnumDrift && currentPath != "" {
		lastDot := strings.LastIndex(currentPath, ".")
		keyName := currentPath
		if lastDot != -1 {
			keyName = currentPath[lastDot+1:]
		}

		if IsEnumLikeKey(keyName) && expected != received {
			*items = append(*items, NewWarningDiff(
				currentPath,
				fmt.Sprintf("enum/status value changed: expected '%v', received '%v'", expected, received),
				expected,
				received,
			))
		}
	}
}
