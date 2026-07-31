package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadValidConfig(t *testing.T) {
	t.Parallel()

	config, err := loadConfig(t, validConfig, map[string]string{
		"TENANT_ID":     "tenant",
		"CLIENT_ID":     "client",
		"CLIENT_SECRET": "secret:with#yaml",
		"STAFF_GROUP":   "staff-group",
	})
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}

	if got, want := config.Connections["microsoft"].ClientSecret, "secret:with#yaml"; got != want {
		t.Errorf("client secret = %q, want %q", got, want)
	}
	if got, want := config.Reconcile.PollInterval.Duration, time.Minute; got != want {
		t.Errorf("poll interval = %v, want %v", got, want)
	}
	if got, want := config.State.Type, "memory"; got != want {
		t.Errorf("state type = %q, want %q", got, want)
	}
	if got, want := len(config.Programs.Rules), 1; got != want {
		t.Errorf("compiled rules = %d, want %d", got, want)
	}
	if got, want := len(config.Programs.Ranks), 1; got != want {
		t.Errorf("compiled ranks = %d, want %d", got, want)
	}
}

func TestLoadSupportsIntuneAndJamf(t *testing.T) {
	t.Parallel()

	contents := strings.Replace(validConfig, "    client_secret: ${CLIENT_SECRET}\n", `    client_secret: ${CLIENT_SECRET}
  jamf:
    type: jamf
    url: ${JAMF_URL}
    client_id: ${JAMF_CLIENT_ID}
    client_secret: ${JAMF_CLIENT_SECRET}

`, 1)
	contents = strings.Replace(contents, "    platforms: [macos, ios, windows]\n", `    platforms: [macos, ios, windows]
  - name: jamf
    type: jamf
    connection: jamf
    platforms: [macos, ios]

`, 1)

	config, err := loadConfig(t, contents, map[string]string{
		"TENANT_ID":          "tenant",
		"CLIENT_ID":          "client",
		"CLIENT_SECRET":      "secret",
		"STAFF_GROUP":        "staff-group",
		"JAMF_URL":           "https://example.jamfcloud.com",
		"JAMF_CLIENT_ID":     "jamf-client",
		"JAMF_CLIENT_SECRET": "jamf-secret",
	})
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if got, want := len(config.Devices), 2; got != want {
		t.Errorf("device sources = %d, want %d", got, want)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	t.Parallel()

	contents := strings.Replace(validConfig, "version: 1", "version: 1\nunknown: true", 1)
	_, err := loadConfig(t, contents, standardEnvironment())
	if err == nil {
		t.Fatal("load() error = nil, want unknown field error")
	}
	if !strings.Contains(err.Error(), `field unknown not found`) {
		t.Errorf("load() error = %q, want unknown field error", err)
	}
}

func TestLoadRejectsMissingEnvironmentVariable(t *testing.T) {
	t.Parallel()

	_, err := loadConfig(t, validConfig, map[string]string{})
	if err == nil {
		t.Fatal("load() error = nil, want missing environment error")
	}
	if got, want := err.Error(), "environment variable TENANT_ID is not set"; got != want {
		t.Errorf("load() error = %q, want %q", got, want)
	}
}

func TestLoadRejectsInlineEnvironmentInterpolation(t *testing.T) {
	t.Parallel()

	contents := strings.Replace(validConfig, "${CLIENT_ID}", "client-${CLIENT_ID}", 1)
	_, err := loadConfig(t, contents, standardEnvironment())
	if err == nil {
		t.Fatal("load() error = nil, want interpolation error")
	}
	if got, want := err.Error(), "environment placeholders must occupy an entire YAML scalar"; got != want {
		t.Errorf("load() error = %q, want %q", got, want)
	}
}

func TestLoadTypeChecksCELFields(t *testing.T) {
	t.Parallel()

	contents := strings.Replace(validConfig, "device.enrolled_at", "device.enroled_at", 1)
	_, err := loadConfig(t, contents, standardEnvironment())
	if err == nil {
		t.Fatal("load() error = nil, want CEL field error")
	}
	if !strings.Contains(err.Error(), "undefined field 'enroled_at'") {
		t.Errorf("load() error = %q, want undefined field", err)
	}
}

func TestLoadPreventsVariablesInOverrides(t *testing.T) {
	t.Parallel()

	contents := strings.Replace(validConfig, `device.serial_number == "EXAMPLE-SERIAL"`, `"role" in vars`, 1)
	_, err := loadConfig(t, contents, standardEnvironment())
	if err == nil {
		t.Fatal("load() error = nil, want undeclared vars error")
	}
	if !strings.Contains(err.Error(), "undeclared reference to 'vars'") {
		t.Errorf("load() error = %q, want undeclared vars error", err)
	}
}

func loadConfig(t *testing.T, contents string, environment map[string]string) (*Config, error) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return load(path, func(name string) (string, bool) {
		value, ok := environment[name]
		return value, ok
	})
}

func standardEnvironment() map[string]string {
	return map[string]string{
		"TENANT_ID":     "tenant",
		"CLIENT_ID":     "client",
		"CLIENT_SECRET": "secret",
		"STAFF_GROUP":   "staff-group",
	}
}

const validConfig = `version: 1

connections:
  microsoft:
    type: microsoft_graph
    tenant_id: ${TENANT_ID}
    client_id: ${CLIENT_ID}
    client_secret: ${CLIENT_SECRET}

devices:
  - name: intune
    type: intune
    connection: microsoft
    platforms: [macos, ios, windows]

identity:
  name: entra
  type: entra
  connection: microsoft
  groups:
    staff:
      - ${STAFF_GROUP}

naming:
  constraints:
    max_length: 15
    pattern: '^[A-Z0-9][A-Z0-9-]*$'
  overrides:
    - name: leave-example-device-alone
      when: 'device.serial_number == "EXAMPLE-SERIAL"'
      exclude: true
  variables:
    role:
      cases:
        - when: '"staff" in user.groups'
          value: EMP
  rules:
    - name: assigned-user
      when: 'user.present && "role" in vars && user.mail_nickname != ""'
      desired_name: '(vars["role"] + "-" + slug(user.mail_nickname)).upperAscii()'
  collisions:
    rank:
      - expression: device.enrolled_at
        order: ascending
    disambiguate:
      type: sequence
      separator: '-'
      preserve_existing: true
`
