package diff

import (
	"fmt"
	"strings"

	"github.com/pandusatrianura/driftmock/internal/model"
)

// DefaultVolatileKeys lists keys that frequently change across requests and should be ignored by default.
var DefaultVolatileKeys = map[string]bool{
	"updated_at":    true,
	"created_at":    true,
	"timestamp":     true,
	"ts":            true,
	"_ts":           true,
	"last_modified": true,
	"datetime":      true,
	"current_time":  true,
}

// enumLikeKeys lists key names whose scalar value shifts represent potential contract enum drift.
var enumLikeKeys = map[string]bool{
	"status": true,
	"state":  true,
	"type":   true,
	"role":   true,
	"mode":   true,
	"gender": true,
	"tier":   true,
	"phase":  true,
}

// DescribeType returns a canonical JSON type name for any Go value.
func DescribeType(v any) string {
	if v == nil {
		return "null"
	}

	switch val := v.(type) {
	case bool:
		return "boolean"
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "number"
	case string:
		return "string"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	default:
		return fmt.Sprintf("%T", val)
	}
}

// IsVolatileKey checks if a JSON field name corresponds to volatile runtime timestamps.
func IsVolatileKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	return DefaultVolatileKeys[lower]
}

// IsEnumLikeKey checks if a field name commonly represents an enum or categorical state.
func IsEnumLikeKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	return enumLikeKeys[lower]
}

// BuildFieldPath constructs a dotted path representation (e.g., data.user.email).
func BuildFieldPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

// NewBreakingDiff creates a DiffItem categorized as a breaking change.
func NewBreakingDiff(field, message string, expected, received any) model.DiffItem {
	return model.DiffItem{
		Field:    field,
		Severity: model.SeverityBreaking,
		Message:  message,
		Expected: expected,
		Received: received,
	}
}

// NewWarningDiff creates a DiffItem categorized as a warning or potential drift.
func NewWarningDiff(field, message string, expected, received any) model.DiffItem {
	return model.DiffItem{
		Field:    field,
		Severity: model.SeverityWarning,
		Message:  message,
		Expected: expected,
		Received: received,
	}
}

// NewAdditionDiff creates a DiffItem categorized as a backward-compatible addition.
func NewAdditionDiff(field, message string, received any) model.DiffItem {
	return model.DiffItem{
		Field:    field,
		Severity: model.SeverityAddition,
		Message:  message,
		Expected: nil,
		Received: received,
	}
}
