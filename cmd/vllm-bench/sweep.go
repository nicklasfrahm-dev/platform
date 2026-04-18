package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

type sweepPoint struct {
	targetTokens     int
	promptTokens     int
	completionTokens int
	latency          time.Duration
}

func parseSweepSizes(s string) []int {
	var sizes []int
	for part := range strings.SplitSeq(s, ",") {
		v, err := strconv.Atoi(strings.TrimSpace(part))
		if err == nil && v > 0 {
			sizes = append(sizes, v)
		}
	}

	return sizes
}

func runSweep(ctx context.Context, baseURL string, cfg config, sizes []int) {
	_, _ = fmt.Fprintf(os.Stdout, "sweep: %d context sizes, %d requests each, %d output tokens\n\n",
		len(sizes), cfg.sweepReqs, cfg.sweepOutToks)

	results := make([]sweepPoint, 0, len(sizes))

	for _, target := range sizes {
		if ctx.Err() != nil {
			break
		}
		_, _ = fmt.Fprintf(os.Stdout, "context ~%dk tokens ... ", target/tokensPerK)

		pt, ok := measureSweepPoint(ctx, baseURL, cfg, target)
		if !ok {
			_, _ = fmt.Fprintln(os.Stdout, "all requests failed, skipping")

			continue
		}

		results = append(results, pt)
		_, _ = fmt.Fprintf(os.Stdout, "prompt=%d  lat=%s  tok/s=%.1f\n",
			pt.promptTokens,
			pt.latency.Round(time.Millisecond),
			float64(pt.completionTokens)/pt.latency.Seconds(),
		)
	}

	if len(results) < minChartSamples {
		_, _ = fmt.Fprintln(os.Stdout, "not enough data points for charts")

		return
	}

	printSweepCharts(results)
}

func measureSweepPoint(ctx context.Context, baseURL string, cfg config, target int) (sweepPoint, bool) {
	prompt := paddedPrompt(target)
	var totalLat time.Duration
	var totalPrompt, totalCompletion, succeeded int

	for i := range cfg.sweepReqs {
		if ctx.Err() != nil {
			break
		}
		waitForCapacity(ctx, baseURL, cfg)

		result, err := sendChatCompletion(ctx, cfg.client, baseURL, cfg.model, prompt, cfg.sweepOutToks)
		if err != nil {
			log.Printf("request %d for %d tokens: %v", i+1, target, err)

			continue
		}
		totalLat += result.latency
		totalPrompt += result.promptTokens
		totalCompletion += result.completionTokens
		succeeded++
	}

	if succeeded == 0 {
		return sweepPoint{}, false
	}

	return sweepPoint{
		targetTokens:     target,
		promptTokens:     totalPrompt / succeeded,
		completionTokens: totalCompletion / succeeded,
		latency:          totalLat / time.Duration(succeeded),
	}, true
}

// waitForCapacity blocks until vLLM's waiting queue has room or the context is cancelled.
func waitForCapacity(ctx context.Context, baseURL string, cfg config) {
waitLoop:
	for {
		snap, err := scrapeMetrics(baseURL + "/metrics")
		if err != nil || snap.waiting < float64(cfg.maxQueue) {
			break waitLoop
		}
		select {
		case <-ctx.Done():
			break waitLoop
		case <-time.After(backoffDuration):
		}
	}
}

func printSweepCharts(results []sweepPoint) {
	labels := make([]string, len(results))
	latencies := make([]float64, len(results))
	throughputs := make([]float64, len(results))

	for i, r := range results {
		k := r.promptTokens / tokensPerK
		if k == 0 {
			labels[i] = fmt.Sprintf("%dt", r.promptTokens)
		} else {
			labels[i] = fmt.Sprintf("%dk", k)
		}
		latencies[i] = r.latency.Seconds()
		throughputs[i] = float64(r.completionTokens) / r.latency.Seconds()
	}

	_, _ = fmt.Fprintln(os.Stdout)
	_, _ = fmt.Fprintln(os.Stdout, lineChart("Context window vs latency (s)", labels, latencies, "s"))
	_, _ = fmt.Fprintln(os.Stdout)
	_, _ = fmt.Fprintln(os.Stdout, lineChart("Context window vs throughput (tok/s)", labels, throughputs, "tok/s"))
}

// paddedPrompt returns a prompt whose token count is approximately targetTokens.
// Approximation: 1 token ≈ 3.5 characters for English prose.
func paddedPrompt(targetTokens int) string {
	question := "Summarise the above in one sentence."
	charsNeeded := int(float64(targetTokens)*charsPerToken) - len(question) - paddingOverhead
	if charsNeeded < 0 {
		return question
	}
	seed := "The transformer architecture relies on self-attention mechanisms " +
		"that allow each token in a sequence to attend to every other token, " +
		"enabling the model to capture long-range dependencies efficiently. "
	var sb strings.Builder
	for sb.Len() < charsNeeded {
		sb.WriteString(seed)
	}

	return sb.String()[:charsNeeded] + "\n\n" + question
}
