package matcher

import (
	"regexp"
	"strings"
)

var (
	// intPattern matches positive integer segments (e.g., 42, 1001)
	intPattern = regexp.MustCompile(`^[0-9]+$`)

	// uuidPattern matches canonical RFC 4122 UUID segments
	uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

	// mongoIDPattern matches standard 24-character hexadecimal MongoDB ObjectIDs
	mongoIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{24}$`)
)

// IsDynamicSegment returns true if a single path segment represents an ID.
func IsDynamicSegment(segment string) bool {
	if segment == "" {
		return false
	}
	return intPattern.MatchString(segment) ||
		uuidPattern.MatchString(segment) ||
		mongoIDPattern.MatchString(segment)
}

// CollapsePath converts dynamic ID segments in a REST URL path to ':id'.
// Examples:
//
//	/api/v1/users/42          -> /api/v1/users/:id
//	/api/v1/users/42/posts/99 -> /api/v1/users/:id/posts/:id
//	/orders/550e8400-e29b-41d4-a716-446655440000 -> /orders/:id
//	/documents/507f1f77bcf86cd799439011 -> /documents/:id
//	/api/v1/users/profile     -> /api/v1/users/profile (unchanged)
func CollapsePath(path string) string {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" || cleanPath == "/" {
		return "/"
	}

	hasLeadingSlash := strings.HasPrefix(cleanPath, "/")
	hasTrailingSlash := strings.HasSuffix(cleanPath, "/") && len(cleanPath) > 1

	trimmed := strings.Trim(cleanPath, "/")
	if trimmed == "" {
		return "/"
	}

	segments := strings.Split(trimmed, "/")
	for i, seg := range segments {
		if IsDynamicSegment(seg) {
			segments[i] = ":id"
		}
	}

	result := strings.Join(segments, "/")
	if hasLeadingSlash {
		result = "/" + result
	}
	if hasTrailingSlash {
		result = result + "/"
	}

	return result
}
