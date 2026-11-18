package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pandusatrianura/driftmock/internal/cli"
	"github.com/pandusatrianura/driftmock/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRecordOptions(t *testing.T) {
	tests := []struct {
		name        string
		opts        cli.RecordOptions
		expectError bool
		errorMsg    string
	}{
		{
			name: "missing target",
			opts: cli.RecordOptions{
				Target: "",
				Port:   8080,
			},
			expectError: true,
			errorMsg:    "target URL is required",
		},
		{
			name: "invalid scheme ftp",
			opts: cli.RecordOptions{
				Target: "ftp://example.com",
				Port:   8080,
			},
			expectError: true,
			errorMsg:    "invalid target scheme",
		},
		{
			name: "missing host",
			opts: cli.RecordOptions{
				Target: "http://",
				Port:   8080,
			},
			expectError: true,
			errorMsg:    "valid host",
		},
		{
			name: "invalid port 0",
			opts: cli.RecordOptions{
				Target: "https://api.staging.example.com",
				Port:   0,
			},
			expectError: true,
			errorMsg:    "invalid port",
		},
		{
			name: "valid options",
			opts: cli.RecordOptions{
				Target: "https://api.staging.example.com",
				Port:   8080,
				OutDir: "./.custom-drift",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := cli.ValidateRecordOptions(&tt.opts)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, parsed)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, parsed)
				assert.Equal(t, "https", parsed.Scheme)
				assert.Equal(t, "api.staging.example.com", parsed.Host)
			}
		})
	}
}

// findFreePort gets an available local port for testing.
func findFreePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestRunRecord_EndToEnd(t *testing.T) {
	// 1. Upstream target mock
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "hello from staging",
			"time":    time.Now().Unix(),
		})
	}))
	defer targetServer.Close()

	// 2. Setup options
	tempDir := t.TempDir()
	freePort := findFreePort(t)

	opts := cli.RecordOptions{
		Target: targetServer.URL,
		Port:   freePort,
		OutDir: tempDir,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	outputBuf := bytes.NewBuffer(nil)
	errChan := make(chan error, 1)

	// 3. Start RunRecord in background
	go func() {
		errChan <- cli.RunRecord(ctx, opts, outputBuf)
	}()

	// Wait briefly for server to bind
	proxyURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/ping", freePort)
	client := &http.Client{Timeout: 2 * time.Second}

	var resp *http.Response
	var fetchErr error
	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		resp, fetchErr = client.Get(proxyURL)
		if fetchErr == nil {
			break
		}
	}

	require.NoError(t, fetchErr, "proxy should be reachable")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var resBody map[string]any
	err := json.NewDecoder(resp.Body).Decode(&resBody)
	require.NoError(t, err)
	assert.Equal(t, "hello from staging", resBody["message"])

	// 4. Verify stub file written
	store, err := storage.NewFileStore(tempDir)
	require.NoError(t, err)
	stubs, err := store.List()
	require.NoError(t, err)
	assert.Len(t, stubs, 1)
	assert.Equal(t, "GET", stubs[0].Request.Method)
	assert.Equal(t, "/api/v1/ping", stubs[0].Request.Path)

	// 5. Trigger graceful shutdown
	client.CloseIdleConnections()
	cancel()
	select {
	case err := <-errChan:
		require.NoError(t, err, "RunRecord should shut down cleanly without error")
	case <-time.After(10 * time.Second):
		t.Fatal("RunRecord timed out waiting for shutdown")
	}

	assert.Contains(t, outputBuf.String(), "DriftMock Traffic Recorder")
	assert.Contains(t, outputBuf.String(), "RECORDED")
}
