package matcher

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
)

// NormalizePath cleans an HTTP path and ensures it has a leading slash.
func NormalizePath(rawPath string) string {
	cleaned := path.Clean(strings.TrimSpace(rawPath))
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	return cleaned
}

// NormalizeQuery serializes url.Values into a deterministically sorted query string.
func NormalizeQuery(query url.Values) string {
	if len(query) == 0 {
		return ""
	}

	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for i, k := range keys {
		values := query[k]
		sort.Strings(values)
		for j, v := range values {
			if i > 0 || j > 0 {
				sb.WriteString("&")
			}
			sb.WriteString(url.QueryEscape(k))
			sb.WriteString("=")
			sb.WriteString(url.QueryEscape(v))
		}
	}
	return sb.String()
}

// BuildExactKey generates a deterministic lookup key for method, path, and sorted query.
// Format: <METHOD>:<path>[?<query>]
func BuildExactKey(method, rawPath string, query url.Values) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	cleanPath := NormalizePath(rawPath)
	normQuery := NormalizeQuery(query)

	if normQuery != "" {
		return fmt.Sprintf("%s:%s?%s", method, cleanPath, normQuery)
	}
	return fmt.Sprintf("%s:%s", method, cleanPath)
}

// BuildPathKey generates a lookup key using method and exact path only.
// Format: <METHOD>:<path>
func BuildPathKey(method, rawPath string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	cleanPath := NormalizePath(rawPath)
	return fmt.Sprintf("%s:%s", method, cleanPath)
}

// BuildPatternKey generates a lookup key using method and collapsed path pattern.
// Format: <METHOD>:<collapsed_path>
func BuildPatternKey(method, rawPath string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	collapsed := CollapsePath(NormalizePath(rawPath))
	return fmt.Sprintf("%s:%s", method, collapsed)
}

// HashBody returns the SHA-256 hash of a payload.
func HashBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:])
}
