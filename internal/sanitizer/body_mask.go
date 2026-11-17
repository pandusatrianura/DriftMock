package sanitizer

import (
	"encoding/json"
	"strings"
)

// DefaultSensitiveKeys lists JSON keys whose values must be redacted.
var DefaultSensitiveKeys = map[string]bool{
	"password":      true,
	"pass":          true,
	"secret":        true,
	"token":         true,
	"access_token":  true,
	"refresh_token": true,
	"id_token":      true,
	"auth":          true,
	"api_key":       true,
	"apikey":        true,
	"credit_card":   true,
	"card_number":   true,
	"cvv":           true,
	"ssn":           true,
	"nik":           true,
	"pin":           true,
}

var sensitiveSubstrings = []string{
	"password",
	"secret",
	"token",
	"api_key",
	"apikey",
	"credit_card",
	"cvv",
}

func isSensitiveKey(key string, sensitiveKeys map[string]bool) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	if sensitiveKeys != nil && sensitiveKeys[lower] {
		return true
	}
	if DefaultSensitiveKeys[lower] {
		return true
	}

	for _, sub := range sensitiveSubstrings {
		if strings.Contains(lower, sub) {
			return true
		}
	}
	return false
}

// MaskJSONBody parses raw JSON bytes, masks sensitive fields, and returns formatted JSON bytes.
// If the data is empty or invalid JSON (e.g. plain text, HTML, binary), it returns the data unchanged.
func MaskJSONBody(data []byte, sensitiveKeys map[string]bool) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		// Non-JSON payload: return as is
		return data, nil
	}

	masked := MaskValue(root, sensitiveKeys)
	return json.MarshalIndent(masked, "", "  ")
}

// MaskValue recursively traverses generic maps and slices, redacting sensitive leaf values.
func MaskValue(val any, sensitiveKeys map[string]bool) any {
	switch v := val.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		for k, child := range v {
			if isSensitiveKey(k, sensitiveKeys) {
				result[k] = RedactedValue
			} else {
				result[k] = MaskValue(child, sensitiveKeys)
			}
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = MaskValue(item, sensitiveKeys)
		}
		return result
	default:
		return val
	}
}
