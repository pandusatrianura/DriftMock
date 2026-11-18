package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/pandusatrianura/driftmock/internal/matcher"
	"github.com/pandusatrianura/driftmock/internal/model"
	"github.com/pandusatrianura/driftmock/internal/sanitizer"
	"github.com/pandusatrianura/driftmock/internal/storage"
)

// DefaultMaxBodyBytes limits request and response buffering to 10MB to prevent OOM/DoS.
const DefaultMaxBodyBytes = 10 * 1024 * 1024

// RecorderProxy acts as an HTTP reverse proxy that intercepts, sanitizes,
// and persists request-response pairs as JSON stubs.
type RecorderProxy struct {
	targetURL    *url.URL
	store        storage.Store
	sanitizer    *sanitizer.Pipeline
	reverseProxy *httputil.ReverseProxy
	maxBodyBytes int64
	OnStubSaved  func(stub *model.Stub, filePath string)
	OnError      func(err error)
}

// NewRecorderProxy constructs a new RecorderProxy pointing to the specified target.
func NewRecorderProxy(target *url.URL, store storage.Store, pipeline *sanitizer.Pipeline) (*RecorderProxy, error) {
	if target == nil {
		return nil, fmt.Errorf("target URL cannot be nil")
	}
	if store == nil {
		return nil, fmt.Errorf("storage store cannot be nil")
	}
	if pipeline == nil {
		pipeline = sanitizer.NewPipeline()
	}

	rp := &RecorderProxy{
		targetURL:    target,
		store:        store,
		sanitizer:    pipeline,
		maxBodyBytes: DefaultMaxBodyBytes,
	}

	proxy := &httputil.ReverseProxy{
		Director:       rp.director,
		ModifyResponse: rp.modifyResponse,
		ErrorHandler:   rp.errorHandler,
	}

	rp.reverseProxy = proxy
	return rp, nil
}

// ServeHTTP delegates request handling to the internal ReverseProxy.
func (rp *RecorderProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rp.reverseProxy.ServeHTTP(w, r)
}

// director rewrites request parameters to point to the upstream target and captures metadata.
func (rp *RecorderProxy) director(req *http.Request) {
	req.URL.Scheme = rp.targetURL.Scheme
	req.URL.Host = rp.targetURL.Host
	req.Host = rp.targetURL.Host

	// Record start timestamp for latency measurement
	ctx := withStartTime(req.Context(), time.Now())

	// Read and buffer request body if present, bounded by maxBodyBytes
	var bodyBytes []byte
	if req.Body != nil {
		bodyBytes, _ = io.ReadAll(io.LimitReader(req.Body, rp.maxBodyBytes))
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	ctx = contextWithValue(ctx, requestBodyKey, bodyBytes)
	*req = *req.WithContext(ctx)
}

// modifyResponse captures the upstream response, measures latency, sanitizes, and persists the stub.
func (rp *RecorderProxy) modifyResponse(resp *http.Response) error {
	req := resp.Request
	startTime, ok := getStartTime(req.Context())
	var latencyMs int64
	if ok {
		latencyMs = time.Since(startTime).Milliseconds()
	}

	// Read raw response body bytes, bounded by maxBodyBytes
	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, rp.maxBodyBytes))
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Restore original body to response so downstream client receives it intact
	resp.Body = io.NopCloser(bytes.NewBuffer(rawBody))

	// Decompress if gzip encoded for readable stub storage
	decompressedBody := decompressIfNeeded(resp.Header.Get("Content-Encoding"), rawBody)

	// Parse and mask sensitive body contents
	sanitizedBody, err := rp.sanitizer.SanitizeRawResponseBody(decompressedBody)
	if err != nil {
		sanitizedBody = string(decompressedBody)
	}

	// Extract buffered request body for hash calculation
	var reqBodyBytes []byte
	if val := req.Context().Value(requestBodyKey); val != nil {
		if b, ok := val.([]byte); ok {
			reqBodyBytes = b
		}
	}
	bodyHash := hashBody(reqBodyBytes)

	// Construct model.Stub
	stub := &model.Stub{
		Version:    "1.0",
		RecordedAt: time.Now().UTC().Truncate(time.Millisecond),
		Request: model.RequestPattern{
			Method:         req.Method,
			Path:           req.URL.Path,
			NormalizedPath: matcher.CollapsePath(req.URL.Path),
			Query:          cloneHeader(http.Header(req.URL.Query())),
			Headers:        cloneHeader(req.Header),
			BodyHash:       bodyHash,
		},
		Response: model.ResponsePattern{
			StatusCode: resp.StatusCode,
			Headers:    cloneHeader(resp.Header),
			Body:       sanitizedBody,
			LatencyMs:  latencyMs,
		},
	}

	// Apply sanitizer to headers and body
	rp.sanitizer.SanitizeStub(stub)

	// Persist to store
	filePath, err := rp.store.Save(stub)
	if err != nil {
		if rp.OnError != nil {
			rp.OnError(fmt.Errorf("failed to save stub: %w", err))
		}
	} else if rp.OnStubSaved != nil {
		rp.OnStubSaved(stub, filePath)
	}

	return nil
}

// errorHandler handles failures communicating with the upstream target server.
func (rp *RecorderProxy) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	if rp.OnError != nil {
		rp.OnError(err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)

	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":   "Bad Gateway",
		"message": fmt.Sprintf("Failed to reach target server at '%s': %v", rp.targetURL.String(), err),
	})
}

func contextWithValue(ctx context.Context, key contextKey, val any) context.Context {
	return context.WithValue(ctx, key, val)
}
