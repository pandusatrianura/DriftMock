package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/pandusatrianura/driftmock/internal/cli"
	"github.com/pandusatrianura/driftmock/internal/model"
	"github.com/pandusatrianura/driftmock/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateReplayOptions(t *testing.T) {
	tests := []struct {
		name        string
		opts        cli.ReplayCommandOptions
		expectError bool
		errorMsg    string
	}{
		{
			name: "invalid port 0",
			opts: cli.ReplayCommandOptions{
				Port: 0,
			},
			expectError: true,
			errorMsg:    "invalid port",
		},
		{
			name: "invalid port 70000",
			opts: cli.ReplayCommandOptions{
				Port: 70000,
			},
			expectError: true,
			errorMsg:    "invalid port",
		},
		{
			name: "negative latency",
			opts: cli.ReplayCommandOptions{
				Port:      8080,
				LatencyMs: -50,
			},
			expectError: true,
			errorMsg:    "latency cannot be negative",
		},
		{
			name: "valid options",
			opts: cli.ReplayCommandOptions{
				Port:      8080,
				LatencyMs: 100,
				StubsDir:  "./.custom",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cli.ValidateReplayOptions(&tt.opts)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func seedReplayStubs(t *testing.T, store storage.Store) {
	stubPing := &model.Stub{
		ID: "GET_api_v1_ping",
		Request: model.RequestPattern{
			Method:         "GET",
			Path:           "/api/v1/ping",
			NormalizedPath: "/api/v1/ping",
		},
		Response: model.ResponsePattern{
			StatusCode: 200,
			Headers: map[string][]string{
				"Content-Type": {"application/json"},
			},
			Body: map[string]any{
				"status": "pong",
			},
			LatencyMs: 25,
		},
	}
	_, err := store.Save(stubPing)
	require.NoError(t, err)

	stubUser := &model.Stub{
		ID: "GET_api_v1_users_42",
		Request: model.RequestPattern{
			Method:         "GET",
			Path:           "/api/v1/users/42",
			NormalizedPath: "/api/v1/users/:id",
		},
		Response: model.ResponsePattern{
			StatusCode: 200,
			Headers: map[string][]string{
				"Content-Type": {"application/json"},
			},
			Body: map[string]any{
				"id":   42,
				"name": "Jane",
			},
			LatencyMs: 15,
		},
	}
	_, err = store.Save(stubUser)
	require.NoError(t, err)
}

func TestRunReplay_EndToEnd(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewFileStore(tempDir)
	require.NoError(t, err)
	seedReplayStubs(t, store)

	freePort := findFreePort(t)
	opts := cli.ReplayCommandOptions{
		StubsDir:      tempDir,
		Port:          freePort,
		SimulateDelay: false,
		LatencyMs:     0,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	outputBuf := bytes.NewBuffer(nil)
	errChan := make(chan error, 1)

	go func() {
		errChan <- cli.RunReplay(ctx, opts, outputBuf)
	}()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", freePort)
	client := &http.Client{Timeout: 2 * time.Second}

	// 1. Wait for mock server to be ready
	var resp *http.Response
	var fetchErr error
	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		resp, fetchErr = client.Get(baseURL + "/api/v1/ping")
		if fetchErr == nil {
			break
		}
	}
	require.NoError(t, fetchErr, "replay server should be reachable")
	defer resp.Body.Close()

	// Verify exact match
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "true", resp.Header.Get("X-DriftMock-Replay"))
	assert.Equal(t, "EXACT", resp.Header.Get("X-DriftMock-Tier"))

	var pingBody map[string]any
	err = json.NewDecoder(resp.Body).Decode(&pingBody)
	require.NoError(t, err)
	assert.Equal(t, "pong", pingBody["status"])

	// 2. Verify dynamic pattern fallback match (/users/99 matches /users/:id)
	userResp, err := client.Get(baseURL + "/api/v1/users/99")
	require.NoError(t, err)
	defer userResp.Body.Close()

	assert.Equal(t, http.StatusOK, userResp.StatusCode)
	assert.Equal(t, "PATTERN", userResp.Header.Get("X-DriftMock-Tier"))

	var userBody map[string]any
	err = json.NewDecoder(userResp.Body).Decode(&userBody)
	require.NoError(t, err)
	assert.Equal(t, "Jane", userBody["name"])

	// 3. Verify 404 on unrecorded route
	notFoundResp, err := client.Get(baseURL + "/api/v1/nonexistent")
	require.NoError(t, err)
	defer notFoundResp.Body.Close()

	assert.Equal(t, http.StatusNotFound, notFoundResp.StatusCode)
	var diagBody map[string]any
	err = json.NewDecoder(notFoundResp.Body).Decode(&diagBody)
	require.NoError(t, err)
	assert.Equal(t, "Not Found", diagBody["error"])

	// 4. Verify CORS preflight
	corsReq, err := http.NewRequest(http.MethodOptions, baseURL+"/api/v1/ping", nil)
	require.NoError(t, err)
	corsReq.Header.Set("Origin", "http://localhost:5173")
	corsResp, err := client.Do(corsReq)
	require.NoError(t, err)
	defer corsResp.Body.Close()

	assert.Equal(t, http.StatusNoContent, corsResp.StatusCode)
	assert.Equal(t, "http://localhost:5173", corsResp.Header.Get("Access-Control-Allow-Origin"))

	// 5. Trigger graceful shutdown
	cancel()
	select {
	case err := <-errChan:
		require.NoError(t, err, "RunReplay should shut down cleanly without error")
	case <-time.After(3 * time.Second):
		t.Fatal("RunReplay timed out waiting for shutdown")
	}

	assert.Contains(t, outputBuf.String(), "DriftMock Offline Replay")
	assert.Contains(t, outputBuf.String(), "GET:/api/v1/ping")
	assert.Contains(t, outputBuf.String(), "HIT")
}
