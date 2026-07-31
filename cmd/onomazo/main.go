package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/woodleighschool/onomazo/internal/app"
	"github.com/woodleighschool/onomazo/internal/config"
	"github.com/woodleighschool/onomazo/internal/intune"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		slog.Error("Command execution failed", "error", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "onomazo",
		Short: "Reconcile managed device names",
		Long:  "Onomazo reconciles managed device names from configured device and identity data.",

		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOnomazo()
		},
	}

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	flags := cmd.Flags()
	flags.String("config", "config.yaml", "path to the YAML configuration file")
	flags.Bool("once", false, "run a single sync and exit")
	flags.String("poll-interval", "5m", "interval between sync runs in continuous mode")
	flags.String("log-level", "info", "logging level: debug, info, warn, error")
	flags.Bool("dry-run", false, "log rename actions without applying them")
	flags.Int("max-name-length", 63, "maximum allowed device name length")
	flags.String("graph-base-url", "https://graph.microsoft.com/v1.0", "Microsoft Graph base URL")
	flags.String("tenant-id", "", "Azure AD tenant ID (can also use TENANT_ID env var)")
	flags.String("client-id", "", "Azure AD application client ID (can also use CLIENT_ID env var)")
	flags.String("client-secret", "", "Azure AD application client secret (can also use CLIENT_SECRET env var)")

	if err := viper.BindPFlag("config", flags.Lookup("config")); err != nil {
		panic(fmt.Sprintf("failed to bind config flag: %v", err))
	}
	if err := viper.BindPFlag("once", flags.Lookup("once")); err != nil {
		panic(fmt.Sprintf("failed to bind once flag: %v", err))
	}
	if err := viper.BindPFlag("poll_interval", flags.Lookup("poll-interval")); err != nil {
		panic(fmt.Sprintf("failed to bind poll-interval flag: %v", err))
	}
	if err := viper.BindPFlag("log_level", flags.Lookup("log-level")); err != nil {
		panic(fmt.Sprintf("failed to bind log-level flag: %v", err))
	}
	if err := viper.BindPFlag("dry_run", flags.Lookup("dry-run")); err != nil {
		panic(fmt.Sprintf("failed to bind dry-run flag: %v", err))
	}
	if err := viper.BindPFlag("max_name_length", flags.Lookup("max-name-length")); err != nil {
		panic(fmt.Sprintf("failed to bind max-name-length flag: %v", err))
	}
	if err := viper.BindPFlag("tenant_id", flags.Lookup("tenant-id")); err != nil {
		panic(fmt.Sprintf("failed to bind tenant-id flag: %v", err))
	}
	if err := viper.BindPFlag("client_id", flags.Lookup("client-id")); err != nil {
		panic(fmt.Sprintf("failed to bind client-id flag: %v", err))
	}
	if err := viper.BindPFlag("client_secret", flags.Lookup("client-secret")); err != nil {
		panic(fmt.Sprintf("failed to bind client-secret flag: %v", err))
	}
	if err := viper.BindPFlag("graph_base_url", flags.Lookup("graph-base-url")); err != nil {
		panic(fmt.Sprintf("failed to bind graph-base-url flag: %v", err))
	}

	cmd.AddCommand(newVersionCmd())

	return cmd
}

func runOnomazo() error {
	appCfg, err := config.LoadAppConfig()
	if err != nil {
		return fmt.Errorf("failed to load app config: %w", err)
	}

	cfg, err := config.Load(appCfg.ConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config %s: %w", appCfg.ConfigPath, err)
	}

	opts, err := buildOptions()
	if err != nil {
		return err
	}

	logger := setupLogging(viper.GetString("log_level"))

	logger.Info("onomazo starting",
		"version", version,
		"log_level", viper.GetString("log_level"),
		"config_path", appCfg.ConfigPath,
		"oneshot_mode", viper.GetBool("once"))

	service, err := app.New(cfg, opts, logger)
	if err != nil {
		return fmt.Errorf("failed to initialise service: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if viper.GetBool("once") {
		logger.Info("Running single sync cycle (oneshot mode)")
		if err := service.RunOnce(ctx); err != nil {
			return fmt.Errorf("sync failed: %w", err)
		}
		return nil
	}

	logger.Info("Starting continuous sync mode", "poll_interval", viper.GetString("poll_interval"))

	if err := service.Run(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			logger.Info("Shutdown signal received, stopping...")
			return nil
		}
		return fmt.Errorf("service run failed: %w", err)
	}

	return nil
}

func buildOptions() (*app.Options, error) {
	pollInterval, err := time.ParseDuration(viper.GetString("poll_interval"))
	if err != nil {
		return nil, fmt.Errorf("invalid poll interval: %w", err)
	}
	if pollInterval <= 0 {
		return nil, fmt.Errorf("poll interval must be greater than zero")
	}

	maxLen := viper.GetInt("max_name_length")
	if maxLen <= 0 {
		return nil, fmt.Errorf("max-name-length must be a positive integer")
	}

	creds := intune.Credentials{
		TenantID:     strings.TrimSpace(viper.GetString("tenant_id")),
		ClientID:     strings.TrimSpace(viper.GetString("client_id")),
		ClientSecret: strings.TrimSpace(viper.GetString("client_secret")),
	}
	if creds.TenantID == "" {
		return nil, fmt.Errorf("tenant-id is required (TENANT_ID)")
	}
	if creds.ClientID == "" {
		return nil, fmt.Errorf("client-id is required (CLIENT_ID)")
	}
	if creds.ClientSecret == "" {
		return nil, fmt.Errorf("client-secret is required (CLIENT_SECRET)")
	}

	return &app.Options{
		PollInterval:     pollInterval,
		DryRun:           viper.GetBool("dry_run"),
		MaxDeviceNameLen: maxLen,
		GraphBaseURL:     strings.TrimSpace(viper.GetString("graph_base_url")),
		Credentials:      creds,
	}, nil
}

func setupLogging(level string) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: parseLevel(level),
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	logger := slog.New(handler)

	slog.SetDefault(logger)

	return logger
}

func parseLevel(level string) slog.Leveler {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("onomazo %s\n", version)
			fmt.Printf("commit: %s\n", commit)
			fmt.Printf("built: %s\n", date)
		},
	}
}
