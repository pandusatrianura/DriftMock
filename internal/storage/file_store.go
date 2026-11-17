package storage

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pandusatrianura/driftmock/internal/model"
)

// Store defines persistence operations for recorded stubs.
type Store interface {
	Save(stub *model.Stub) (string, error)
	Load(idOrFilename string) (*model.Stub, error)
	List() ([]*model.Stub, error)
	StubsDir() string
}

// FileStore manages reading and writing stubs to disk in JSON format.
type FileStore struct {
	baseDir  string
	stubsDir string
	mu       sync.RWMutex
}

// NewFileStore initializes a FileStore with the specified base directory (e.g. "./.drift").
// It ensures that the nested stubs/ directory exists.
func NewFileStore(baseDir string) (*FileStore, error) {
	if strings.TrimSpace(baseDir) == "" {
		baseDir = ".drift"
	}

	stubsDir := filepath.Join(baseDir, "stubs")
	if err := os.MkdirAll(stubsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create stubs directory '%s': %w", stubsDir, err)
	}

	return &FileStore{
		baseDir:  baseDir,
		stubsDir: stubsDir,
	}, nil
}

// StubsDir returns the absolute or configured path to the stubs folder.
func (s *FileStore) StubsDir() string {
	return s.stubsDir
}

// Save serializes and persists a Stub to disk. If stub.ID is empty, it generates one.
// Returns the written file path.
func (s *FileStore) Save(stub *model.Stub) (string, error) {
	if stub == nil {
		return "", fmt.Errorf("cannot save nil stub")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if stub.ID == "" {
		queryValues := url.Values(stub.Request.Query)
		stub.ID = GenerateStubID(stub.Request.Method, stub.Request.Path, queryValues)
	}

	filename := stub.ID
	if !strings.HasSuffix(filename, ".json") {
		filename += ".json"
	}

	targetPath := filepath.Join(s.stubsDir, filename)

	data, err := json.MarshalIndent(stub, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal stub to JSON: %w", err)
	}

	// Append newline at end of file for POSIX/Git compatibility
	data = append(data, '\n')

	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write stub file '%s': %w", targetPath, err)
	}

	return targetPath, nil
}

// Load reads and parses a stub file given its ID or filename.
func (s *FileStore) Load(idOrFilename string) (*model.Stub, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filename := idOrFilename
	if !strings.HasSuffix(filename, ".json") {
		filename += ".json"
	}

	targetPath := filepath.Join(s.stubsDir, filename)

	data, err := os.ReadFile(targetPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read stub file '%s': %w", targetPath, err)
	}

	var stub model.Stub
	if err := json.Unmarshal(data, &stub); err != nil {
		return nil, fmt.Errorf("failed to parse stub JSON from '%s': %w", targetPath, err)
	}

	return &stub, nil
}

// List scans the stubs directory and loads all valid JSON stub records.
func (s *FileStore) List() ([]*model.Stub, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.stubsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*model.Stub{}, nil
		}
		return nil, fmt.Errorf("failed to read stubs directory: %w", err)
	}

	stubs := make([]*model.Stub, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(s.stubsDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			// Skip unreadable files or continue
			continue
		}

		var stub model.Stub
		if err := json.Unmarshal(data, &stub); err != nil {
			// Ignore non-stub or invalid JSON files
			continue
		}

		stubs = append(stubs, &stub)
	}

	return stubs, nil
}
