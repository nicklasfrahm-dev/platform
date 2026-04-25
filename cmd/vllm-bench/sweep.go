package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

type sweepPoint struct {
	targetTokens     int
	promptTokens     int
	completionTokens int
	avgLatency       time.Duration
	minLatency       time.Duration
	maxLatency       time.Duration
	avgTokPerSec     float64
	minTokPerSec     float64
	maxTokPerSec     float64
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

		sweepPoint, ok := measureSweepPoint(ctx, baseURL, cfg, target)
		if !ok {
			_, _ = fmt.Fprintln(os.Stdout, "all requests failed, skipping")

			continue
		}

		results = append(results, sweepPoint)
		_, _ = fmt.Fprintf(os.Stdout,
			"prompt=%d  lat min=%s avg=%s max=%s  tok/s min=%.1f avg=%.1f max=%.1f\n",
			sweepPoint.promptTokens,
			sweepPoint.minLatency.Round(time.Millisecond),
			sweepPoint.avgLatency.Round(time.Millisecond),
			sweepPoint.maxLatency.Round(time.Millisecond),
			sweepPoint.minTokPerSec,
			sweepPoint.avgTokPerSec,
			sweepPoint.maxTokPerSec,
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

	var (
		totalLat        time.Duration
		minLat          = time.Duration(math.MaxInt64)
		maxLat          time.Duration
		totalPrompt     int
		totalCompletion int
		succeeded       int
		minTok          = math.MaxFloat64
		maxTok          float64
	)

	for sweepIdx := range cfg.sweepReqs {
		if ctx.Err() != nil {
			break
		}

		waitForCapacity(ctx, baseURL, cfg)

		result, err := sendChatCompletion(ctx, cfg.client, baseURL, cfg.model, prompt, cfg.sweepOutToks)
		if err != nil {
			log.Printf("request %d for %d tokens: %v", sweepIdx+1, target, err)

			continue
		}

		tok := float64(result.completionTokens) / result.latency.Seconds()

		if result.latency < minLat {
			minLat = result.latency
		}

		if result.latency > maxLat {
			maxLat = result.latency
		}

		if tok < minTok {
			minTok = tok
		}

		if tok > maxTok {
			maxTok = tok
		}

		totalLat += result.latency
		totalPrompt += result.promptTokens
		totalCompletion += result.completionTokens
		succeeded++
	}

	if succeeded == 0 {
		return sweepPoint{}, false
	}

	return buildSweepPoint(target, totalPrompt, totalCompletion, succeeded, totalLat, minLat, maxLat, minTok, maxTok), true
}

func buildSweepPoint(
	target, totalPrompt, totalCompletion, succeeded int,
	totalLat, minLat, maxLat time.Duration,
	minTok, maxTok float64,
) sweepPoint {
	avgLat := totalLat / time.Duration(succeeded)
	avgCompletion := totalCompletion / succeeded

	return sweepPoint{
		targetTokens:     target,
		promptTokens:     totalPrompt / succeeded,
		completionTokens: avgCompletion,
		avgLatency:       avgLat,
		minLatency:       minLat,
		maxLatency:       maxLat,
		avgTokPerSec:     float64(avgCompletion) / avgLat.Seconds(),
		minTokPerSec:     minTok,
		maxTokPerSec:     maxTok,
	}
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
	numResults := len(results)
	labels := make([]string, numResults)
	minLat := make([]float64, numResults)
	avgLat := make([]float64, numResults)
	maxLat := make([]float64, numResults)
	minTok := make([]float64, numResults)
	avgTok := make([]float64, numResults)
	maxTok := make([]float64, numResults)

	for idx, result := range results {
		k := result.promptTokens / tokensPerK
		if k == 0 {
			labels[idx] = fmt.Sprintf("%dt", result.promptTokens)
		} else {
			labels[idx] = fmt.Sprintf("%dk", k)
		}

		minLat[idx] = result.minLatency.Seconds()
		avgLat[idx] = result.avgLatency.Seconds()
		maxLat[idx] = result.maxLatency.Seconds()
		minTok[idx] = result.minTokPerSec
		avgTok[idx] = result.avgTokPerSec
		maxTok[idx] = result.maxTokPerSec
	}

	_, _ = fmt.Fprintln(os.Stdout)
	_, _ = fmt.Fprintln(os.Stdout, errorBarChart("Context window vs latency (s)", labels, minLat, avgLat, maxLat))
	_, _ = fmt.Fprintln(os.Stdout)
	_, _ = fmt.Fprintln(os.Stdout, errorBarChart("Context window vs throughput (tok/s)", labels, minTok, avgTok, maxTok))
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
