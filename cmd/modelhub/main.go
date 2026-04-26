// Package main is the entry point for modelhub, a CLI to manage ML models in Ceph S3 storage.
package main

import (
	"os"

	"github.com/spf13/cobra"
)

// globalConfig holds the flags shared across all subcommands.
type globalConfig struct {
	kubeContext string
	namespace   string
	endpoint    string
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var cfg globalConfig

	root := &cobra.Command{
		Use:   "modelhub",
		Short: "Manage ML models in Ceph S3 storage.",
	}

	root.PersistentFlags().StringVar(&cfg.kubeContext, "context", "admin@cph02", "kubectl context")
	root.PersistentFlags().StringVar(&cfg.namespace, "namespace", "llm", "Kubernetes namespace")
	root.PersistentFlags().StringVar(&cfg.endpoint, "endpoint", "https://s3.cph02.nicklasfrahm.dev", "S3 endpoint URL")

	root.AddCommand(newAddCmd(&cfg))
	root.AddCommand(newLsCmd(&cfg))
	root.AddCommand(newRmCmd(&cfg))

	return root
}
