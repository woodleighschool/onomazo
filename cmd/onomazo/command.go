package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/woodleighschool/onomazo/internal/app"
	"github.com/woodleighschool/onomazo/internal/config"
)

func newRootCommand() *cobra.Command {
	var configPaths []string
	command := &cobra.Command{
		Use:           "onomazo",
		Short:         "Reconcile managed device names",
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.PersistentFlags().StringArrayVar(
		&configPaths,
		"config",
		defaultConfigPaths(),
		"path to a YAML configuration file; may be repeated in overlay order",
	)
	command.AddCommand(
		newValidateCommand(&configPaths),
		newPlanCommand(&configPaths),
		newRunCommand(&configPaths),
		newSchemaCommand(),
		newVersionCommand(),
	)
	return command
}

func defaultConfigPaths() []string {
	info, err := os.Stat("config.yaml")
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	return []string{"config.yaml"}
}

func newValidateCommand(configPaths *[]string) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration and naming expressions",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if _, err := config.Load((*configPaths)...); err != nil {
				return fmt.Errorf("validate configuration: %w", err)
			}
			_, err := fmt.Fprintln(command.OutOrStdout(), "configuration valid")
			return err
		},
	}
}

func newPlanCommand(configPaths *[]string) *cobra.Command {
	var output string
	command := &cobra.Command{
		Use:   "plan",
		Short: "Fetch inventory and print a read-only naming plan",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if output != "human" && output != "json" {
				return fmt.Errorf("output must be human or json")
			}
			cfg, err := config.Load((*configPaths)...)
			if err != nil {
				return fmt.Errorf("load configuration: %w", err)
			}
			service, err := app.Build(cfg, app.BuildReadOnly)
			if err != nil {
				return fmt.Errorf("start onomazo: %w", err)
			}
			results, err := service.Reconcile(command.Context(), false)
			if err != nil {
				return errors.Join(err, service.Close())
			}
			return errors.Join(writePlan(command.OutOrStdout(), output, results), service.Close())
		},
	}
	command.Flags().StringVar(&output, "output", "human", "plan output format: human or json")
	return command
}

func newRunCommand(configPaths *[]string) *cobra.Command {
	var once bool
	var logLevel string
	command := &cobra.Command{
		Use:   "run",
		Short: "Reconcile names immediately, then continue at the configured interval",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			level, err := parseLogLevel(logLevel)
			if err != nil {
				return err
			}
			cfg, err := config.Load((*configPaths)...)
			if err != nil {
				return fmt.Errorf("load configuration: %w", err)
			}
			service, err := app.Build(cfg, app.BuildApply)
			if err != nil {
				return fmt.Errorf("start onomazo: %w", err)
			}
			logger := slog.New(slog.NewJSONHandler(command.ErrOrStderr(), &slog.HandlerOptions{Level: level}))
			logger.Info("Onomazo started", "version", version, "config", *configPaths, "once", once)
			if once {
				return errors.Join(runCycle(command.Context(), service, logger, true), service.Close())
			}
			runLoop(command.Context(), cfg.Reconcile.PollInterval.Duration, service, logger)
			return service.Close()
		},
	}
	command.Flags().BoolVar(&once, "once", false, "run one reconciliation cycle and exit")
	command.Flags().StringVar(&logLevel, "log-level", "info", "log level: debug, info, warn, or error")
	return command
}

func newSchemaCommand() *cobra.Command {
	var outputPath string
	command := &cobra.Command{
		Use:   "schema",
		Short: "Generate the JSON Schema used by YAML editors",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			document, err := config.JSONSchemaDocument()
			if err != nil {
				return fmt.Errorf("generate config schema: %w", err)
			}
			if outputPath == "-" {
				_, err = command.OutOrStdout().Write(document)
				return err
			}
			if err := os.WriteFile(outputPath, document, 0o644); err != nil {
				return fmt.Errorf("write config schema: %w", err)
			}
			return nil
		},
	}
	command.Flags().StringVar(&outputPath, "output", "-", "schema output path, or - for stdout")
	return command
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(command.OutOrStdout(), "onomazo %s\ncommit: %s\nbuilt: %s\n", version, commit, date)
			return err
		},
	}
}

type reconciler interface {
	Reconcile(context.Context, bool) ([]app.Result, error)
}

func runLoop(ctx context.Context, interval time.Duration, service reconciler, logger *slog.Logger) {
	initialCycle := true
	for {
		_ = runCycle(ctx, service, logger, initialCycle)
		initialCycle = false
		if contextStopped(ctx) {
			return
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-timer.C:
		}
	}
}

func contextStopped(ctx context.Context) bool {
	return ctx.Err() != nil
}

func runCycle(ctx context.Context, service reconciler, logger *slog.Logger, initialCycle bool) error {
	started := time.Now()
	results, err := service.Reconcile(ctx, true)
	for _, result := range results {
		logResult(logger, result, initialCycle)
	}
	attributes := []any{
		"devices", len(results),
		"renames_submitted", countAction(results, app.ActionSubmitted),
		"renames_pending", countAction(results, app.ActionPending),
		"duration", time.Since(started),
	}
	if err != nil {
		logger.ErrorContext(ctx, "reconciliation failed", append(attributes, "error", err)...)
	} else {
		logger.InfoContext(ctx, "reconciliation complete", attributes...)
	}
	return err
}

func parseLogLevel(value string) (slog.Level, error) {
	switch strings.ToLower(value) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("log level must be debug, info, warn, or error")
	}
}
