package main

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProgressShowsOnlyUnfinishedOperations(t *testing.T) {
	var out bytes.Buffer
	p := newTerminalProgress(&out)
	p.update(activity{label: "Reading inventory", stage: true})
	p.update(activity{label: "Resolving identities", stage: true})
	if got := p.view(time.Now(), 80, 20); got != "" {
		t.Fatalf("fast work flashed: %s", got)
	}
	p.update(activity{label: "Reading inventory", status: true})
	p.update(activity{label: "Resolving identities", progress: true, current: 2, total: 4, unit: "identities"})
	got := p.view(time.Now().Add(time.Second), 80, 20)
	if strings.Contains(got, "Reading") || !strings.Contains(got, "2 / 4 identities") {
		t.Fatal(got)
	}
	p.update(activity{label: "Resolving identities", status: true})
	if got := p.view(time.Now().Add(time.Second), 80, 20); got != "" {
		t.Fatal(got)
	}
	p.stop()
	if out.Len() != 0 {
		t.Fatalf("completed activity became output: %s", out.String())
	}
}

func TestProgressRequiresHumanTerminalOutput(t *testing.T) {
	terminal, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		t.Skip("requires a controlling terminal")
	}
	t.Cleanup(func() { _ = terminal.Close() })
	t.Setenv("TERM", "xterm")
	t.Setenv("CI", "")
	for _, test := range []struct {
		name, command              string
		json, stdout, stderr, want bool
	}{
		{name: "human", command: "plan", stdout: true, stderr: true, want: true},
		{name: "json", command: "plan", json: true, stdout: true, stderr: true},
		{name: "stdout pipe", command: "plan", stderr: true, want: true},
		{name: "stderr pipe", command: "plan", stdout: true},
		{name: "both pipes", command: "plan"},
		{name: "daemon", command: "run", stdout: true, stderr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, output := newRootCommand()
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			if test.stdout {
				root.SetOut(terminal)
			}
			if test.stderr {
				root.SetErr(terminal)
			}
			command, _, err := root.Find([]string{test.command})
			if err != nil {
				t.Fatal(err)
			}
			if test.json {
				if err := command.Flags().Set("json", "true"); err != nil {
					t.Fatal(err)
				}
			}
			if err := output.start(command); err != nil {
				t.Fatal(err)
			}
			if output.interactive != test.want {
				t.Fatalf("interactive = %t, want %t", output.interactive, test.want)
			}
			output.stop()
		})
	}
}

func TestConcurrentStagesKeepWarnings(t *testing.T) {
	var output bytes.Buffer
	command := &commandOutput{out: &output, interactive: true}
	logger := slog.New(&activityHandler{Handler: slog.NewJSONHandler(&output, nil), output: command})
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			logger.Info("Fetching inventory", "stage", true)
			logger.Info("Fetching inventory", "progress", true, "current", 1, "total", 2, "unit", "sources")
			logger.Info("Fetching inventory", "stage_result", true, "error", errors.New("failed"))
			logger.Warn("Provider notice")
			command.stop()
		})
	}
	workers.Wait()
	command.stop()
	if got, want := output.String(), strings.Repeat("! Provider notice\n", 4); got != want {
		t.Fatalf("persistent progress or lost warnings: %q", got)
	}
}

func TestPlainProgressIsSparseAndEndsWithWork(t *testing.T) {
	var out bytes.Buffer
	p := newTerminalProgress(&out)
	p.plain = true
	p.update(activity{label: "Reading inventory", stage: true})
	now := time.Now()
	p.draw(now)
	if out.Len() != 0 {
		t.Fatal("fast work printed")
	}
	p.draw(now.Add(3 * time.Second))
	first := out.String()
	if !strings.Contains(first, "Reading inventory") || strings.Contains(first, "\x1b") {
		t.Fatalf("milestone: %q", first)
	}
	p.draw(now.Add(4 * time.Second))
	if out.String() != first {
		t.Fatal("duplicate milestone")
	}
	p.update(activity{label: "Reading inventory", status: true})
	p.draw(now.Add(time.Minute))
	p.stop()
	if out.String() != first {
		t.Fatal("completed work printed")
	}
}
