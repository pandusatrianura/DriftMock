package diff_test

import (
	"testing"

	"github.com/pandusatrianura/driftmock/internal/diff"
	"github.com/pandusatrianura/driftmock/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffer_KeyReorderingNoDiff(t *testing.T) {
	differ := diff.NewDiffer(diff.DefaultDiffOptions())

	// Baseline JSON
	expectedJSON := []byte(`{"name": "Pandu", "age": 28, "active": true}`)
	// Live response with keys in different order and different whitespace
	receivedJSON := []byte(`{
		"active": true,
		"name": "Pandu",
		"age": 28
	}`)

	items, err := differ.DiffBytes(expectedJSON, receivedJSON)
	require.NoError(t, err)
	assert.Empty(t, items, "reordered JSON keys must yield zero diff items (no false positives)")
}

func TestDiffer_MissingFieldIsBreaking(t *testing.T) {
	differ := diff.NewDiffer(diff.DefaultDiffOptions())

	expected := []byte(`{
		"data": {
			"user": {
				"id": 1,
				"name": "Pandu",
				"email": "pandu@example.com"
			}
		}
	}`)

	// "email" field is removed
	received := []byte(`{
		"data": {
			"user": {
				"id": 1,
				"name": "Pandu"
			}
		}
	}`)

	items, err := differ.DiffBytes(expected, received)
	require.NoError(t, err)
	require.Len(t, items, 1)

	assert.Equal(t, "data.user.email", items[0].Field)
	assert.Equal(t, model.SeverityBreaking, items[0].Severity)
	assert.Contains(t, items[0].Message, "missing")
}

func TestDiffer_TypeMismatchIsBreaking(t *testing.T) {
	differ := diff.NewDiffer(diff.DefaultDiffOptions())

	// Expected id as number, received as string
	expected := []byte(`{"id": 101, "tags": ["a", "b"]}`)
	received := []byte(`{"id": "101", "tags": {"tag1": "a"}}`)

	items, err := differ.DiffBytes(expected, received)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "id", items[0].Field)
	assert.Equal(t, model.SeverityBreaking, items[0].Severity)
	assert.Contains(t, items[0].Message, "expected number, received string")

	assert.Equal(t, "tags", items[1].Field)
	assert.Equal(t, model.SeverityBreaking, items[1].Severity)
	assert.Contains(t, items[1].Message, "expected array, received object")
}

func TestDiffer_NullabilityShift(t *testing.T) {
	differ := diff.NewDiffer(diff.DefaultDiffOptions())

	expected := []byte(`{"profile": {"bio": "developer"}}`)
	received := []byte(`{"profile": {"bio": null}}`)

	items, err := differ.DiffBytes(expected, received)
	require.NoError(t, err)
	require.Len(t, items, 1)

	assert.Equal(t, "profile.bio", items[0].Field)
	assert.Equal(t, model.SeverityBreaking, items[0].Severity)
	assert.Contains(t, items[0].Message, "expected string, received null")
}

func TestDiffer_NewFieldIsAddition(t *testing.T) {
	differ := diff.NewDiffer(diff.DefaultDiffOptions())

	expected := []byte(`{"id": 1, "name": "Jane"}`)
	received := []byte(`{"id": 1, "name": "Jane", "avatar_url": "https://img.com/1.png"}`)

	items, err := differ.DiffBytes(expected, received)
	require.NoError(t, err)
	require.Len(t, items, 1)

	assert.Equal(t, "avatar_url", items[0].Field)
	assert.Equal(t, model.SeverityAddition, items[0].Severity)
	assert.Contains(t, items[0].Message, "new field 'avatar_url' added")
}

func TestDiffer_VolatileKeysIgnored(t *testing.T) {
	differ := diff.NewDiffer(diff.DefaultDiffOptions())

	expected := []byte(`{
		"id": 1,
		"updated_at": "2026-01-01T00:00:00Z",
		"timestamp": 123456789
	}`)

	// Timestamp values completely changed, but should be ignored
	received := []byte(`{
		"id": 1,
		"updated_at": "2026-11-18T12:00:00Z",
		"timestamp": 987654321
	}`)

	items, err := differ.DiffBytes(expected, received)
	require.NoError(t, err)
	assert.Empty(t, items, "volatile keys must be ignored when IgnoreVolatileKeys is true")
}

func TestDiffer_EnumStatusChangeIsWarning(t *testing.T) {
	differ := diff.NewDiffer(diff.DefaultDiffOptions())

	expected := []byte(`{"order_id": 99, "status": "PENDING"}`)
	received := []byte(`{"order_id": 99, "status": "COMPLETED"}`)

	items, err := differ.DiffBytes(expected, received)
	require.NoError(t, err)
	require.Len(t, items, 1)

	assert.Equal(t, "status", items[0].Field)
	assert.Equal(t, model.SeverityWarning, items[0].Severity)
	assert.Contains(t, items[0].Message, "enum/status value changed")
}

func TestDiffer_ArrayItemSchemaDiff(t *testing.T) {
	differ := diff.NewDiffer(diff.DefaultDiffOptions())

	expected := []byte(`{
		"items": [
			{"id": 1, "sku": "ABC", "price": 50.0}
		]
	}`)

	// In live response, "sku" is missing from array item
	received := []byte(`{
		"items": [
			{"id": 2, "price": 75.0}
		]
	}`)

	items, err := differ.DiffBytes(expected, received)
	require.NoError(t, err)
	require.Len(t, items, 1)

	assert.Equal(t, "items[0].sku", items[0].Field)
	assert.Equal(t, model.SeverityBreaking, items[0].Severity)
}

func TestDiffer_CompareStub(t *testing.T) {
	differ := diff.NewDiffer(diff.DefaultDiffOptions())

	stub := &model.Stub{
		ID: "GET_api_v1_profile",
		Request: model.RequestPattern{
			Method: "GET",
			Path:   "/api/v1/profile",
		},
		Response: model.ResponsePattern{
			StatusCode: 200,
			Body: map[string]any{
				"id":    1,
				"name":  "Alice",
				"email": "alice@example.com",
			},
		},
	}

	// 1. Comparison with breaking changes and status code mismatch (500 instead of 200)
	liveBody := []byte(`{"id": 1, "name": "Alice"}`)
	report, err := differ.CompareStub(stub, 500, liveBody)
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.True(t, report.HasBreaking())
	assert.Equal(t, 2, report.BreakingCount()) // status_code + email missing
	assert.Equal(t, 0, report.WarningCount())
	assert.Equal(t, 0, report.AdditionCount())

	// 2. Comparison with backward-compatible response
	cleanLiveBody := []byte(`{"id": 1, "name": "Alice", "email": "alice@example.com", "badge": "gold"}`)
	cleanReport, err := differ.CompareStub(stub, 200, cleanLiveBody)
	require.NoError(t, err)
	assert.False(t, cleanReport.HasBreaking())
	assert.Equal(t, 1, cleanReport.AdditionCount())
}
