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

// resolveStubPath sanitizes an ID or filename and ensures the resulting path is strictly inside stubsDir.
func (s *FileStore) resolveStubPath(idOrFilename string) (string, error) {
	cleanID := strings.TrimSpace(idOrFilename)
	if cleanID == "" {
		return "", fmt.Errorf("stub ID or filename cannot be empty")
	}

	// Reject directory traversal characters explicitly
	if strings.Contains(cleanID, "..") || strings.ContainsAny(cleanID, `/\`) {
		return "", fmt.Errorf("invalid stub ID '%s': directory traversal is not permitted", idOrFilename)
	}

	base := filepath.Base(cleanID)
	filename := base
	if !strings.HasSuffix(filename, ".json") {
		filename += ".json"
	}

	targetPath := filepath.Join(s.stubsDir, filename)
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	absStubsDir, err := filepath.Abs(s.stubsDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve stubs directory: %w", err)
	}

	rel, err := filepath.Rel(absStubsDir, absTarget)
	if err != nil || strings.HasPrefix(rel, "..") || rel == "." {
		return "", fmt.Errorf("invalid stub path '%s': attempted path traversal outside stubs directory", idOrFilename)
	}

	return targetPath, nil
}

// Save serializes and persists a Stub to disk atomically. If stub.ID is empty, it generates one.
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

	targetPath, err := s.resolveStubPath(stub.ID)
	if err != nil {
		return "", fmt.Errorf("invalid stub identifier: %w", err)
	}

	data, err := json.MarshalIndent(stub, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal stub to JSON: %w", err)
	}

	// Append newline at end of file for POSIX/Git compatibility
	data = append(data, '\n')

	// Atomic persistence: write to a temporary file in stubsDir, then atomically rename
	tmpFile, err := os.CreateTemp(s.stubsDir, ".stub-*.tmp")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary stub file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("failed to write to temporary stub file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("failed to close temporary stub file: %w", err)
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("failed to atomically commit stub file '%s': %w", targetPath, err)
	}

	return targetPath, nil
}

// Load reads and parses a stub file given its ID or filename.
func (s *FileStore) Load(idOrFilename string) (*model.Stub, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	targetPath, err := s.resolveStubPath(idOrFilename)
	if err != nil {
		return nil, err
	}

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
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || !strings.HasSuffix(entry.Name(), ".json") {
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
