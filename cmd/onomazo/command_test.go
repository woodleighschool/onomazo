package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/woodleighschool/onomazo/internal/app"
)

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

func TestRunLoopCancelsInFlightReconciliation(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	service := blockingReconciler{started: started}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runLoop(ctx, time.Hour, service, slog.New(slog.DiscardHandler))
		close(done)
	}()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runLoop() did not stop after cancellation")
	}
}

type blockingReconciler struct {
	started chan<- struct{}
}

func (r blockingReconciler) Reconcile(ctx context.Context, _ bool) ([]app.Result, error) {
	close(r.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

func writeCommandConfig(t *testing.T, directory, name, contents string) string {
	t.Helper()

	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
