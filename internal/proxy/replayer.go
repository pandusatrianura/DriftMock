package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/pandusatrianura/driftmock/internal/matcher"
	"github.com/pandusatrianura/driftmock/internal/model"
	"github.com/pandusatrianura/driftmock/internal/storage"
)

// ReplayOptions defines execution behavior for offline mock serving.
type ReplayOptions struct {
	SimulateDelay bool          // if true, simulates the recorded latency_ms from each stub
	FixedLatency  time.Duration // if > 0, overrides all request latencies with a fixed duration
}

// Replayer manages loading stubs, deterministic request matching, and mock response rendering.
type Replayer struct {
	mu      sync.RWMutex
	matcher *matcher.Matcher
	store   storage.Store
	options ReplayOptions
	OnMatch func(stub *model.Stub, tier matcher.MatchTier, r *http.Request)
	OnMiss  func(r *http.Request)
}

// NewReplayer initializes a Replayer, scanning and registering all stubs from storage.
func NewReplayer(store storage.Store, opts ReplayOptions) (*Replayer, error) {
	if store == nil {
		return nil, fmt.Errorf("storage store cannot be nil")
	}

	replayer := &Replayer{
		store:   store,
		options: opts,
	}

	if err := replayer.Reload(); err != nil {
		return nil, fmt.Errorf("failed to load stubs: %w", err)
	}

	return replayer, nil
}

// Reload re-reads all stubs from disk and re-indexes the matcher.
func (rp *Replayer) Reload() error {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	stubs, err := rp.store.List()
	if err != nil {
		return err
	}

	m := matcher.NewMatcher()
	m.RegisterAll(stubs)
	rp.matcher = m

	return nil
}

// Matcher returns the internal Matcher instance.
func (rp *Replayer) Matcher() *matcher.Matcher {
	rp.mu.RLock()
	defer rp.mu.RUnlock()
	return rp.matcher
}

// ServeHTTP handles incoming client requests and returns matched mock responses.
func (rp *Replayer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Handle CORS preflight and headers
	if HandleCORS(w, r) {
		return
	}

	// 2. Read request body if present, bounded by DefaultMaxBodyBytes
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = io.ReadAll(io.LimitReader(r.Body, DefaultMaxBodyBytes))
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	// 3. Match against registered stubs
	rp.mu.RLock()
	m := rp.matcher
	opts := rp.options
	rp.mu.RUnlock()

	stub, tier := m.Match(r.Method, r.URL.Path, r.URL.Query(), bodyBytes)

	// 4. Handle Cache Miss / 404
	if stub == nil {
		if rp.OnMiss != nil {
			rp.OnMiss(r)
		}
		rp.writeNotFound(w, r, m)
		return
	}

	if rp.OnMatch != nil {
		rp.OnMatch(stub, tier, r)
	}

	// 5. Handle simulated latency
	if opts.FixedLatency > 0 {
		time.Sleep(opts.FixedLatency)
	} else if opts.SimulateDelay && stub.Response.LatencyMs > 0 {
		time.Sleep(time.Duration(stub.Response.LatencyMs) * time.Millisecond)
	}

	// 6. Write response headers
	for key, values := range stub.Response.Headers {
		// Do not copy Content-Length or hop-by-hop headers directly
		if isHopByHopOrLength(key) {
			continue
		}
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}

	w.Header().Set("X-DriftMock-Replay", "true")
	w.Header().Set("X-DriftMock-Tier", string(tier))
	w.Header().Set("X-DriftMock-Stub-ID", stub.ID)

	statusCode := stub.Response.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	w.WriteHeader(statusCode)

	// 7. Write response body
	if stub.Response.Body == nil {
		return
	}

	switch b := stub.Response.Body.(type) {
	case string:
		_, _ = w.Write([]byte(b))
	case []byte:
		_, _ = w.Write(b)
	default:
		// Map or slice: serialize back to clean JSON
		data, err := json.Marshal(b)
		if err == nil {
			if w.Header().Get("Content-Type") == "" {
				w.Header().Set("Content-Type", "application/json")
			}
			_, _ = w.Write(data)
		}
	}
}

func (rp *Replayer) writeNotFound(w http.ResponseWriter, r *http.Request, m *matcher.Matcher) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)

	routes := []string{}
	if m != nil {
		routes = m.RegisteredRoutes()
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":   "Not Found",
		"message": fmt.Sprintf("No matching mock stub found for %s %s", r.Method, r.URL.Path),
		"request": map[string]any{
			"method": r.Method,
			"path":   r.URL.Path,
			"query":  r.URL.Query(),
		},
		"available_routes": routes,
	})
}

func isHopByHopOrLength(headerKey string) bool {
	switch http.CanonicalHeaderKey(headerKey) {
	case "Connection", "Keep-Alive", "Proxy-Authenticate",
		"Proxy-Authorization", "Te", "Trailers", "Transfer-Encoding",
		"Upgrade", "Content-Length":
		return true
	default:
		return false
	}
}
