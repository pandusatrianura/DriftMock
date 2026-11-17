package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

var unsafePathChars = regexp.MustCompile(`[^a-zA-Z0-9_\-]+`)

// GenerateStubID generates a deterministic identifier for an HTTP interaction.
// Format: <METHOD>_<path_underscored>[_<query_hash>]
func GenerateStubID(method, rawPath string, query url.Values) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "GET"
	}

	cleanPath := strings.Trim(rawPath, "/")
	if cleanPath == "" {
		cleanPath = "root"
	} else {
		// Replace slashes with underscores and sanitize other unsafe characters
		cleanPath = strings.ReplaceAll(cleanPath, "/", "_")
		cleanPath = unsafePathChars.ReplaceAllString(cleanPath, "_")
		cleanPath = strings.Trim(cleanPath, "_")
	}

	id := fmt.Sprintf("%s_%s", method, cleanPath)

	queryHash := HashQueryParams(query)
	if queryHash != "" {
		id = fmt.Sprintf("%s_%s", id, queryHash)
	}

	return id
}

// GenerateStubFilename returns the filename including .json extension.
func GenerateStubFilename(method, rawPath string, query url.Values) string {
	return GenerateStubID(method, rawPath, query) + ".json"
}

// HashQueryParams returns an 8-character hex hash of sorted query parameters.
// If query is empty, it returns an empty string.
func HashQueryParams(query url.Values) string {
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

	queryString := sb.String()
	if queryString == "" {
		return ""
	}

	hash := sha256.Sum256([]byte(queryString))
	return hex.EncodeToString(hash[:])[:8]
}
