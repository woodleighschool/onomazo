package app

import (
	"path/filepath"
	"testing"

	"github.com/woodleighschool/onomazo/internal/config"
)

func TestBuildStoreLocksApplyButNotReadOnly(t *testing.T) {
	t.Parallel()
	stateConfig := config.State{Type: "file", Path: filepath.Join(t.TempDir(), "state.json")}
	_, firstCloser, err := buildStore(stateConfig, BuildApply)
	if err != nil {
		t.Fatalf("first buildStore(apply) error = %v", err)
	}
	t.Cleanup(func() { _ = firstCloser.Close() })
	if _, secondCloser, err := buildStore(stateConfig, BuildApply); err == nil {
		_ = secondCloser.Close()
		t.Fatal("second buildStore(apply) error = nil, want writer lock error")
	}
	_, readOnlyCloser, err := buildStore(stateConfig, BuildReadOnly)
	if err != nil {
		t.Fatalf("buildStore(read only) error = %v", err)
	}
	if err := readOnlyCloser.Close(); err != nil {
		t.Fatalf("read-only Close() error = %v", err)
	}
}
