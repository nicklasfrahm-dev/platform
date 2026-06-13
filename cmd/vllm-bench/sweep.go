package main

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"time"
)

type sweepPoint struct {
	targetTokens     int
	promptTokens     int
	completionTokens int
	latency          statSummary // seconds
	tokPerSec        statSummary
}

// seedText is repeated to pad prompts and to calibrate charsPerToken.
const seedText = "The transformer architecture relies on self-attention mechanisms " +
	"that allow each token in a sequence to attend to every other token, " +
	"enabling the model to capture long-range dependencies efficiently. "

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
	charsPerToken := calibrateCharsPerToken(ctx, baseURL, cfg)

	_, _ = fmt.Fprintf(os.Stdout, "sweep: %d context sizes, %d requests each, %d output tokens\n\n",
		len(sizes), cfg.sweepReqs, cfg.sweepOutToks)

	results := make([]sweepPoint, 0, len(sizes))

	for _, target := range sizes {
		if ctx.Err() != nil {
			break
		}

		_, _ = fmt.Fprintf(os.Stdout, "context ~%dk tokens ...\n", target/tokensPerK)

		sweepPoint, ok := measureSweepPoint(ctx, baseURL, cfg, target, charsPerToken)
		if !ok {
			_, _ = fmt.Fprintln(os.Stdout, "  all requests failed, skipping")

			continue
		}

		results = append(results, sweepPoint)
		_, _ = fmt.Fprintf(os.Stdout,
			"  prompt=%d\n"+
				"  lat (s)  min=%.3f p50=%.3f avg=%.3f p95=%.3f p99=%.3f max=%.3f stddev=%.3f\n"+
				"  tok/s    min=%.1f p50=%.1f avg=%.1f p95=%.1f p99=%.1f max=%.1f stddev=%.1f\n",
			sweepPoint.promptTokens,
			sweepPoint.latency.min, sweepPoint.latency.p50, sweepPoint.latency.avg,
			sweepPoint.latency.p95, sweepPoint.latency.p99, sweepPoint.latency.max, sweepPoint.latency.stddev,
			sweepPoint.tokPerSec.min, sweepPoint.tokPerSec.p50, sweepPoint.tokPerSec.avg,
			sweepPoint.tokPerSec.p95, sweepPoint.tokPerSec.p99, sweepPoint.tokPerSec.max, sweepPoint.tokPerSec.stddev,
		)
	}

	if len(results) < minChartSamples {
		_, _ = fmt.Fprintln(os.Stdout, "not enough data points for charts")

		return
	}

	printSweepCharts(results)
}

// calibrateCharsPerToken queries vLLM's /tokenize endpoint to determine the actual
// chars-per-token ratio for this model's tokenizer, falling back to
// defaultCharsPerToken if the endpoint is unavailable.
func calibrateCharsPerToken(ctx context.Context, baseURL string, cfg config) float64 {
	var sb strings.Builder
	for sb.Len() < calibrationSampleChars {
		sb.WriteString(seedText)
	}

	sample := sb.String()[:calibrationSampleChars]

	count, err := countTokens(ctx, cfg.client, baseURL, cfg.model, sample, cfg.apiKey)
	if err != nil || count == 0 {
		log.Printf("tokenize calibration failed, using default chars/token=%.2f: %v", defaultCharsPerToken, err)

		return defaultCharsPerToken
	}

	ratio := float64(len(sample)) / float64(count)

	_, _ = fmt.Fprintf(os.Stdout, "calibrated chars/token: %.2f (default %.2f)\n\n", ratio, defaultCharsPerToken)

	return ratio
}

func measureSweepPoint(
	ctx context.Context, baseURL string, cfg config, target int, charsPerToken float64,
) (sweepPoint, bool) {
	var (
		latencies       []float64
		tokRates        []float64
		totalPrompt     int
		totalCompletion int
	)

	for sweepIdx := range cfg.sweepReqs {
		if ctx.Err() != nil {
			break
		}

		waitForCapacity(ctx, baseURL, cfg)

		prompt := paddedPrompt(target, charsPerToken, cfg.poisonPrefix)

		result, err := sendChatCompletion(ctx, cfg.client, baseURL, cfg.model, prompt, cfg.sweepOutToks, cfg.apiKey)
		if err != nil {
			log.Printf("request %d for %d tokens: %v", sweepIdx+1, target, err)

			continue
		}

		latencies = append(latencies, result.latency.Seconds())
		tokRates = append(tokRates, float64(result.completionTokens)/result.latency.Seconds())
		totalPrompt += result.promptTokens
		totalCompletion += result.completionTokens
	}

	if len(latencies) == 0 {
		return sweepPoint{}, false
	}

	return buildSweepPoint(target, totalPrompt, totalCompletion, latencies, tokRates), true
}

func buildSweepPoint(target, totalPrompt, totalCompletion int, latencies, tokRates []float64) sweepPoint {
	succeeded := len(latencies)

	return sweepPoint{
		targetTokens:     target,
		promptTokens:     totalPrompt / succeeded,
		completionTokens: totalCompletion / succeeded,
		latency:          summarize(latencies),
		tokPerSec:        summarize(tokRates),
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
	p50Lat := make([]float64, numResults)
	p95Lat := make([]float64, numResults)
	p50Tok := make([]float64, numResults)
	p95Tok := make([]float64, numResults)

	for idx, result := range results {
		k := result.promptTokens / tokensPerK
		if k == 0 {
			labels[idx] = fmt.Sprintf("%dt", result.promptTokens)
		} else {
			labels[idx] = fmt.Sprintf("%dk", k)
		}

		p50Lat[idx] = result.latency.p50
		p95Lat[idx] = result.latency.p95
		p50Tok[idx] = result.tokPerSec.p50
		p95Tok[idx] = result.tokPerSec.p95
	}

	_, _ = fmt.Fprintln(os.Stdout)
	_, _ = fmt.Fprintln(os.Stdout, errorBarChart("Context window vs latency (s)", labels, p50Lat, p95Lat))
	_, _ = fmt.Fprintln(os.Stdout)
	_, _ = fmt.Fprintln(os.Stdout, errorBarChart("Context window vs throughput (tok/s)", labels, p50Tok, p95Tok))
}

// paddedPrompt returns a prompt whose token count is approximately targetTokens,
// given the chars-per-token ratio of the model's tokenizer (see calibrateCharsPerToken).
//
// If poisonPrefixCache is true, a random ID is prepended so the prompt shares
// no common prefix with previous requests, defeating vLLM's prefix cache and
// forcing a full prefill on every request.
func paddedPrompt(targetTokens int, charsPerToken float64, poisonPrefixCache bool) string {
	question := "Summarise the above in one sentence."

	prefix := ""
	if poisonPrefixCache {
		// Weak randomness is fine: this only needs to be unique, not unpredictable.
		prefix = fmt.Sprintf("Request ID: %016x\n\n", rand.Uint64()) //nolint:gosec
	}

	charsNeeded := int(float64(targetTokens)*charsPerToken) - len(question) - len(prefix) - paddingOverhead
	if charsNeeded < 0 {
		return prefix + question
	}

	var sb strings.Builder
	for sb.Len() < charsNeeded {
		sb.WriteString(seedText)
	}

	return prefix + sb.String()[:charsNeeded] + "\n\n" + question
}
