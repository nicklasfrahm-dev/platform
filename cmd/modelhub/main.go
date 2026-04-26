// Package main is the entry point for modelhub, a CLI to manage ML models in Ceph S3 storage.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

// globalConfig holds the flags shared across all subcommands.
type globalConfig struct {
	kubeContext string
	namespace   string
	endpoint    string
}

func main() {
	err := newRootCmd().Execute()
	if err != nil {
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

func newLsCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "ls [model]",
		Short: "List models or versions in S3 storage.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			return runLs(ctx, cfg, args)
		},
	}
}

func runLs(ctx context.Context, cfg *globalConfig, args []string) error {
	creds, err := ensureOBC(ctx, cfg)
	if err != nil {
		return fmt.Errorf("ensure OBC: %w", err)
	}

	s3Client := newS3Client(cfg.endpoint, creds.accessKey, creds.secretKey)

	if len(args) == 0 {
		// List all models
		models, err := listModels(ctx, s3Client, creds.bucketName)
		if err != nil {
			return fmt.Errorf("list models: %w", err)
		}

		for _, model := range models {
			log.Printf("%s\n", model)
		}
	} else {
		// List versions for a specific model
		modelName := args[0]

		versions, err := listVersions(ctx, s3Client, creds.bucketName, modelName)
		if err != nil {
			return fmt.Errorf("list versions: %w", err)
		}

		for _, version := range versions {
			log.Printf("%s\n", version)
		}
	}

	return nil
}

const maxArgs = 2

func newRmCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <model> [version]",
		Short: "Remove a model or version from S3 storage.",
		Args:  cobra.RangeArgs(1, maxArgs),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			return runRm(ctx, cfg, args)
		},
	}
}

func runRm(ctx context.Context, cfg *globalConfig, args []string) error {
	creds, err := ensureOBC(ctx, cfg)
	if err != nil {
		return fmt.Errorf("ensure OBC: %w", err)
	}

	s3Client := newS3Client(cfg.endpoint, creds.accessKey, creds.secretKey)

	modelName := args[0]

	var prefix string
	if len(args) == 1 {
		// Remove entire model
		prefix = modelName + "/"
	} else {
		// Remove specific version
		version := args[1]
		prefix = fmt.Sprintf("%s/%s/", modelName, version)
	}

	deleted, err := deletePrefix(ctx, s3Client, creds.bucketName, prefix)
	if err != nil {
		return fmt.Errorf("delete objects: %w", err)
	}

	log.Printf("Deleted %d objects\n", deleted)

	return nil
}
