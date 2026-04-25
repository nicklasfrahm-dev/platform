// Package main is the entry point for vllm-bench, a token throughput benchmark
// for vLLM deployments running in Kubernetes.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var (
	errNoPod              = errors.New("no running pod found")
	errPortForwardTimeout = errors.New("port-forward did not become ready within 30s")
)

const (
	defaultSelector  = "serving.kserve.io/inferenceservice=llm"
	defaultPrompt    = "Explain in detail how transformer attention mechanisms work in large language models."
	defaultSweepSizes = "1024,4096,16384,32768,65536,131072"
)

type config struct {
	duration     time.Duration
	workers      int
	namespace    string
	selector     string
	model        string
	maxTokens    int
	maxQueue     int
	port         int
	prompt       string
	sweepSizes   string
	sweepReqs    int
	sweepOutToks int
	client       *http.Client
}

func main() {
	err := newRootCmd().Execute()
	if err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var cfg config

	root := &cobra.Command{
		Use:   "vllm-bench",
		Short: "Token throughput benchmark for vLLM deployments running in Kubernetes",
	}

	root.PersistentFlags().StringVar(&cfg.namespace, "namespace", "llm", "Kubernetes namespace")
	root.PersistentFlags().StringVar(&cfg.selector, "selector", defaultSelector, "pod label selector")
	root.PersistentFlags().StringVar(&cfg.model, "model", "gemma4-26b", "served model name")
	root.PersistentFlags().IntVar(&cfg.maxQueue, "max-queue", defaultMaxQueue,
		"pause worker when vLLM waiting queue reaches this depth")
	root.PersistentFlags().IntVar(&cfg.port, "port", defaultPort, "local port for kubectl port-forward")

	root.AddCommand(newRunCmd(&cfg))
	root.AddCommand(newSweepCmd(&cfg))

	return root
}

func newRunCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a sustained throughput benchmark",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runWithPortForward(cfg, func(ctx context.Context, baseURL string) {
				runBench(ctx, baseURL, *cfg)
			})
		},
	}

	cmd.Flags().DurationVar(&cfg.duration, "duration", defaultDuration, "benchmark duration")
	cmd.Flags().IntVar(&cfg.workers, "workers", 1, "concurrent workers; each sends requests sequentially")
	cmd.Flags().IntVar(&cfg.maxTokens, "max-tokens", defaultMaxTokens, "max completion tokens per request")
	cmd.Flags().StringVar(&cfg.prompt, "prompt", defaultPrompt, "prompt text")

	return cmd
}

func newSweepCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sweep",
		Short: "Sweep context sizes and plot throughput vs latency",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runWithPortForward(cfg, func(ctx context.Context, baseURL string) {
				sizes := parseSweepSizes(cfg.sweepSizes)
				runSweep(ctx, baseURL, *cfg, sizes)
			})
		},
	}

	cmd.Flags().StringVar(&cfg.sweepSizes, "sizes", defaultSweepSizes, "comma-separated input token targets")
	cmd.Flags().IntVar(&cfg.sweepReqs, "requests", defaultSweepReqs, "requests per context size (averaged)")
	cmd.Flags().IntVar(&cfg.sweepOutToks, "out-tokens", defaultSweepOutToks, "output tokens per request")

	return cmd
}

func runWithPortForward(cfg *config, bench func(ctx context.Context, baseURL string)) error {
	cfg.client = &http.Client{Timeout: clientTimeout}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pod, err := findPod(ctx, cfg.namespace, cfg.selector)
	if err != nil {
		return fmt.Errorf("find pod: %w", err)
	}

	_, _ = fmt.Fprintf(os.Stdout, "pod: %s\n", pod)

	pfCtx, pfCancel := context.WithCancel(ctx)
	defer pfCancel()

	err = startPortForward(pfCtx, cfg.namespace, pod, cfg.port)
	if err != nil {
		return fmt.Errorf("port-forward: %w", err)
	}

	baseURL := fmt.Sprintf("http://localhost:%d", cfg.port)

	bench(ctx, baseURL)

	return nil
}

func findPod(ctx context.Context, namespace, selector string) (string, error) {
	out, err := exec.CommandContext(ctx, //nolint:gosec
		"kubectl", "get", "pod",
		"-n", namespace, "-l", selector,
		"--field-selector=status.phase=Running",
		"-o", "json",
	).Output()
	if err != nil {
		return "", fmt.Errorf("command execution: %w", err)
	}

	var list struct {
		Items []struct {
			Metadata struct {
				Name              string `json:"name"`
				DeletionTimestamp string `json:"deletionTimestamp"`
			} `json:"metadata"`
		} `json:"items"`
	}

	err = json.Unmarshal(out, &list)
	if err != nil {
		return "", fmt.Errorf("parse pod list: %w", err)
	}

	for _, item := range list.Items {
		if item.Metadata.DeletionTimestamp == "" {
			return item.Metadata.Name, nil
		}
	}

	return "", fmt.Errorf("selector %q in namespace %q: %w", selector, namespace, errNoPod)
}

func startPortForward(ctx context.Context, namespace, pod string, localPort int) error {
	cmd := exec.CommandContext(ctx, //nolint:gosec
		"kubectl", "port-forward",
		"-n", namespace, "pod/"+pod,
		fmt.Sprintf("%d:8000", localPort),
	)
	cmd.Stderr = nil

	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}

	// Monitor the command in a goroutine to catch unexpected exits.
	exitChan := make(chan error, 1)

	go func() {
		exitChan <- cmd.Wait()
	}()

	url := fmt.Sprintf("http://localhost:%d/metrics", localPort)

	deadline := time.Now().Add(pfTimeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) //nolint:gosec,noctx
		if err == nil {
			_ = resp.Body.Close()

			_, _ = fmt.Fprintf(os.Stdout, "port-forward ready on localhost:%d\n\n", localPort)

			return nil
		}

		select {
		case err := <-exitChan:
			return fmt.Errorf("port-forward process exited unexpectedly: %w", err)
		case <-ctx.Done():
			return fmt.Errorf("port-forward context error: %w", ctx.Err())
		case <-time.After(backoffDuration):
		}
	}

	// Cleanup if timeout occurs.
	_ = cmd.Process.Kill()

	return fmt.Errorf("port-forward timeout: %w", errPortForwardTimeout)
}
