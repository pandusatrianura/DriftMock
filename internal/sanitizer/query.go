package sanitizer

import (
	"net/url"
)

// SanitizeQueryParams redacts sensitive query parameters (e.g. token, api_key, secret, password).
func SanitizeQueryParams(query map[string][]string, sensitiveKeys map[string]bool) map[string][]string {
	if query == nil {
		return nil
	}

	sanitized := make(map[string][]string, len(query))
	for key, values := range query {
		if isSensitiveKey(key, sensitiveKeys) {
			sanitized[key] = []string{RedactedValue}
		} else {
			copied := make([]string, len(values))
			copy(copied, values)
			sanitized[key] = copied
		}
	}
	return sanitized
}

// SanitizeURLValues creates a sanitized copy of url.Values with sensitive keys redacted.
func SanitizeURLValues(v url.Values, sensitiveKeys map[string]bool) url.Values {
	if v == nil {
		return nil
	}
	return url.Values(SanitizeQueryParams(map[string][]string(v), sensitiveKeys))
}
