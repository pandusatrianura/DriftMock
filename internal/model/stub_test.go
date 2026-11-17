package model_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pandusatrianura/driftmock/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStubJSONSerialization(t *testing.T) {
	recordedTime := time.Date(2026, 11, 17, 18, 0, 0, 0, time.UTC)

	stub := model.Stub{
		Version:    "1.0",
		ID:         "get_api_v1_users",
		RecordedAt: recordedTime,
		Request: model.RequestPattern{
			Method:         "GET",
			Path:           "/api/v1/users/42",
			NormalizedPath: "/api/v1/users/:id",
			Query: map[string][]string{
				"sort":  {"asc"},
				"limit": {"10"},
			},
			Headers: map[string][]string{
				"Accept": {"application/json"},
			},
			BodyHash: "",
		},
		Response: model.ResponsePattern{
			StatusCode: 200,
			Headers: map[string][]string{
				"Content-Type": {"application/json"},
			},
			Body: map[string]any{
				"data": map[string]any{
					"id":    float64(42),
					"name":  "Jane Doe",
					"email": "jane@example.com",
				},
			},
			LatencyMs: 85,
		},
	}

	data, err := json.MarshalIndent(stub, "", "  ")
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	var unmarshaled model.Stub
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, stub.Version, unmarshaled.Version)
	assert.Equal(t, stub.ID, unmarshaled.ID)
	assert.True(t, stub.RecordedAt.Equal(unmarshaled.RecordedAt))
	assert.Equal(t, stub.Request.Method, unmarshaled.Request.Method)
	assert.Equal(t, stub.Request.Path, unmarshaled.Request.Path)
	assert.Equal(t, stub.Request.NormalizedPath, unmarshaled.Request.NormalizedPath)
	assert.Equal(t, stub.Response.StatusCode, unmarshaled.Response.StatusCode)
	assert.Equal(t, int64(85), unmarshaled.Response.LatencyMs)
}

func TestDiffReportCounts(t *testing.T) {
	report := model.DiffReport{
		Method: "GET",
		Path:   "/api/v1/users/42",
		StubID: "get_api_v1_users",
		Items: []model.DiffItem{
			{
				Field:    "data.email",
				Severity: model.SeverityBreaking,
				Message:  "field is missing",
			},
			{
				Field:    "data.id",
				Severity: model.SeverityBreaking,
				Message:  "type mismatch: expected number, got string",
			},
			{
				Field:    "data.status",
				Severity: model.SeverityWarning,
				Message:  "enum value changed",
			},
			{
				Field:    "data.avatar",
				Severity: model.SeverityAddition,
				Message:  "new field added",
			},
		},
	}

	assert.True(t, report.HasBreaking())
	assert.Equal(t, 2, report.BreakingCount())
	assert.Equal(t, 1, report.WarningCount())
	assert.Equal(t, 1, report.AdditionCount())

	cleanReport := model.DiffReport{
		Method: "GET",
		Path:   "/api/v1/users/42",
		StubID: "get_api_v1_users",
		Items: []model.DiffItem{
			{
				Field:    "data.new_prop",
				Severity: model.SeverityAddition,
				Message:  "new non-breaking field added",
			},
		},
	}

	assert.False(t, cleanReport.HasBreaking())
	assert.Equal(t, 0, cleanReport.BreakingCount())
	assert.Equal(t, 0, cleanReport.WarningCount())
	assert.Equal(t, 1, cleanReport.AdditionCount())
}
