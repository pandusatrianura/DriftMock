package e2e_test

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

// getFreePort obtains a free local TCP port.
func getFreePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestCompleteLifecycle_Record_Replay_Check(t *testing.T) {
	tempDir := t.TempDir()

	// =========================================================================
	// 1. SETUP TARGET SERVER (Staging v1.0)
	// =========================================================================
	stagingMux := http.NewServeMux()
	stagingMux.HandleFunc("/api/v1/users/42", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer super_secret_token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "session=super_secret_cookie; Path=/")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    42,
			"name":  "Budi Satria",
			"email": "budi@example.com",
			"role":  "admin",
		})
	})
	stagingMux.HandleFunc("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":  "access_jwt_token_123",
			"status": "authenticated",
		})
	})
	stagingMux.HandleFunc("/api/v1/products", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "electronics", r.URL.Query().Get("category"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"id": 1, "title": "Headphones", "price": 99.0},
			},
		})
	})

	stagingServerV1 := httptest.NewServer(stagingMux)

	// =========================================================================
	// 2. STAGE 1: RECORD MODE
	// =========================================================================
	proxyPort := getFreePort(t)
	recordCtx, cancelRecord := context.WithCancel(context.Background())
	recordBuf := bytes.NewBuffer(nil)

	recordOpts := cli.RecordOptions{
		Target: stagingServerV1.URL,
		Port:   proxyPort,
		OutDir: tempDir,
	}

	recordDone := make(chan error, 1)
	go func() {
		recordDone <- cli.RunRecord(recordCtx, recordOpts, recordBuf)
	}()

	proxyBaseURL := fmt.Sprintf("http://127.0.0.1:%d", proxyPort)
	client := &http.Client{Timeout: 3 * time.Second}

	// Wait for recording proxy to be ready
	var ready bool
	for i := 0; i < 30; i++ {
		time.Sleep(50 * time.Millisecond)
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort), 100*time.Millisecond)
		if err == nil {
			conn.Close()
			ready = true
			break
		}
	}
	require.True(t, ready, "recording proxy should start up within 1.5s")

	// 2.1 Send User Profile request with Authorization
	userReq, err := http.NewRequest("GET", proxyBaseURL+"/api/v1/users/42", nil)
	require.NoError(t, err)
	userReq.Header.Set("Authorization", "Bearer super_secret_token")
	userResp, err := client.Do(userReq)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, userResp.StatusCode)
	userResp.Body.Close()

	// 2.2 Send Login request with password in body
	loginPayload := `{"username": "budi", "password": "supersecretpassword"}`
	loginResp, err := client.Post(proxyBaseURL+"/api/v1/auth/login", "application/json", bytes.NewBufferString(loginPayload))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, loginResp.StatusCode)
	loginResp.Body.Close()

	// 2.3 Send Products query request
	prodResp, err := client.Get(proxyBaseURL + "/api/v1/products?category=electronics")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, prodResp.StatusCode)
	prodResp.Body.Close()

	// Stop record proxy
	client.CloseIdleConnections()
	cancelRecord()
	select {
	case err := <-recordDone:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out shutting down recording proxy")
	}

	// Verify persistence & sanitization on disk
	store, err := storage.NewFileStore(tempDir)
	require.NoError(t, err)
	stubs, err := store.List()
	require.NoError(t, err)
	require.Len(t, stubs, 3, "three stubs should be recorded")

	for _, s := range stubs {
		// Confirm all Authorization / Cookie headers are sanitized
		for _, v := range s.Request.Headers["Authorization"] {
			assert.Equal(t, "[REDACTED]", v)
		}
		for _, v := range s.Response.Headers["Set-Cookie"] {
			assert.Equal(t, "[REDACTED]", v)
		}
		// Confirm dynamic path collapsed
		if s.Request.Path == "/api/v1/users/42" {
			assert.Equal(t, "/api/v1/users/:id", s.Request.NormalizedPath)
		}
	}

	// =========================================================================
	// 3. STAGE 2: REPLAY MODE (Target Staging is completely SHUT DOWN)
	// =========================================================================
	stagingServerV1.Close() // Staging is now 100% offline!

	replayPort := getFreePort(t)
	replayCtx, cancelReplay := context.WithCancel(context.Background())
	replayBuf := bytes.NewBuffer(nil)

	replayOpts := cli.ReplayCommandOptions{
		StubsDir:      tempDir,
		Port:          replayPort,
		SimulateDelay: false,
	}

	replayDone := make(chan error, 1)
	go func() {
		replayDone <- cli.RunReplay(replayCtx, replayOpts, replayBuf)
	}()

	mockBaseURL := fmt.Sprintf("http://127.0.0.1:%d", replayPort)

	// Wait for replay server to be ready
	ready = false
	for i := 0; i < 30; i++ {
		time.Sleep(50 * time.Millisecond)
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", replayPort), 100*time.Millisecond)
		if err == nil {
			conn.Close()
			ready = true
			break
		}
	}
	require.True(t, ready, "mock replay server should start up within 1.5s")

	// 3.1 Exact Match test
	r1, err := client.Get(mockBaseURL + "/api/v1/users/42")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, r1.StatusCode)
	assert.Equal(t, "true", r1.Header.Get("X-DriftMock-Replay"))
	assert.Equal(t, "EXACT", r1.Header.Get("X-DriftMock-Tier"))
	var r1Body map[string]any
	_ = json.NewDecoder(r1.Body).Decode(&r1Body)
	r1.Body.Close()
	assert.Equal(t, "Budi Satria", r1Body["name"])

	// 3.2 Dynamic Route Pattern Fallback test (/users/999 matches /users/:id)
	r2, err := client.Get(mockBaseURL + "/api/v1/users/999")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, r2.StatusCode)
	assert.Equal(t, "PATTERN", r2.Header.Get("X-DriftMock-Tier"))
	var r2Body map[string]any
	_ = json.NewDecoder(r2.Body).Decode(&r2Body)
	r2.Body.Close()
	assert.Equal(t, "Budi Satria", r2Body["name"])

	// 3.3 CORS Preflight test
	corsReq, _ := http.NewRequest(http.MethodOptions, mockBaseURL+"/api/v1/products", nil)
	corsReq.Header.Set("Origin", "http://localhost:3000")
	corsResp, err := client.Do(corsReq)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, corsResp.StatusCode)
	assert.Equal(t, "http://localhost:3000", corsResp.Header.Get("Access-Control-Allow-Origin"))
	corsResp.Body.Close()

	// Stop replay server
	client.CloseIdleConnections()
	cancelReplay()
	select {
	case err := <-replayDone:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out shutting down mock replay server")
	}

	// =========================================================================
	// 4. STAGE 3: CONTRACT DRIFT DETECTION (Simulating Breaking Staging v2.0)
	// =========================================================================
	stagingMuxV2 := http.NewServeMux()
	// Breaking changes introduced on /api/v1/users/42:
	// - "email" field is REMOVED
	// - "id" type is changed from number to string ("42")
	stagingMuxV2.HandleFunc("/api/v1/users/42", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   "42", // type mismatch!
			"name": "Budi Satria",
			"role": "admin",
			// "email" missing!
		})
	})
	stagingMuxV2.HandleFunc("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":  "[REDACTED]",
			"status": "authenticated",
		})
	})
	// Backward-compatible addition on /api/v1/products:
	stagingMuxV2.HandleFunc("/api/v1/products", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"id": 1, "title": "Headphones", "price": 99.0, "in_stock": true}, // new field
			},
		})
	})

	stagingServerV2 := httptest.NewServer(stagingMuxV2)
	defer stagingServerV2.Close()

	checkBuf := bytes.NewBuffer(nil)
	checkOpts := cli.CheckOptions{
		Target:         stagingServerV2.URL,
		StubsDir:       tempDir,
		Strict:         true,
		IgnoreVolatile: true,
	}

	hasBreaking, err := cli.RunCheck(context.Background(), checkOpts, checkBuf)
	require.NoError(t, err)
	assert.True(t, hasBreaking, "drift check must detect breaking contract changes")

	checkOutput := checkBuf.String()
	assert.Contains(t, checkOutput, "CONTRACT DRIFT DETECTED")
	assert.Contains(t, checkOutput, "email: field 'email' is missing")
	assert.Contains(t, checkOutput, "id: type mismatch: expected number, received string")
	assert.Contains(t, checkOutput, "FAILED (breaking contract drift detected)")
}
