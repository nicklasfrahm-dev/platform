package main

import (
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// RootCommand returns the root command for the node profiler CLI.
func RootCommand(logger *zap.Logger) *cobra.Command {
	var help bool

	rootCmd := &cobra.Command{
		Use:   "nodeprofiler",
		Short: "A CLI to profile node scalability",
		Long:  `A command line interface to profile node scalability in Kubernetes.`,
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			if help {
				_ = cmd.Help()

				os.Exit(0)
			}
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = cmd.Help()

			return nil
		},
		Version:      version,
		SilenceUsage: true,
	}

	rootCmd.PersistentFlags().BoolVarP(&help, "help", "h", false, "display help for command")
	rootCmd.AddCommand(ScaleUpCommand(logger))

	return rootCmd
}
