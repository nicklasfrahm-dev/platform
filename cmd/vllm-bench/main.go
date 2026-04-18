// Package main is the entry point for vllm-bench, a token throughput benchmark
// for vLLM deployments running in Kubernetes.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var (
	flagDuration     = flag.Duration("duration", 2*time.Minute, "benchmark duration (standard mode)")
	flagWorkers      = flag.Int("workers", 1, "concurrent workers; each sends requests sequentially")
	flagNamespace    = flag.String("namespace", "llm", "kubernetes namespace")
	flagSelector     = flag.String("selector", "serving.kserve.io/inferenceservice=gemma4-moe", "pod label selector")
	flagModel        = flag.String("model", "gemma4-26b-a4b-it", "served model name")
	flagMaxTokens    = flag.Int("max-tokens", 512, "max completion tokens per request")
	flagMaxQueue     = flag.Int("max-queue", 3, "pause worker when vLLM waiting queue reaches this depth")
	flagPort         = flag.Int("port", 18000, "local port for kubectl port-forward")
	flagPrompt       = flag.String("prompt", "Explain in detail how transformer attention mechanisms work in large language models.", "prompt for standard mode")
	flagSweep        = flag.Bool("sweep", false, "sweep context sizes and plot results")
	flagSweepSizes   = flag.String("sweep-sizes", "1024,4096,16384,32768,65536,131072", "comma-separated input token targets for sweep mode")
	flagSweepReqs    = flag.Int("sweep-requests", 3, "requests per context size in sweep mode (averaged)")
	flagSweepOutToks = flag.Int("sweep-out-tokens", 64, "output tokens per request in sweep mode")
)

func main() {
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pod, err := findPod(ctx, *flagNamespace, *flagSelector)
	if err != nil {
		log.Fatalf("find pod: %v", err)
	}
	fmt.Printf("pod: %s\n", pod)

	pfCtx, pfCancel := context.WithCancel(ctx)
	defer pfCancel()

	if err := startPortForward(pfCtx, *flagNamespace, pod, *flagPort); err != nil {
		log.Fatalf("port-forward: %v", err)
	}

	baseURL := fmt.Sprintf("http://localhost:%d", *flagPort)

	if *flagSweep {
		sizes := parseSweepSizes(*flagSweepSizes)
		runSweep(ctx, baseURL, sizes)
	} else {
		runBench(ctx, baseURL)
	}
}

func findPod(ctx context.Context, ns, selector string) (string, error) {
	out, err := exec.CommandContext(ctx,
		"kubectl", "get", "pod",
		"-n", ns, "-l", selector,
		"--field-selector=status.phase=Running",
		"-o", "jsonpath={.items[0].metadata.name}",
	).Output()
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", fmt.Errorf("no running pod for selector %q in namespace %q", selector, ns)
	}
	return name, nil
}

func startPortForward(ctx context.Context, ns, pod string, localPort int) error {
	cmd := exec.CommandContext(ctx,
		"kubectl", "port-forward",
		"-n", ns, "pod/"+pod,
		fmt.Sprintf("%d:8000", localPort),
	)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	url := fmt.Sprintf("http://localhost:%d/metrics", localPort)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) //nolint:noctx
		if err == nil {
			resp.Body.Close()
			fmt.Printf("port-forward ready on localhost:%d\n\n", localPort)
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("port-forward did not become ready within 30s")
}
