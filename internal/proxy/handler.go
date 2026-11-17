package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"time"
)

type contextKey string

const (
	startTimeKey   contextKey = "driftmock_start_time"
	requestBodyKey contextKey = "driftmock_request_body"
)

// decompressIfNeeded handles gzip decompressed content for stub persistence.
// If the content is not gzip or decompression fails, the original data is returned.
func decompressIfNeeded(encoding string, data []byte) []byte {
	if len(data) == 0 {
		return data
	}

	if strings.EqualFold(strings.TrimSpace(encoding), "gzip") {
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return data
		}
		defer reader.Close()

		decompressed, err := io.ReadAll(reader)
		if err != nil {
			return data
		}
		return decompressed
	}

	return data
}

// hashBody returns the SHA-256 hash of a body payload.
func hashBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:])
}

// withStartTime attaches a recording start timestamp to the request context.
func withStartTime(ctx context.Context, t time.Time) context.Context {
	return context.WithValue(ctx, startTimeKey, t)
}

// getStartTime retrieves the recording start timestamp from context.
func getStartTime(ctx context.Context) (time.Time, bool) {
	t, ok := ctx.Value(startTimeKey).(time.Time)
	return t, ok
}

// cloneHeader creates a deep copy of an http.Header.
func cloneHeader(h http.Header) map[string][]string {
	if h == nil {
		return nil
	}
	res := make(map[string][]string, len(h))
	for k, vv := range h {
		copied := make([]string, len(vv))
		copy(copied, vv)
		res[k] = copied
	}
	return res
}
