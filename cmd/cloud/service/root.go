package service

import (
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// RootCommand returns the root command for the service CLI.
func RootCommand(logger *zap.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service [command]",
		Short: "Manage the lifecycle of a service",
		Long:  `Manage the lifecycle of a service.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(Bootstrap(logger))

	return cmd
}
