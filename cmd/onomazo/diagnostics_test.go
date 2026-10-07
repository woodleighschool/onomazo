package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestFiniteCommandsKeepWarningsAndDiscardProgress(t *testing.T) {
	t.Setenv("ONOMAZO_LOG_LEVEL", "error")
	path := writeCommandConfig(t, t.TempDir(), "config.yaml", commandConfig)
	command, output := newRootCommand()
	var report, diagnostics bytes.Buffer
	command.SetOut(&report)
	command.SetErr(&diagnostics)
	command.AddCommand(&cobra.Command{Use: "probe", RunE: func(cmd *cobra.Command, _ []string) error {
		if _, err := (&cli{configPaths: []string{path}, output: output}).loadConfig(cmd); err != nil {
			return err
		}
		output.logger.Info("Fetching inventory", "stage", true)
		output.logger.Info("Fetching inventory", "progress", true, "current", 1, "total", 2, "unit", "sources")
		output.logger.Info("Fetching inventory", "stage_result", true)
		output.logger.Debug("Request detail")
		output.logger.Warn("Provider notice", "source", "fixture")
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "final report")
		return err
	}})
	command.SetArgs([]string{"probe"})
	executed, err := command.ExecuteC()
	output.finish(executed, err)
	if err != nil {
		t.Fatal(err)
	}
	if report.String() != "final report\n" || diagnostics.String() != "! Provider notice; source=fixture\n" {
		t.Fatalf("stdout=%q stderr=%q", report.String(), diagnostics.String())
	}
}

func TestDaemonUsesJSONAndConfiguredLevel(t *testing.T) {
	for _, override := range []bool{false, true} {
		t.Run(fmt.Sprint(override), func(t *testing.T) {
			t.Setenv("ONOMAZO_LOG_LEVEL", "warn")
			path := writeCommandConfig(t, t.TempDir(), "config.yaml", commandConfig)
			command, output := newRootCommand()
			var diagnostics, report bytes.Buffer
			command.SetOut(&report)
			command.SetErr(&diagnostics)
			run, _, err := command.Find([]string{"run"})
			if err != nil {
				t.Fatal(err)
			}
			run.RunE = func(cmd *cobra.Command, _ []string) error {
				if _, err := (&cli{configPaths: []string{path}, output: output}).loadConfig(cmd); err != nil {
					return err
				}
				output.logger.Info("Fetching inventory", "stage", true)
				output.logger.Info("Device changed")
				output.logger.Warn("Provider notice")
				return nil
			}
			args := []string{"run"}
			if override {
				args = append(args, "--log-level", "debug")
			}
			command.SetArgs(args)
			executed, err := command.ExecuteC()
			output.finish(executed, err)
			if err != nil || report.Len() != 0 {
				t.Fatalf("error=%v stdout=%q", err, report.String())
			}
			if strings.Contains(diagnostics.String(), "Device changed") != override || strings.Contains(diagnostics.String(), "Fetching inventory") != override {
				t.Fatalf("configured diagnostics: %s", diagnostics.String())
			}
			decoder := json.NewDecoder(&diagnostics)
			for {
				var record struct {
					Message string `json:"msg"`
					Level   string `json:"level"`
				}
				if err := decoder.Decode(&record); errors.Is(err, io.EOF) {
					break
				} else if err != nil {
					t.Fatal(err)
				}
				if record.Message == "Fetching inventory" && record.Level != "DEBUG" {
					t.Fatalf("stage level = %s", record.Level)
				}
			}
		})
	}
}

func TestRemovedOutputFlagsNeverExecute(t *testing.T) {
	for _, name := range []string{"plan", "apply", "validate", "schema", "version", "run"} {
		flags := [][]string{{"--output", "json"}, {"--log-format", "text"}, {"--quiet"}, {"--verbose"}, {"--debug"}, {"-q"}, {"-v"}, {"-d"}}
		if name != "run" {
			flags = append(flags, []string{"--log-level", "debug"})
		} else {
			flags = append(flags, []string{"--json"}, []string{"--log-level", "trace"})
		}
		for _, flag := range flags {
			command, output := newRootCommand()
			command.SetOut(io.Discard)
			command.SetErr(io.Discard)
			child, _, err := command.Find([]string{name})
			if err != nil {
				t.Fatal(err)
			}
			called := false
			child.RunE = func(*cobra.Command, []string) error { called = true; return nil }
			command.SetArgs(append([]string{name}, flag...))
			executed, err := command.ExecuteC()
			output.finish(executed, err)
			if err == nil || called {
				t.Fatalf("%s %v: executed=%t error=%v", name, flag, called, err)
			}
		}
	}
}

func TestStartupFailureHasNoReport(t *testing.T) {
	for _, name := range []string{"plan", "apply"} {
		command, output := newRootCommand()
		var report, diagnostics bytes.Buffer
		command.SetOut(&report)
		command.SetErr(&diagnostics)
		command.SetArgs([]string{name, "--config", t.TempDir() + "/missing.yaml", "--json"})
		executed, err := command.ExecuteC()
		output.finish(executed, err)
		if err == nil || report.Len() != 0 || strings.Count(diagnostics.String(), "✗") != 1 || strings.Count(diagnostics.String(), "\n") != 1 {
			t.Fatalf("error=%v stdout=%q stderr=%q", err, report.String(), diagnostics.String())
		}
	}
}

func TestDaemonHelpDoesNotReportServiceStopped(t *testing.T) {
	command, output := newRootCommand()
	var report, diagnostics bytes.Buffer
	command.SetOut(&report)
	command.SetErr(&diagnostics)
	command.SetArgs([]string{"run", "--help"})
	executed, err := command.ExecuteC()
	output.finish(executed, err)
	if err != nil || !strings.Contains(report.String(), "Usage:") || diagnostics.Len() != 0 {
		t.Fatalf("help error=%v stdout=%q stderr=%q", err, report.String(), diagnostics.String())
	}
}

func TestCancellationIsNotAutomaticallyAnInterrupt(t *testing.T) {
	for _, interrupted := range []bool{false, true} {
		cmd, output := newRootCommand()
		var logs bytes.Buffer
		cmd.SetErr(&logs)
		ctx, cancel := context.WithCancelCause(t.Context())
		defer cancel(nil)
		cmd.SetContext(ctx)
		if interrupted {
			cancel(errInterrupted)
		}
		output.finish(cmd, context.Canceled)
		if strings.Contains(logs.String(), "– Interrupted.") != interrupted {
			t.Fatalf("interrupted=%v: %s", interrupted, logs.String())
		}
	}
}
