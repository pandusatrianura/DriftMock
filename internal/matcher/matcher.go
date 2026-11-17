package matcher

import (
	"net/url"
	"sort"
	"sync"

	"github.com/pandusatrianura/driftmock/internal/model"
)

// MatchTier represents the fidelity level of a matched stub.
type MatchTier string

const (
	// MatchTierExact matches method, path, and query parameters completely.
	MatchTierExact MatchTier = "EXACT"

	// MatchTierPath matches method and path, falling back when query parameters differ.
	MatchTierPath MatchTier = "PATH"

	// MatchTierPattern matches dynamic route patterns with collapsed path parameters (e.g. /users/:id).
	MatchTierPattern MatchTier = "PATTERN"

	// MatchTierNone indicates no stub matched the incoming request.
	MatchTierNone MatchTier = "NONE"
)

// Matcher performs deterministic two-tier matching of incoming requests against recorded stubs.
type Matcher struct {
	mu           sync.RWMutex
	exactMap     map[string]*model.Stub
	exactPathMap map[string]*model.Stub
	patternMap   map[string]*model.Stub
}

// NewMatcher creates an initialized Matcher instance.
func NewMatcher() *Matcher {
	return &Matcher{
		exactMap:     make(map[string]*model.Stub),
		exactPathMap: make(map[string]*model.Stub),
		patternMap:   make(map[string]*model.Stub),
	}
}

// Register registers a single stub into the exact, path, and pattern lookup indices.
func (m *Matcher) Register(stub *model.Stub) {
	if stub == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	method := stub.Request.Method
	reqPath := stub.Request.Path
	queryValues := url.Values(stub.Request.Query)

	// Ensure NormalizedPath is set if missing
	if stub.Request.NormalizedPath == "" {
		stub.Request.NormalizedPath = CollapsePath(reqPath)
	}

	// 1. Exact Key: METHOD:/path?query
	exactKey := BuildExactKey(method, reqPath, queryValues)
	m.exactMap[exactKey] = stub

	// 2. Exact Path Key: METHOD:/path
	pathKey := BuildPathKey(method, reqPath)
	if _, exists := m.exactPathMap[pathKey]; !exists {
		m.exactPathMap[pathKey] = stub
	}

	// 3. Pattern Key: METHOD:/path_with_collapsed_ids (e.g. GET:/api/v1/users/:id)
	patternKey := BuildPatternKey(method, reqPath)
	if _, exists := m.patternMap[patternKey]; !exists {
		m.patternMap[patternKey] = stub
	}
}

// RegisterAll registers a collection of stubs into the matcher.
func (m *Matcher) RegisterAll(stubs []*model.Stub) {
	for _, stub := range stubs {
		m.Register(stub)
	}
}

// Count returns the total number of exact route stubs registered.
func (m *Matcher) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.exactMap)
}

// RegisteredRoutes returns a sorted list of unique route descriptions.
func (m *Matcher) RegisteredRoutes() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	routes := make([]string, 0, len(m.exactMap))
	for k := range m.exactMap {
		routes = append(routes, k)
	}
	sort.Strings(routes)
	return routes
}

// Match finds the best matching Stub for an incoming HTTP request.
// Hierarchy:
//  1. Tier 1: Exact Match (Method + Path + Query)
//  2. Tier 2: Path Match (Method + Path fallback)
//  3. Tier 3: Pattern Match (Method + Collapsed Path Fallback, e.g. /users/:id)
func (m *Matcher) Match(method, rawPath string, query url.Values, body []byte) (*model.Stub, MatchTier) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Tier 1: Exact match with query params
	exactKey := BuildExactKey(method, rawPath, query)
	if stub, found := m.exactMap[exactKey]; found {
		return stub, MatchTierExact
	}

	// Tier 2: Exact path match (ignoring query differences)
	pathKey := BuildPathKey(method, rawPath)
	if stub, found := m.exactPathMap[pathKey]; found {
		return stub, MatchTierPath
	}

	// Tier 3: Pattern match for dynamic routes (/users/123 -> /users/:id)
	patternKey := BuildPatternKey(method, rawPath)
	if stub, found := m.patternMap[patternKey]; found {
		return stub, MatchTierPattern
	}

	return nil, MatchTierNone
}
