package proxy_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/pandusatrianura/driftmock/internal/model"
	"github.com/pandusatrianura/driftmock/internal/proxy"
	"github.com/pandusatrianura/driftmock/internal/sanitizer"
	"github.com/pandusatrianura/driftmock/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecorderProxy_BasicForwardAndSave(t *testing.T) {
	// 1. Mock upstream target server
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/users", r.URL.Path)
		assert.Equal(t, "1", r.URL.Query().Get("page"))

		time.Sleep(10 * time.Millisecond) // simulate small latency

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Custom-Header", "custom-value")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"users": []map[string]any{
				{"id": 1, "name": "Budi"},
			},
		})
	}))
	defer targetServer.Close()

	// 2. Setup storage and proxy
	tempDir := t.TempDir()
	store, err := storage.NewFileStore(tempDir)
	require.NoError(t, err)

	targetURL, err := url.Parse(targetServer.URL)
	require.NoError(t, err)

	var savedStub *model.Stub
	var savedPath string
	recorder, err := proxy.NewRecorderProxy(targetURL, store, nil)
	require.NoError(t, err)
	recorder.OnStubSaved = func(s *model.Stub, path string) {
		savedStub = s
		savedPath = path
	}

	proxyServer := httptest.NewServer(recorder)
	defer proxyServer.Close()

	// 3. Send request through proxy
	client := proxyServer.Client()
	resp, err := client.Get(proxyServer.URL + "/api/v1/users?page=1")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "custom-value", resp.Header.Get("X-Custom-Header"))

	var clientBody map[string]any
	err = json.NewDecoder(resp.Body).Decode(&clientBody)
	require.NoError(t, err)
	assert.Contains(t, clientBody, "users")

	// 4. Verify recorded stub
	require.NotNil(t, savedStub)
	assert.NotEmpty(t, savedPath)
	assert.Equal(t, "GET", savedStub.Request.Method)
	assert.Equal(t, "/api/v1/users", savedStub.Request.Path)
	assert.Equal(t, 200, savedStub.Response.StatusCode)
	assert.True(t, savedStub.Response.LatencyMs >= 5)

	// Verify persistence in store
	loaded, err := store.Load(savedStub.ID)
	require.NoError(t, err)
	assert.Equal(t, savedStub.ID, loaded.ID)
}

func TestRecorderProxy_Sanitization(t *testing.T) {
	// 1. Mock upstream returning secrets
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Upstream sees client's real headers
		assert.Equal(t, "Bearer secret_jwt_123", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "session=secret_session; HttpOnly")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":   "success",
			"password": "supersecretpassword",
			"token":    "access_token_value",
			"user": map[string]any{
				"name": "Pandu",
			},
		})
	}))
	defer targetServer.Close()

	tempDir := t.TempDir()
	store, err := storage.NewFileStore(tempDir)
	require.NoError(t, err)

	targetURL, _ := url.Parse(targetServer.URL)
	var savedStub *model.Stub
	recorder, err := proxy.NewRecorderProxy(targetURL, store, sanitizer.NewPipeline())
	require.NoError(t, err)
	recorder.OnStubSaved = func(s *model.Stub, path string) {
		savedStub = s
	}

	proxyServer := httptest.NewServer(recorder)
	defer proxyServer.Close()

	// 2. Client sends request with sensitive headers
	req, err := http.NewRequest("GET", proxyServer.URL+"/api/v1/profile", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer secret_jwt_123")
	req.Header.Set("Cookie", "tracking_id=12345")

	resp, err := proxyServer.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Client still receives original response data
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "session=secret_session; HttpOnly", resp.Header.Get("Set-Cookie"))

	// 3. Verify stub is sanitized on disk
	require.NotNil(t, savedStub)
	assert.Equal(t, []string{"[REDACTED]"}, savedStub.Request.Headers["Authorization"])
	assert.Equal(t, []string{"[REDACTED]"}, savedStub.Request.Headers["Cookie"])
	assert.Equal(t, []string{"[REDACTED]"}, savedStub.Response.Headers["Set-Cookie"])

	bodyMap := savedStub.Response.Body.(map[string]any)
	assert.Equal(t, "[REDACTED]", bodyMap["password"])
	assert.Equal(t, "[REDACTED]", bodyMap["token"])
	assert.Equal(t, "success", bodyMap["status"])
}

func TestRecorderProxy_GzipDecompression(t *testing.T) {
	// 1. Mock upstream sending gzip compressed JSON
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := bytes.NewBuffer(nil)
		gz := gzip.NewWriter(buf)
		_ = json.NewEncoder(gz).Encode(map[string]any{
			"message": "compressed response",
			"count":   42,
		})
		_ = gz.Close()

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
	}))
	defer targetServer.Close()

	tempDir := t.TempDir()
	store, err := storage.NewFileStore(tempDir)
	require.NoError(t, err)

	targetURL, _ := url.Parse(targetServer.URL)
	var savedStub *model.Stub
	recorder, err := proxy.NewRecorderProxy(targetURL, store, nil)
	require.NoError(t, err)
	recorder.OnStubSaved = func(s *model.Stub, path string) {
		savedStub = s
	}

	proxyServer := httptest.NewServer(recorder)
	defer proxyServer.Close()

	// 2. Request through proxy
	resp, err := proxyServer.Client().Get(proxyServer.URL + "/api/v1/compressed")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// 3. Saved stub must contain decompressed readable JSON, not raw binary
	require.NotNil(t, savedStub)
	bodyMap, ok := savedStub.Response.Body.(map[string]any)
	require.True(t, ok, "body in stub must be parsed JSON map, not raw bytes")
	assert.Equal(t, "compressed response", bodyMap["message"])
	assert.Equal(t, float64(42), bodyMap["count"])
}

func TestRecorderProxy_PostWithBodyHash(t *testing.T) {
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(body), "Laptop")

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "item_101"})
	}))
	defer targetServer.Close()

	tempDir := t.TempDir()
	store, _ := storage.NewFileStore(tempDir)
	targetURL, _ := url.Parse(targetServer.URL)

	var savedStub *model.Stub
	recorder, _ := proxy.NewRecorderProxy(targetURL, store, nil)
	recorder.OnStubSaved = func(s *model.Stub, path string) {
		savedStub = s
	}

	proxyServer := httptest.NewServer(recorder)
	defer proxyServer.Close()

	payload := `{"name": "Laptop", "price": 1200}`
	resp, err := proxyServer.Client().Post(proxyServer.URL+"/api/v1/items", "application/json", bytes.NewBufferString(payload))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, savedStub)
	assert.NotEmpty(t, savedStub.Request.BodyHash)
	assert.Equal(t, "POST", savedStub.Request.Method)
}

func TestRecorderProxy_TargetOffline_BadGateway(t *testing.T) {
	// Point to unreachable localhost port
	deadURL, _ := url.Parse("http://127.0.0.1:59999")

	tempDir := t.TempDir()
	store, _ := storage.NewFileStore(tempDir)

	var errorOccurred bool
	recorder, err := proxy.NewRecorderProxy(deadURL, store, nil)
	require.NoError(t, err)
	recorder.OnError = func(err error) {
		errorOccurred = true
	}

	proxyServer := httptest.NewServer(recorder)
	defer proxyServer.Close()

	resp, err := proxyServer.Client().Get(proxyServer.URL + "/api/v1/anything")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
	assert.True(t, errorOccurred)

	stubs, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, stubs, "no stub should be saved when target is offline")
}
