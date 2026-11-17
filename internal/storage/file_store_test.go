package storage_test

import (
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/pandusatrianura/driftmock/internal/model"
	"github.com/pandusatrianura/driftmock/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateStubFilename(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		query    url.Values
		expected string
	}{
		{
			name:     "root path",
			method:   "GET",
			path:     "/",
			query:    nil,
			expected: "GET_root.json",
		},
		{
			name:     "simple resource",
			method:   "get",
			path:     "/api/v1/users",
			query:    nil,
			expected: "GET_api_v1_users.json",
		},
		{
			name:     "nested resource with path params and special chars",
			method:   "POST",
			path:     "/api/v1/users/42/settings",
			query:    nil,
			expected: "POST_api_v1_users_42_settings.json",
		},
		{
			name:   "path with query parameters",
			method: "GET",
			path:   "/api/v1/users",
			query: url.Values{
				"sort":  {"asc"},
				"limit": {"10"},
			},
			// Hash should be deterministic for sorted query params
			expected: "GET_api_v1_users_" + storage.HashQueryParams(url.Values{"limit": {"10"}, "sort": {"asc"}}) + ".json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := storage.GenerateStubFilename(tt.method, tt.path, tt.query)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHashQueryParamsDeterminism(t *testing.T) {
	// Order of keys should not affect hash
	q1 := url.Values{
		"a": {"1"},
		"b": {"2"},
		"c": {"3"},
	}
	q2 := url.Values{
		"c": {"3"},
		"a": {"1"},
		"b": {"2"},
	}

	hash1 := storage.HashQueryParams(q1)
	hash2 := storage.HashQueryParams(q2)

	assert.NotEmpty(t, hash1)
	assert.Equal(t, hash1, hash2)
	assert.Len(t, hash1, 8)
}

func TestFileStoreSaveAndLoad(t *testing.T) {
	tempDir := t.TempDir()

	store, err := storage.NewFileStore(tempDir)
	require.NoError(t, err)
	assert.DirExists(t, filepath.Join(tempDir, "stubs"))

	sampleStub := &model.Stub{
		Version:    "1.0",
		ID:         "GET_api_v1_profile",
		RecordedAt: time.Now().UTC().Truncate(time.Second),
		Request: model.RequestPattern{
			Method:         "GET",
			Path:           "/api/v1/profile",
			NormalizedPath: "/api/v1/profile",
		},
		Response: model.ResponsePattern{
			StatusCode: 200,
			Headers: map[string][]string{
				"Content-Type": {"application/json"},
			},
			Body: map[string]any{
				"username": "pandu",
				"role":     "engineer",
			},
			LatencyMs: 45,
		},
	}

	// 1. Save stub
	filePath, err := store.Save(sampleStub)
	require.NoError(t, err)
	assert.FileExists(t, filePath)

	// Verify file ends with newline
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.True(t, len(data) > 0 && data[len(data)-1] == '\n')

	// 2. Load stub by ID
	loaded, err := store.Load(sampleStub.ID)
	require.NoError(t, err)
	assert.Equal(t, sampleStub.ID, loaded.ID)
	assert.Equal(t, sampleStub.Request.Method, loaded.Request.Method)
	assert.Equal(t, sampleStub.Request.Path, loaded.Request.Path)
	assert.Equal(t, sampleStub.Response.StatusCode, loaded.Response.StatusCode)
	assert.Equal(t, int64(45), loaded.Response.LatencyMs)

	// 3. Load stub by full filename
	loadedByFilename, err := store.Load(sampleStub.ID + ".json")
	require.NoError(t, err)
	assert.Equal(t, sampleStub.ID, loadedByFilename.ID)

	// 4. List stubs
	stubs, err := store.List()
	require.NoError(t, err)
	assert.Len(t, stubs, 1)
	assert.Equal(t, sampleStub.ID, stubs[0].ID)

	// 5. Load non-existent stub returns error
	_, err = store.Load("non_existent_stub")
	assert.Error(t, err)
}

func TestFileStoreAutoIDGeneration(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewFileStore(tempDir)
	require.NoError(t, err)

	stub := &model.Stub{
		Version: "1.0",
		Request: model.RequestPattern{
			Method: "POST",
			Path:   "/api/v1/checkout",
		},
		Response: model.ResponsePattern{
			StatusCode: 201,
			Body:       map[string]any{"status": "ok"},
		},
	}

	filePath, err := store.Save(stub)
	require.NoError(t, err)
	assert.Contains(t, filePath, "POST_api_v1_checkout.json")
	assert.Equal(t, "POST_api_v1_checkout", stub.ID)
}

func TestFileStoreConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewFileStore(tempDir)
	require.NoError(t, err)

	const workerCount = 10
	var wg sync.WaitGroup
	wg.Add(workerCount)

	for i := 0; i < workerCount; i++ {
		go func(idx int) {
			defer wg.Done()
			stub := &model.Stub{
				Version: "1.0",
				Request: model.RequestPattern{
					Method: "GET",
					Path:   "/api/v1/items",
					Query: map[string][]string{
						"page": {string(rune('0' + idx))},
					},
				},
				Response: model.ResponsePattern{
					StatusCode: 200,
					Body:       map[string]any{"item": idx},
				},
			}
			_, saveErr := store.Save(stub)
			assert.NoError(t, saveErr)

			_, listErr := store.List()
			assert.NoError(t, listErr)
		}(i)
	}

	wg.Wait()

	allStubs, err := store.List()
	require.NoError(t, err)
	assert.Equal(t, workerCount, len(allStubs))
}
