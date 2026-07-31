package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/woodleighschool/onomazo/internal/config"
	"github.com/woodleighschool/onomazo/internal/domain"
	"github.com/woodleighschool/onomazo/internal/provider/jamf"
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

func TestJamfSourceRenamesByResourceNamespace(t *testing.T) {
	t.Parallel()

	requestedPaths := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/oauth/token":
			response.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(response).Encode(map[string]any{
				"access_token": "fixture-token",
				"expires_in":   3600,
			}); err != nil {
				t.Errorf("encode token response: %v", err)
			}
		case "/api/v4/computers-inventory-detail/shared-id":
			requestedPaths <- request.URL.Path
			response.WriteHeader(http.StatusNoContent)
		case "/api/v2/mobile-devices/shared-id":
			requestedPaths <- request.URL.Path
			response.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected path %q", request.URL.Path)
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := jamf.NewClient(jamf.Config{
		URL:          server.URL,
		ClientID:     "fixture-client",
		ClientSecret: "fixture-secret",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	source := &jamfSource{name: "jamf", client: client}
	for _, namespace := range []string{jamf.ComputerNamespace, jamf.MobileDeviceNamespace} {
		device := domain.Device{Namespace: namespace, ID: "shared-id"}
		if err := source.Rename(context.Background(), device, "FIXTURE-NAME"); err != nil {
			t.Fatalf("Rename(%s) error = %v", namespace, err)
		}
	}

	got := []string{<-requestedPaths, <-requestedPaths}
	want := []string{
		"/api/v4/computers-inventory-detail/shared-id",
		"/api/v2/mobile-devices/shared-id",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rename paths = %#v, want %#v", got, want)
	}
}
