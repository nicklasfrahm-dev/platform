// Package main provides the entry point for the cloud CLI.
package main

import (
	"os"

	"github.com/nicklasfrahm-dev/appkit/logging"
	"github.com/nicklasfrahm-dev/platform/cmd/cloud/service"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// version holds the version of the application. It is injected at build time.
var version string

func RootCommand(logger *zap.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cloud [command]",
		Short:   "cloud manages services in my infrastructure.",
		Long:    `cloud is a command line tool to manage services in my infrastructure.`,
		Version: version,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	// Add global flags.
	cmd.PersistentFlags().BoolP("help", "h", false, "Show help for command")

	// Add subcommands.
	cmd.AddCommand(service.RootCommand(logger))

	return cmd
}

func main() {
	// Default to console logger.
	format := os.Getenv("LOG_FORMAT")
	if format == "" {
		_ = os.Setenv("LOG_FORMAT", "console")
	}

	logger := logging.NewLogger()

	err := RootCommand(logger).Execute()
	if err != nil {
		logger.Fatal("Failed to execute command", zap.Error(err))
	}
}
