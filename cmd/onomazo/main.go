package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/woodleighschool/onomazo/internal/config"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	var configPath string
	command := &cobra.Command{
		Use:           "onomazo",
		Short:         "Reconcile managed device names",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.PersistentFlags().StringVar(&configPath, "config", "config.yaml", "path to the YAML configuration file")
	command.AddCommand(newValidateCommand(&configPath), newVersionCommand())
	return command
}

func newValidateCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration and naming expressions",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if _, err := config.Load(*configPath); err != nil {
				return fmt.Errorf("validate %s: %w", *configPath, err)
			}
			_, err := fmt.Fprintln(command.OutOrStdout(), "configuration valid")
			return err
		},
	}
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
