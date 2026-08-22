package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
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
	basePath := writeCommandConfig(t, directory, "base.yaml", `version: 1
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
`)
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

	command := newRootCommand()
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
