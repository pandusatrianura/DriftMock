package diff_test

import (
	"bytes"
	"testing"

	"github.com/fatih/color"
	"github.com/pandusatrianura/driftmock/internal/diff"
	"github.com/pandusatrianura/driftmock/internal/model"
	"github.com/stretchr/testify/assert"
)

func init() {
	// Keep color codes deterministic for testing
	color.NoColor = true
}

func TestFormatter_FormatReport_Breaking(t *testing.T) {
	formatter := diff.NewFormatter()

	report := &model.DiffReport{
		Method: "GET",
		Path:   "/api/v1/profile",
		StubID: "GET_api_v1_profile",
		Items: []model.DiffItem{
			{
				Field:    "data.user.email",
				Severity: model.SeverityBreaking,
				Message:  "field 'data.user.email' is missing",
			},
			{
				Field:    "data.user.id",
				Severity: model.SeverityBreaking,
				Message:  "type mismatch: expected number, received string",
			},
			{
				Field:    "data.user.avatar_url",
				Severity: model.SeverityAddition,
				Message:  "new field 'data.user.avatar_url' added (string)",
			},
		},
	}

	buf := bytes.NewBuffer(nil)
	formatter.FormatReport(buf, report)
	output := buf.String()

	assert.Contains(t, output, "CONTRACT DRIFT DETECTED: GET /api/v1/profile")
	assert.Contains(t, output, "- data.user.email: field 'data.user.email' is missing")
	assert.Contains(t, output, "~ data.user.id: type mismatch: expected number, received string")
	assert.Contains(t, output, "+ data.user.avatar_url: new field 'data.user.avatar_url' added")
	assert.Contains(t, output, "2 breaking changes found compared to stub 'GET_api_v1_profile'")
}

func TestFormatter_FormatReport_Clean(t *testing.T) {
	formatter := diff.NewFormatter()

	report := &model.DiffReport{
		Method: "GET",
		Path:   "/api/v1/users",
		StubID: "GET_api_v1_users",
		Items:  nil,
	}

	buf := bytes.NewBuffer(nil)
	formatter.FormatReport(buf, report)
	output := buf.String()

	assert.Contains(t, output, "CONTRACT VERIFIED: GET /api/v1/users")
	assert.Contains(t, output, "No contract changes detected. 100% compatible.")
}

func TestFormatter_FormatReport_NonBreakingAdditions(t *testing.T) {
	formatter := diff.NewFormatter()

	report := &model.DiffReport{
		Method: "POST",
		Path:   "/api/v1/checkout",
		StubID: "POST_api_v1_checkout",
		Items: []model.DiffItem{
			{
				Field:    "order.tracking_number",
				Severity: model.SeverityAddition,
				Message:  "new field added",
			},
		},
	}

	buf := bytes.NewBuffer(nil)
	formatter.FormatReport(buf, report)
	output := buf.String()

	assert.Contains(t, output, "CONTRACT MODIFIED (NON-BREAKING): POST /api/v1/checkout")
	assert.Contains(t, output, "+ order.tracking_number: new field added")
	assert.Contains(t, output, "Backward-compatible (1 additions, 0 warnings)")
}

func TestFormatter_FormatSummary(t *testing.T) {
	formatter := diff.NewFormatter()

	reports := []*model.DiffReport{
		{
			Method: "GET",
			Path:   "/api/v1/users",
			StubID: "GET_users",
			Items:  nil,
		},
		{
			Method: "GET",
			Path:   "/api/v1/profile",
			StubID: "GET_profile",
			Items: []model.DiffItem{
				{
					Field:    "email",
					Severity: model.SeverityBreaking,
					Message:  "missing",
				},
				{
					Field:    "status",
					Severity: model.SeverityWarning,
					Message:  "enum changed",
				},
				{
					Field:    "avatar",
					Severity: model.SeverityAddition,
					Message:  "new field",
				},
			},
		},
	}

	buf := bytes.NewBuffer(nil)
	totals := formatter.FormatSummary(buf, reports)
	output := buf.String()

	assert.Equal(t, 2, totals.TotalChecked)
	assert.Equal(t, 1, totals.PassedCount)
	assert.Equal(t, 1, totals.FailedCount)
	assert.Equal(t, 1, totals.TotalBreaking)
	assert.Equal(t, 1, totals.TotalWarnings)
	assert.Equal(t, 1, totals.TotalAdditions)

	assert.Contains(t, output, "Contract Drift Summary")
	assert.Contains(t, output, "Endpoints Checked: 2")
	assert.Contains(t, output, "Passed:            1")
	assert.Contains(t, output, "Failed (Breaking): 1")
	assert.Contains(t, output, "1 Breaking changes")
	assert.Contains(t, output, "FAILED (breaking contract drift detected)")
}
