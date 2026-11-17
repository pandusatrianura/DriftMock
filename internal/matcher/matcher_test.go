package matcher_test

import (
	"net/url"
	"sync"
	"testing"

	"github.com/pandusatrianura/driftmock/internal/matcher"
	"github.com/pandusatrianura/driftmock/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestCollapsePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "root",
			input:    "/",
			expected: "/",
		},
		{
			name:     "empty",
			input:    "",
			expected: "/",
		},
		{
			name:     "static path",
			input:    "/api/v1/users",
			expected: "/api/v1/users",
		},
		{
			name:     "static nested path",
			input:    "/api/v1/users/profile",
			expected: "/api/v1/users/profile",
		},
		{
			name:     "integer ID",
			input:    "/api/v1/users/42",
			expected: "/api/v1/users/:id",
		},
		{
			name:     "multiple integer IDs",
			input:    "/api/v1/users/42/posts/101",
			expected: "/api/v1/users/:id/posts/:id",
		},
		{
			name:     "UUID segment",
			input:    "/orders/550e8400-e29b-41d4-a716-446655440000/status",
			expected: "/orders/:id/status",
		},
		{
			name:     "MongoDB ObjectID segment (24 hex)",
			input:    "/documents/507f1f77bcf86cd799439011",
			expected: "/documents/:id",
		},
		{
			name:     "trailing slash preserved",
			input:    "/users/42/",
			expected: "/users/:id/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.CollapsePath(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeQuery(t *testing.T) {
	// Keys in random order
	q1 := url.Values{
		"page":  {"1"},
		"limit": {"20"},
		"sort":  {"asc"},
	}
	q2 := url.Values{
		"sort":  {"asc"},
		"page":  {"1"},
		"limit": {"20"},
	}

	norm1 := matcher.NormalizeQuery(q1)
	norm2 := matcher.NormalizeQuery(q2)

	assert.Equal(t, "limit=20&page=1&sort=asc", norm1)
	assert.Equal(t, norm1, norm2)
}

func TestMatcherHierarchy(t *testing.T) {
	m := matcher.NewMatcher()

	stubExactWithQuery := &model.Stub{
		ID: "GET_users_42_sort_asc",
		Request: model.RequestPattern{
			Method:         "GET",
			Path:           "/api/v1/users/42",
			NormalizedPath: "/api/v1/users/:id",
			Query: map[string][]string{
				"sort": {"asc"},
			},
		},
		Response: model.ResponsePattern{
			StatusCode: 200,
			Body:       map[string]any{"user": "exact_with_query"},
		},
	}

	stubExactNoQuery := &model.Stub{
		ID: "GET_users_42",
		Request: model.RequestPattern{
			Method:         "GET",
			Path:           "/api/v1/users/42",
			NormalizedPath: "/api/v1/users/:id",
		},
		Response: model.ResponsePattern{
			StatusCode: 200,
			Body:       map[string]any{"user": "exact_no_query"},
		},
	}

	m.Register(stubExactWithQuery)
	m.Register(stubExactNoQuery)

	assert.Equal(t, 2, m.Count())
	assert.Len(t, m.RegisteredRoutes(), 2)

	// 1. Tier 1: Exact match with query
	matched, tier := m.Match("GET", "/api/v1/users/42", url.Values{"sort": {"asc"}}, nil)
	assert.Equal(t, matcher.MatchTierExact, tier)
	assert.NotNil(t, matched)
	assert.Equal(t, "GET_users_42_sort_asc", matched.ID)

	// 2. Tier 1: Exact match with query params in different order
	matched, tier = m.Match("GET", "/api/v1/users/42", url.Values{"sort": {"asc"}}, nil)
	assert.Equal(t, matcher.MatchTierExact, tier)
	assert.Equal(t, "GET_users_42_sort_asc", matched.ID)

	// 3. Tier 1: Exact match without query
	matched, tier = m.Match("GET", "/api/v1/users/42", nil, nil)
	assert.Equal(t, matcher.MatchTierExact, tier)
	assert.Equal(t, "GET_users_42", matched.ID)

	// 4. Tier 2: Path match with unknown query
	matched, tier = m.Match("GET", "/api/v1/users/42", url.Values{"unknown_filter": {"xyz"}}, nil)
	assert.Equal(t, matcher.MatchTierPath, tier)
	assert.NotNil(t, matched)
	assert.Equal(t, "/api/v1/users/42", matched.Request.Path)

	// 5. Tier 3: Pattern match for different ID (e.g. user 999 instead of 42)
	matched, tier = m.Match("GET", "/api/v1/users/999", nil, nil)
	assert.Equal(t, matcher.MatchTierPattern, tier)
	assert.NotNil(t, matched)
	assert.Equal(t, "/api/v1/users/:id", matcher.CollapsePath(matched.Request.Path))

	// 6. Tier None: Unknown route
	matched, tier = m.Match("GET", "/api/v1/products", nil, nil)
	assert.Equal(t, matcher.MatchTierNone, tier)
	assert.Nil(t, matched)
}

func TestMatcherConcurrency(t *testing.T) {
	m := matcher.NewMatcher()

	// Pre-register some stubs
	for i := 0; i < 20; i++ {
		m.Register(&model.Stub{
			ID: "stub",
			Request: model.RequestPattern{
				Method: "GET",
				Path:   "/api/v1/items/1",
			},
		})
	}

	var wg sync.WaitGroup
	workers := 10
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, _ = m.Match("GET", "/api/v1/items/1", nil, nil)
				_, _ = m.Match("GET", "/api/v1/items/999", nil, nil)
			}
		}(i)
	}

	wg.Wait()
}
