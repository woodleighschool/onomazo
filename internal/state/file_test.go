package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileStoreRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "state.json")
	store, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	intent := Intent{
		Source:       "intune",
		DeviceID:     "device",
		SerialNumber: "SERIAL",
		DesiredName:  "NEW",
		AttemptedAt:  time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC),
		Attempts:     1,
		Status:       IntentPending,
	}
	if err := store.Save([]Intent{intent}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := len(loaded), 1; got != want {
		t.Fatalf("loaded intents = %d, want %d", got, want)
	}
	if got, want := loaded[0], intent; got != want {
		t.Errorf("loaded intent = %#v, want %#v", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Errorf("file mode = %o, want %o", got, want)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".onomazo-state-*"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("temporary state files remain: %v", matches)
	}
}

func TestFileStoreMissingFileIsEmpty(t *testing.T) {
	t.Parallel()

	store, err := NewFileStore(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	intents, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(intents) != 0 {
		t.Errorf("Load() = %#v, want empty", intents)
	}
}

func TestFileStoreRejectsMalformedOrUnknownState(t *testing.T) {
	t.Parallel()

	for name, contents := range map[string]string{
		"malformed":     `{`,
		"unknown field": `{"version":1,"intents":[],"secret":"nope"}`,
		"wrong version": `{"version":2,"intents":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "state.json")
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatalf("write state: %v", err)
			}
			store, err := NewFileStore(path)
			if err != nil {
				t.Fatalf("NewFileStore() error = %v", err)
			}
			if _, err := store.Load(); err == nil {
				t.Fatal("Load() error = nil, want corrupt state error")
			}
		})
	}
}

func TestOpenRejectsSemanticallyInvalidFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	contents := `{"version":1,"intents":[{"source":"intune","device_id":"device"}]}`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write state: %v", err)
	}
	store, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	_, err = Open(store)
	if err == nil {
		t.Fatal("Open() error = nil, want invalid intent error")
	}
	if !strings.Contains(err.Error(), "serial_number and desired_name are required") {
		t.Errorf("Open() error = %q, want semantic validation error", err)
	}
}

func TestLockedFileStoreAllowsOnlyOneWriter(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	first, err := NewLockedFileStore(path)
	if err != nil {
		t.Fatalf("first NewLockedFileStore() error = %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := NewLockedFileStore(path)
	if err == nil {
		_ = second.Close()
		t.Fatal("second NewLockedFileStore() error = nil, want writer lock error")
	}
	if err := first.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	third, err := NewLockedFileStore(path)
	if err != nil {
		t.Fatalf("third NewLockedFileStore() error = %v", err)
	}
	if err := third.Close(); err != nil {
		t.Fatalf("third Close() error = %v", err)
	}
}
