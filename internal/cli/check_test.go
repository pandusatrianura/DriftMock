package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pandusatrianura/driftmock/internal/cli"
	"github.com/pandusatrianura/driftmock/internal/logger"
	"github.com/pandusatrianura/driftmock/internal/model"
	"github.com/pandusatrianura/driftmock/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateCheckOptions(t *testing.T) {
	tests := []struct {
		name        string
		opts        cli.CheckOptions
		expectError bool
		errorMsg    string
	}{
		{
			name: "missing target",
			opts: cli.CheckOptions{
				Target: "",
			},
			expectError: true,
			errorMsg:    "target URL is required",
		},
		{
			name: "invalid scheme ftp",
			opts: cli.CheckOptions{
				Target: "ftp://api.staging.example.com",
			},
			expectError: true,
			errorMsg:    "invalid target scheme",
		},
		{
			name: "valid options",
			opts: cli.CheckOptions{
				Target:   "https://api.staging.example.com",
				StubsDir: "./.custom",
				Strict:   true,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := cli.ValidateCheckOptions(&tt.opts)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, parsed)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, parsed)
				assert.Equal(t, "https", parsed.Scheme)
			}
		})
	}
}

func TestRunCheck_CompatibleAndAdditions(t *testing.T) {
	// Upstream staging mock server returning compatible response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/profile", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         1,
			"name":       "Pandu",
			"email":      "pandu@example.com",
			"badge":      "gold", // new non-breaking addition
			"updated_at": "2026-11-18T12:00:00Z",
		})
	}))
	defer server.Close()

	tempDir := t.TempDir()
	store, err := storage.NewFileStore(tempDir)
	require.NoError(t, err)

	// Save baseline stub
	stub := &model.Stub{
		ID: "GET_api_v1_profile",
		Request: model.RequestPattern{
			Method:         "GET",
			Path:           "/api/v1/profile",
			NormalizedPath: "/api/v1/profile",
		},
		Response: model.ResponsePattern{
			StatusCode: 200,
			Body: map[string]any{
				"id":         1,
				"name":       "Pandu",
				"email":      "pandu@example.com",
				"updated_at": "2026-01-01T00:00:00Z",
			},
		},
	}
	_, err = store.Save(stub)
	require.NoError(t, err)

	opts := cli.CheckOptions{
		Target:         server.URL,
		StubsDir:       tempDir,
		Strict:         true,
		IgnoreVolatile: true,
	}

	buf := bytes.NewBuffer(nil)
	hasBreaking, err := cli.RunCheck(context.Background(), opts, buf)
	require.NoError(t, err)
	assert.False(t, hasBreaking, "compatible addition must not be flagged as breaking")

	output := buf.String()
	assert.Contains(t, output, "CONTRACT MODIFIED (NON-BREAKING)")
	assert.Contains(t, output, "+ badge: new field 'badge' added")
	assert.Contains(t, output, "PASSED (all endpoints backward-compatible)")
}

func TestRunCheck_BreakingChangesDetected(t *testing.T) {
	// Upstream staging mock server returning BREAKING change
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   "string_id_101", // type changed from number to string!
			"name": "Pandu",
			// "email" field was removed!
		})
	}))
	defer server.Close()

	tempDir := t.TempDir()
	store, err := storage.NewFileStore(tempDir)
	require.NoError(t, err)

	stub := &model.Stub{
		ID: "GET_api_v1_profile",
		Request: model.RequestPattern{
			Method:         "GET",
			Path:           "/api/v1/profile",
			NormalizedPath: "/api/v1/profile",
		},
		Response: model.ResponsePattern{
			StatusCode: 200,
			Body: map[string]any{
				"id":    101,
				"name":  "Pandu",
				"email": "pandu@example.com",
			},
		},
	}
	_, err = store.Save(stub)
	require.NoError(t, err)

	opts := cli.CheckOptions{
		Target:   server.URL,
		StubsDir: tempDir,
		Strict:   true,
	}

	buf := bytes.NewBuffer(nil)
	hasBreaking, err := cli.RunCheck(context.Background(), opts, buf)
	require.NoError(t, err)
	assert.True(t, hasBreaking, "breaking changes must be detected")

	output := buf.String()
	assert.Contains(t, output, "CONTRACT DRIFT DETECTED")
	assert.Contains(t, output, "email: field 'email' is missing")
	assert.Contains(t, output, "id: type mismatch: expected number, received string")
	assert.Contains(t, output, "FAILED (breaking contract drift detected)")
}

func TestRunCheck_EmptyStubs(t *testing.T) {
	tempDir := t.TempDir()
	opts := cli.CheckOptions{
		Target:   "http://localhost:8080",
		StubsDir: tempDir,
	}

	buf := bytes.NewBuffer(nil)
	hasBreaking, err := cli.RunCheck(context.Background(), opts, buf)
	require.NoError(t, err)
	assert.False(t, hasBreaking)
	assert.Contains(t, buf.String(), "No stubs found")
}

func TestRunCheck_JSONFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   101,
			"name": "Pandu",
		})
	}))
	defer server.Close()

	tempDir := t.TempDir()
	store, err := storage.NewFileStore(tempDir)
	require.NoError(t, err)

	stub := &model.Stub{
		ID: "GET_api_v1_profile",
		Request: model.RequestPattern{
			Method:         "GET",
			Path:           "/api/v1/profile",
			NormalizedPath: "/api/v1/profile",
		},
		Response: model.ResponsePattern{
			StatusCode: 200,
			Body: map[string]any{
				"id":   101,
				"name": "Pandu",
			},
		},
	}
	_, err = store.Save(stub)
	require.NoError(t, err)

	logBuf := &bytes.Buffer{}
	logger.Init("json", false, logBuf)
	defer logger.SetJSONForTesting(false)

	opts := cli.CheckOptions{
		Target:   server.URL,
		StubsDir: tempDir,
		Strict:   true,
	}

	buf := bytes.NewBuffer(nil)
	hasBreaking, err := cli.RunCheck(context.Background(), opts, buf)
	require.NoError(t, err)
	assert.False(t, hasBreaking)

	logOutput := logBuf.String()
	assert.Contains(t, logOutput, "contract drift check result")
	assert.Contains(t, logOutput, "contract drift check summary")
}
