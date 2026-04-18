package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nicklasfrahm-dev/platform/cmd/nodeprofiler/scaleup"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var errMissingDurationArg = errors.New("failed to fetch duration argument")

// ScaleUpCommand returns a cobra command to benchmark node scaling.
func ScaleUpCommand(_ *zap.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scaleup <duration>",
		Short: "Benchmark node scaling in Kubernetes",
		Long: `This command waits for the creation of the first node.
Upon creation, it will log all node events, pod events
on the node and the node conditions.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			logger, err := setupLogger()
			if err != nil {
				return err
			}

			if len(args) < 1 {
				return errMissingDurationArg
			}

			timeout, err := time.ParseDuration(args[0])
			if err != nil {
				return fmt.Errorf("failed to parse duration argument: %w", err)
			}

			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

			ctx, cancelSignal := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer cancelSignal()

			go func() {
				sig := <-sigs
				logger.Info("Received signal", zap.String("signal", sig.String()))
			}()

			profiler, err := scaleup.NewNodeProfiler(logger)
			if err != nil {
				return fmt.Errorf("failed to create node profiler: %w", err)
			}

			ctx, cancelDeadline := context.WithDeadline(ctx, time.Now().Add(timeout))
			defer cancelDeadline()

			err = profiler.Run(ctx)
			if err != nil {
				return fmt.Errorf("failed to run node profiler: %w", err)
			}

			err = profiler.Print(context.Background())
			if err != nil {
				return fmt.Errorf("failed to print node profiler results: %w", err)
			}

			return nil
		},
	}

	return cmd
}
