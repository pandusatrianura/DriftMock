package diff_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pandusatrianura/driftmock/internal/diff"
	"github.com/pandusatrianura/driftmock/internal/model"
	"github.com/pandusatrianura/driftmock/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunner_Run(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   1,
			"name": "Jane",
		})
	}))
	defer server.Close()

	tempDir := t.TempDir()
	store, err := storage.NewFileStore(tempDir)
	require.NoError(t, err)

	stub := &model.Stub{
		ID: "GET_user",
		Request: model.RequestPattern{
			Method: "GET",
			Path:   "/api/v1/user",
		},
		Response: model.ResponsePattern{
			StatusCode: 200,
			Body: map[string]any{
				"id":   1,
				"name": "Jane",
			},
		},
	}
	_, err = store.Save(stub)
	require.NoError(t, err)

	runner := diff.NewRunner(store, diff.RunnerOptions{
		TargetBaseURL: server.URL,
		Timeout:       2 * time.Second,
		DiffOptions:   diff.DefaultDiffOptions(),
	})

	reports, err := runner.Run(context.Background())
	require.NoError(t, err)
	require.Len(t, reports, 1)

	assert.False(t, reports[0].HasBreaking())
	assert.Equal(t, "GET_user", reports[0].StubID)

	// Test CompareResponse convenience method
	directReport, err := runner.CompareResponse(stub, 200, []byte(`{"id": 1, "name": "Jane"}`))
	require.NoError(t, err)
	assert.False(t, directReport.HasBreaking())
}

func TestRunner_TargetUnreachable(t *testing.T) {
	tempDir := t.TempDir()
	store, _ := storage.NewFileStore(tempDir)

	stub := &model.Stub{
		ID: "GET_dead",
		Request: model.RequestPattern{
			Method: "GET",
			Path:   "/dead",
		},
	}
	_, _ = store.Save(stub)

	runner := diff.NewRunner(store, diff.RunnerOptions{
		TargetBaseURL: "http://127.0.0.1:59998",
		Timeout:       500 * time.Millisecond,
	})

	reports, err := runner.Run(context.Background())
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.True(t, reports[0].HasBreaking())
	assert.Equal(t, "network", reports[0].Items[0].Field)
}
