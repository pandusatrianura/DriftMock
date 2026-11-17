package sanitizer

import (
	"net/http"
	"strings"
)

// DefaultSensitiveHeaders lists HTTP header names that must be redacted by default.
var DefaultSensitiveHeaders = map[string]bool{
	"authorization":       true,
	"cookie":              true,
	"set-cookie":          true,
	"x-api-key":           true,
	"x-auth-token":        true,
	"x-access-token":      true,
	"x-secret-token":      true,
	"proxy-authorization": true,
}

// RedactedValue is the standard replacement string for redacted sensitive values.
const RedactedValue = "[REDACTED]"

// SanitizeHeaders creates a sanitized copy of a headers map, redacting sensitive header values.
func SanitizeHeaders(headers map[string][]string, sensitiveHeaders map[string]bool) map[string][]string {
	if headers == nil {
		return nil
	}

	if sensitiveHeaders == nil {
		sensitiveHeaders = DefaultSensitiveHeaders
	}

	sanitized := make(map[string][]string, len(headers))
	for key, values := range headers {
		lowerKey := strings.ToLower(key)
		if sensitiveHeaders[lowerKey] {
			sanitized[key] = []string{RedactedValue}
		} else {
			// Copy values slice to avoid mutation
			copiedValues := make([]string, len(values))
			copy(copiedValues, values)
			sanitized[key] = copiedValues
		}
	}

	return sanitized
}

// SanitizeHTTPHeader creates a sanitized copy of an http.Header.
func SanitizeHTTPHeader(h http.Header, sensitiveHeaders map[string]bool) http.Header {
	return http.Header(SanitizeHeaders(map[string][]string(h), sensitiveHeaders))
}
