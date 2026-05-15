// Package main provides the entry point for the skatd secret management server.
package main

import (
	"os"

	"github.com/nicklasfrahm-dev/appkit/logging"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/serve"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// version is injected at build time via -ldflags.
var version string

func rootCommand(logger *zap.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "skatd [command]",
		Short:        "skatd is a storage-agnostic secret management server.",
		Version:      version,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.PersistentFlags().BoolP("help", "h", false, "Show help for command")
	cmd.AddCommand(serve.RootCommand(logger))
	return cmd
}

func main() {
	format := os.Getenv("LOG_FORMAT")
	if format == "" {
		_ = os.Setenv("LOG_FORMAT", "console")
	}
	logger := logging.NewLogger()
	if err := rootCommand(logger).Execute(); err != nil {
		logger.Fatal("failed to execute command", zap.Error(err))
	}
}
