package model

import "time"

// Stub represents a recorded and sanitized HTTP interaction stored on disk.
type Stub struct {
	Version    string          `json:"version"`
	ID         string          `json:"id"`
	RecordedAt time.Time       `json:"recorded_at"`
	Request    RequestPattern  `json:"request"`
	Response   ResponsePattern `json:"response"`
}

// RequestPattern captures request metadata used for deterministic matching.
type RequestPattern struct {
	Method         string              `json:"method"`
	Path           string              `json:"path"`
	NormalizedPath string              `json:"normalized_path"`
	Query          map[string][]string `json:"query,omitempty"`
	Headers        map[string][]string `json:"headers,omitempty"`
	BodyHash       string              `json:"body_hash,omitempty"`
}

// ResponsePattern captures the recorded HTTP response to replay or diff against.
type ResponsePattern struct {
	StatusCode int                 `json:"status_code"`
	Headers    map[string][]string `json:"headers,omitempty"`
	Body       any                 `json:"body"`
	LatencyMs  int64               `json:"latency_ms"`
}
