package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/woodleighschool/onomazo/internal/app"
)

func TestDefaultConfigPathsUsesConfigInCurrentDirectory(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	if got := defaultConfigPaths(); got != nil {
		t.Fatalf("defaultConfigPaths() without config = %v, want nil", got)
	}
	writeCommandConfig(t, directory, "config.yaml", "version: 1\n")
	if got, want := defaultConfigPaths(), []string{"config.yaml"}; !slices.Equal(got, want) {
		t.Fatalf("defaultConfigPaths() = %v, want %v", got, want)
	}
}

func TestValidateAcceptsOrderedConfigurationFiles(t *testing.T) {
	directory := t.TempDir()
	basePath := writeCommandConfig(t, directory, "base.yaml", commandConfig)
	groupsPath := writeCommandConfig(t, directory, "groups.yaml", `identity:
  groups:
    staff: [staff-group]
`)
	overridesPath := writeCommandConfig(t, directory, "overrides.yaml", `naming:
  overrides:
    - name: excluded-device
      when: 'device.serial_number == "SITE-SERIAL"'
      exclude: true
`)

	command, _ := newRootCommand()
	command.SetArgs([]string{
		"validate",
		"--config", basePath,
		"--config", groupsPath,
		"--config", overridesPath,
	})
	var output bytes.Buffer
	command.SetOut(&output)

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := output.String(), "configuration valid\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func writeCommandConfig(t *testing.T, directory, name, contents string) string {
	t.Helper()

	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestReconciliationCommandsReportPolicyAndPartialSubmission(t *testing.T) {
	t.Setenv("ONOMAZO_STATE_TYPE", "memory")
	var submissions atomic.Int32
	var inventoryFails atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/oauth/token":
			_, _ = io.WriteString(response, `{"access_token":"fixture-token","expires_in":3600}`)
		case "/api/v4/computers-inventory":
			if inventoryFails.Load() {
				http.Error(response, "inventory unavailable", http.StatusServiceUnavailable)
				return
			}
			_, _ = io.WriteString(response, `{"totalCount":3,"results":[
				{"id":"1","general":{"name":"OLD-1"},"hardware":{"serialNumber":"1"}},
				{"id":"2","general":{"name":"OLD-2"},"hardware":{"serialNumber":"2"}},
				{"id":"3","general":{"name":"NEW-3"},"hardware":{"serialNumber":"3"}}
			]}`)
		case "/api/v4/computers-inventory-detail/1", "/api/v4/computers-inventory-detail/2":
			if request.Method != http.MethodPatch {
				t.Errorf("rename method = %s", request.Method)
			}
			submissions.Add(1)
			if strings.HasSuffix(request.URL.Path, "/2") {
				http.Error(response, "rename rejected", http.StatusBadRequest)
			} else {
				response.WriteHeader(http.StatusNoContent)
			}
		default:
			t.Errorf("unexpected request: %s", request.URL.Path)
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	path := writeCommandConfig(t, t.TempDir(), "config.yaml", fmt.Sprintf(`version: 1
connections:
  fixture:
    type: jamf
    url: %s
    client_id: fixture-client
    client_secret: fixture-secret
devices:
  - name: jamf
    type: jamf
    connection: fixture
    platforms: [macos]
naming:
  constraints:
    max_length: 15
    pattern: '^[A-Z0-9][A-Z0-9-]*$'
  rules:
    - name: serial
      when: 'true'
      desired_name: '"NEW-" + device.serial_number'
`, server.URL))

	for _, name := range []string{"plan", "apply"} {
		command, output := newRootCommand()
		var stdout, stderr bytes.Buffer
		command.SetOut(&stdout)
		command.SetErr(&stderr)
		command.SetArgs([]string{name, "--config", path, "--json"})
		executed, err := command.ExecuteC()
		output.finish(executed, err)
		if (err != nil) != (name == "apply") {
			t.Fatalf("%s error = %v", name, err)
		}
		var report reconciliationReport
		decoder := json.NewDecoder(&stdout)
		if err := decoder.Decode(&report); err != nil {
			t.Fatal(err)
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			t.Fatalf("trailing stdout: %v", err)
		}
		if report.Summary.Total != 3 || len(report.Devices) != 3 || report.Devices[0].CurrentName != "OLD-1" || report.Devices[0].DesiredName != "NEW-1" {
			t.Fatalf("%s report = %#v", name, report)
		}
		if name == "plan" {
			if submissions.Load() != 0 || report.Devices[0].Action != app.ActionPlanned || stderr.Len() != 0 {
				t.Fatalf("plan submitted requests or printed diagnostics: %s", stderr.String())
			}
		} else if submissions.Load() != 2 || report.Devices[0].Action != app.ActionSubmitted || report.Devices[1].Action != app.ActionFailed || report.Error == "" || !strings.Contains(stderr.String(), "rename rejected") || strings.Count(stderr.String(), "\n") != 1 {
			t.Fatalf("apply report=%#v stderr=%s", report, stderr.String())
		}
	}

	inventoryFails.Store(true)
	command, output := newRootCommand()
	var stdout, stderr bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	command.SetArgs([]string{"plan", "--config", path, "--json"})
	executed, err := command.ExecuteC()
	output.finish(executed, err)
	if err == nil || stdout.Len() != 0 || strings.Count(stderr.String(), "Error:") != 1 {
		t.Fatalf("inventory failure: error=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
}

const commandConfig = `version: 1
connections:
  microsoft:
    type: microsoft_graph
    tenant_id: tenant
    client_id: client
    client_secret: secret
devices:
  - name: intune
    type: intune
    connection: microsoft
    platforms: [macos]
identity:
  name: entra
  type: entra
  connection: microsoft
naming:
  constraints:
    max_length: 15
    pattern: '^[A-Z0-9][A-Z0-9-]*$'
  rules:
    - name: assigned-user
      when: user.present
      desired_name: slug(user.mail_nickname).upperAscii()
`
