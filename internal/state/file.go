package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

const (
	fileVersion         = 3
	previousFileVersion = 2
	previousPending     = "pending"
	previousStalled     = "stalled"
)

// FileStore persists intentions as an atomically replaced JSON document.
type FileStore struct {
	path string
	lock *flock.Flock
}

type fileDocument struct {
	Version int      `json:"version"`
	Intents []Intent `json:"intents"`
}

// NewFileStore creates a JSON store at path.
func NewFileStore(path string) (*FileStore, error) {
	if path == "" {
		return nil, fmt.Errorf("state file path is required")
	}
	return &FileStore{path: path}, nil
}

// NewLockedFileStore creates a store after taking the exclusive writer lock beside path.
func NewLockedFileStore(path string) (*FileStore, error) {
	store, err := NewFileStore(path)
	if err != nil {
		return nil, err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}
	lockPath := path + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create state lock: %w", err)
	}
	if err := lockFile.Chmod(0o600); err != nil {
		_ = lockFile.Close()
		return nil, fmt.Errorf("secure state lock: %w", err)
	}
	if err := lockFile.Close(); err != nil {
		return nil, fmt.Errorf("close state lock: %w", err)
	}
	store.lock = flock.New(lockPath)
	locked, err := store.lock.TryLock()
	if err != nil {
		_ = store.lock.Close()
		return nil, fmt.Errorf("lock state file: %w", err)
	}
	if !locked {
		_ = store.lock.Close()
		return nil, fmt.Errorf("state file is locked by another process")
	}
	return store, nil
}

// Close releases the writer lock, when held.
func (s *FileStore) Close() error {
	if s == nil || s.lock == nil {
		return nil
	}
	if err := s.lock.Close(); err != nil {
		return fmt.Errorf("release state lock: %w", err)
	}
	s.lock = nil
	return nil
}

// Load reads a complete state snapshot. A missing file is an empty store; malformed state fails closed.
func (s *FileStore) Load() ([]Intent, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state file: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var document fileDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode state file: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, fmt.Errorf("decode state file: %w", err)
		}
		return nil, fmt.Errorf("decode state file: multiple JSON values are not supported")
	}
	switch document.Version {
	case fileVersion:
		return append([]Intent(nil), document.Intents...), nil
	case previousFileVersion:
		return migrateVersion2Intents(document.Intents), nil
	default:
		return nil, fmt.Errorf("state file version must be %d, found %d", fileVersion, document.Version)
	}
}

func migrateVersion2Intents(intents []Intent) []Intent {
	migrated := append([]Intent(nil), intents...)
	for index := range migrated {
		switch string(migrated[index].Status) {
		case previousPending, previousStalled:
			migrated[index].Status = IntentSubmitted
			migrated[index].Failure = ""
		}
	}
	return migrated
}

// Save writes a complete snapshot to a temporary file before replacing the configured path.
func (s *FileStore) Save(intents []Intent) error {
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".onomazo-state-*")
	if err != nil {
		return fmt.Errorf("create temporary state file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = os.Remove(temporaryPath)
	}()

	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure temporary state file: %w", err)
	}
	document := fileDocument{Version: fileVersion, Intents: append([]Intent{}, intents...)}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(document); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("encode state file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync state file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close state file: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("replace state file: %w", err)
	}

	directoryHandle, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open state directory: %w", err)
	}
	defer func() {
		_ = directoryHandle.Close()
	}()
	if err := directoryHandle.Sync(); err != nil {
		return fmt.Errorf("sync state directory: %w", err)
	}
	return nil
}
