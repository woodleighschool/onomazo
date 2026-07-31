package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCommittedJSONSchemaIsCurrent(t *testing.T) {
	t.Parallel()
	generated, err := JSONSchemaDocument()
	if err != nil {
		t.Fatalf("JSONSchemaDocument() error = %v", err)
	}
	path := filepath.Join("..", "..", "onomazo.schema.json")
	committed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read committed schema: %v", err)
	}
	if !bytes.Equal(committed, generated) {
		t.Error("onomazo.schema.json is stale; run mise run generate")
	}
}
