// Package main is the entry point for vllm-bench, a token throughput benchmark
// for vLLM deployments running in Kubernetes.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

var (
	errNoPod              = errors.New("no running pod found")
	errPortForwardTimeout = errors.New("port-forward did not become ready within 30s")
)

const (
	defaultSelector = "serving.kserve.io/inferenceservice=gemma4-moe"
	defaultPrompt   = "Explain in detail how transformer attention mechanisms work in large language models."
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
	sweep        bool
	sweepSizes   string
	sweepReqs    int
	sweepOutToks int
	client       *http.Client
}

func parseFlags() config {
	var cfg config
	flag.DurationVar(&cfg.duration, "duration", defaultDuration, "benchmark duration (standard mode)")
	flag.IntVar(&cfg.workers, "workers", 1, "concurrent workers; each sends requests sequentially")
	flag.StringVar(&cfg.namespace, "namespace", "llm", "kubernetes namespace")
	flag.StringVar(&cfg.selector, "selector", defaultSelector, "pod label selector")
	flag.StringVar(&cfg.model, "model", "gemma-4-26b-a4b-it", "served model name")
	flag.IntVar(&cfg.maxTokens, "max-tokens", defaultMaxTokens, "max completion tokens per request")
	flag.IntVar(&cfg.maxQueue, "max-queue", defaultMaxQueue, "pause worker when vLLM waiting queue reaches this depth")
	flag.IntVar(&cfg.port, "port", defaultPort, "local port for kubectl port-forward")
	flag.StringVar(&cfg.prompt, "prompt", defaultPrompt, "prompt for standard mode")
	flag.BoolVar(&cfg.sweep, "sweep", false, "sweep context sizes and plot results")
	flag.StringVar(&cfg.sweepSizes, "sweep-sizes", defaultSweepSizes, "comma-separated input token targets for sweep mode")
	flag.IntVar(&cfg.sweepReqs, "sweep-requests", defaultSweepReqs, "requests per context size in sweep mode (averaged)")
	flag.IntVar(&cfg.sweepOutToks, "sweep-out-tokens", defaultSweepOutToks, "output tokens per request in sweep mode")
	flag.Parse()

	cfg.client = &http.Client{Timeout: clientTimeout}

	return cfg
}

func main() {
	err := run()
	if err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := parseFlags()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	pod, err := findPod(ctx, cfg.namespace, cfg.selector)
	if err != nil {
		cancel()

		return fmt.Errorf("find pod: %w", err)
	}

	_, _ = fmt.Fprintf(os.Stdout, "pod: %s\n", pod)

	pfCtx, pfCancel := context.WithCancel(ctx)

	err = startPortForward(pfCtx, cfg.namespace, pod, cfg.port)
	if err != nil {
		pfCancel()
		cancel()

		return fmt.Errorf("port-forward: %w", err)
	}

	defer pfCancel()
	defer cancel()

	baseURL := fmt.Sprintf("http://localhost:%d", cfg.port)

	if cfg.sweep {
		sizes := parseSweepSizes(cfg.sweepSizes)
		runSweep(ctx, baseURL, cfg, sizes)
	} else {
		runBench(ctx, baseURL, cfg)
	}

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
	cmd.Stderr = os.Stderr

	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}

	// Monitor the command in a goroutine to catch unexpected exits
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

			// Wait for command to fail or context to be cancelled
			select {
			case err := <-exitChan:
				if err != nil {
					return fmt.Errorf("port-forward process exited unexpectedly: %w", err)
				}

				return nil
			case <-ctx.Done():
				return fmt.Errorf("port-forward context error: %w", ctx.Err())
			}
		}

		select {
		case err := <-exitChan:
			return fmt.Errorf("port-forward process exited unexpectedly: %w", err)
		case <-ctx.Done():
			return fmt.Errorf("port-forward context error: %w", ctx.Err())
		case <-time.After(backoffDuration):
		}
	}

	// Cleanup if timeout occurs
	_ = cmd.Process.Kill()

	return fmt.Errorf("port-forward timeout: %w", errPortForwardTimeout)
}

