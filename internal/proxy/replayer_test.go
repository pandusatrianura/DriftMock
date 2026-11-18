package proxy_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pandusatrianura/driftmock/internal/matcher"
	"github.com/pandusatrianura/driftmock/internal/model"
	"github.com/pandusatrianura/driftmock/internal/proxy"
	"github.com/pandusatrianura/driftmock/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedSampleStubs(t *testing.T, store storage.Store) {
	// 1. Exact route stub
	stubUsers := &model.Stub{
		ID: "GET_api_v1_users",
		Request: model.RequestPattern{
			Method:         "GET",
			Path:           "/api/v1/users",
			NormalizedPath: "/api/v1/users",
			Query: map[string][]string{
				"role": {"admin"},
			},
		},
		Response: model.ResponsePattern{
			StatusCode: 200,
			Headers: map[string][]string{
				"Content-Type": {"application/json"},
			},
			Body: map[string]any{
				"users": []string{"alice", "bob"},
			},
			LatencyMs: 60,
		},
	}
	_, err := store.Save(stubUsers)
	require.NoError(t, err)

	// 2. Dynamic parameter stub (/users/:id)
	stubUserDetail := &model.Stub{
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
				"name": "Jane Doe",
			},
			LatencyMs: 30,
		},
	}
	_, err = store.Save(stubUserDetail)
	require.NoError(t, err)
}

func TestReplay_ExactMatch(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewFileStore(tempDir)
	require.NoError(t, err)
	seedSampleStubs(t, store)

	var matchedTier matcher.MatchTier
	replayer, err := proxy.NewReplayer(store, proxy.ReplayOptions{})
	require.NoError(t, err)
	replayer.OnMatch = func(stub *model.Stub, tier matcher.MatchTier, r *http.Request) {
		matchedTier = tier
	}

	server := httptest.NewServer(replayer)
	defer server.Close()

	resp, err := server.Client().Get(server.URL + "/api/v1/users?role=admin")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "true", resp.Header.Get("X-DriftMock-Replay"))
	assert.Equal(t, "EXACT", resp.Header.Get("X-DriftMock-Tier"))
	assert.Equal(t, matcher.MatchTierExact, matchedTier)

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Contains(t, body, "users")
}

func TestReplay_PatternFallbackMatch(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewFileStore(tempDir)
	require.NoError(t, err)
	seedSampleStubs(t, store)

	var matchedTier matcher.MatchTier
	replayer, err := proxy.NewReplayer(store, proxy.ReplayOptions{})
	require.NoError(t, err)
	replayer.OnMatch = func(stub *model.Stub, tier matcher.MatchTier, r *http.Request) {
		matchedTier = tier
	}

	server := httptest.NewServer(replayer)
	defer server.Close()

	// Request user 999 (which was not explicitly recorded, but matches /users/:id)
	resp, err := server.Client().Get(server.URL + "/api/v1/users/999")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "true", resp.Header.Get("X-DriftMock-Replay"))
	assert.Equal(t, "PATTERN", resp.Header.Get("X-DriftMock-Tier"))
	assert.Equal(t, matcher.MatchTierPattern, matchedTier)

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "Jane Doe", body["name"])
}

func TestReplay_NotFound_Diagnostics(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewFileStore(tempDir)
	require.NoError(t, err)
	seedSampleStubs(t, store)

	var missCalled bool
	replayer, err := proxy.NewReplayer(store, proxy.ReplayOptions{})
	require.NoError(t, err)
	replayer.OnMiss = func(r *http.Request) {
		missCalled = true
	}

	server := httptest.NewServer(replayer)
	defer server.Close()

	resp, err := server.Client().Get(server.URL + "/api/v1/nonexistent")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.True(t, missCalled)

	var diag map[string]any
	err = json.NewDecoder(resp.Body).Decode(&diag)
	require.NoError(t, err)
	assert.Equal(t, "Not Found", diag["error"])
	assert.NotEmpty(t, diag["available_routes"])
}

func TestReplay_CORS(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewFileStore(tempDir)
	require.NoError(t, err)

	replayer, err := proxy.NewReplayer(store, proxy.ReplayOptions{})
	require.NoError(t, err)

	server := httptest.NewServer(replayer)
	defer server.Close()

	// Preflight OPTIONS request
	req, err := http.NewRequest(http.MethodOptions, server.URL+"/api/v1/users", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "http://localhost:3000", resp.Header.Get("Access-Control-Allow-Origin"))
	assert.Contains(t, resp.Header.Get("Access-Control-Allow-Methods"), "GET")
	assert.Equal(t, "true", resp.Header.Get("Access-Control-Allow-Credentials"))
}

func TestReplay_LatencySimulation(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewFileStore(tempDir)
	require.NoError(t, err)
	seedSampleStubs(t, store)

	// 1. Default snappy (< 30ms)
	replayerSnappy, _ := proxy.NewReplayer(store, proxy.ReplayOptions{})
	serverSnappy := httptest.NewServer(replayerSnappy)
	defer serverSnappy.Close()

	start := time.Now()
	resp1, err := serverSnappy.Client().Get(serverSnappy.URL + "/api/v1/users?role=admin")
	require.NoError(t, err)
	resp1.Body.Close()
	elapsedSnappy := time.Since(start)
	assert.True(t, elapsedSnappy < 40*time.Millisecond, "default replay should be snappy (< 40ms)")

	// 2. Simulate delay (stub has 60ms latency)
	replayerDelayed, _ := proxy.NewReplayer(store, proxy.ReplayOptions{SimulateDelay: true})
	serverDelayed := httptest.NewServer(replayerDelayed)
	defer serverDelayed.Close()

	start = time.Now()
	resp2, err := serverDelayed.Client().Get(serverDelayed.URL + "/api/v1/users?role=admin")
	require.NoError(t, err)
	resp2.Body.Close()
	elapsedDelayed := time.Since(start)
	assert.True(t, elapsedDelayed >= 50*time.Millisecond, "simulated delay should sleep at least ~50ms")

	// 3. Fixed latency override
	replayerFixed, _ := proxy.NewReplayer(store, proxy.ReplayOptions{FixedLatency: 40 * time.Millisecond})
	serverFixed := httptest.NewServer(replayerFixed)
	defer serverFixed.Close()

	start = time.Now()
	resp3, err := serverFixed.Client().Get(serverFixed.URL + "/api/v1/users?role=admin")
	require.NoError(t, err)
	resp3.Body.Close()
	elapsedFixed := time.Since(start)
	assert.True(t, elapsedFixed >= 35*time.Millisecond, "fixed delay should sleep at least ~35ms")
}

func TestReplay_Healthcheck(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewFileStore(tempDir)
	require.NoError(t, err)
	seedSampleStubs(t, store)

	replayer, err := proxy.NewReplayer(store, proxy.ReplayOptions{})
	require.NoError(t, err)

	server := httptest.NewServer(replayer)
	defer server.Close()

	// 1. Standard /healthz probe
	resp, err := server.Client().Get(server.URL + "/healthz")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok", resp.Header.Get("X-DriftMock-Health"))

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "ok", body["status"])
	assert.Equal(t, "replay", body["mode"])
	assert.Equal(t, float64(2), body["stubs_count"])

	// 2. Explicit /__driftmock/healthz probe
	resp2, err := server.Client().Get(server.URL + "/__driftmock/healthz")
	require.NoError(t, err)
	defer resp2.Body.Close()

	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	assert.Equal(t, "ok", resp2.Header.Get("X-DriftMock-Health"))
}
