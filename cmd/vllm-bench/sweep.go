package main

import (
	"context"
	"fmt"
	"log"
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
	for _, part := range strings.Split(s, ",") {
		v, err := strconv.Atoi(strings.TrimSpace(part))
		if err == nil && v > 0 {
			sizes = append(sizes, v)
		}
	}
	return sizes
}

func runSweep(ctx context.Context, baseURL string, sizes []int) {
	fmt.Printf("sweep: %d context sizes, %d requests each, %d output tokens\n\n",
		len(sizes), *flagSweepReqs, *flagSweepOutToks)

	results := make([]sweepPoint, 0, len(sizes))

	for _, target := range sizes {
		if ctx.Err() != nil {
			break
		}
		fmt.Printf("context ~%dk tokens ... ", target/1024)
		prompt := paddedPrompt(target)

		var totalLat time.Duration
		var totalPrompt, totalCompletion int
		var succeeded int

		for i := range *flagSweepReqs {
			if ctx.Err() != nil {
				break
			}
			// wait for queue to clear before each sweep request
			for {
				snap, err := scrapeMetrics(baseURL + "/metrics")
				if err != nil || snap.waiting < float64(*flagMaxQueue) {
					break
				}
				select {
				case <-ctx.Done():
					break
				case <-time.After(500 * time.Millisecond):
				}
			}

			result, err := sendChatCompletion(ctx, baseURL, *flagModel, prompt, *flagSweepOutToks)
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
			fmt.Println("all requests failed, skipping")
			continue
		}

		pt := sweepPoint{
			targetTokens:     target,
			promptTokens:     totalPrompt / succeeded,
			completionTokens: totalCompletion / succeeded,
			latency:          totalLat / time.Duration(succeeded),
		}
		results = append(results, pt)
		tokPerSec := float64(pt.completionTokens) / pt.latency.Seconds()
		fmt.Printf("prompt=%d  lat=%s  tok/s=%.1f\n",
			pt.promptTokens,
			pt.latency.Round(time.Millisecond),
			tokPerSec,
		)
	}

	if len(results) < 2 {
		fmt.Println("not enough data points for charts")
		return
	}

	printSweepCharts(results)
}

func printSweepCharts(results []sweepPoint) {
	labels := make([]string, len(results))
	latencies := make([]float64, len(results))
	throughputs := make([]float64, len(results))

	for i, r := range results {
		k := r.promptTokens / 1024
		if k == 0 {
			labels[i] = fmt.Sprintf("%dt", r.promptTokens)
		} else {
			labels[i] = fmt.Sprintf("%dk", k)
		}
		latencies[i] = r.latency.Seconds()
		throughputs[i] = float64(r.completionTokens) / r.latency.Seconds()
	}

	fmt.Println()
	fmt.Println(lineChart("Context window vs latency (s)", labels, latencies, "s"))
	fmt.Println()
	fmt.Println(lineChart("Context window vs throughput (tok/s)", labels, throughputs, "tok/s"))
}

// paddedPrompt returns a prompt whose token count is approximately targetTokens.
// It fills a system-like preamble with repeated text and appends a short question.
// Approximation: 1 token ≈ 3.5 characters for English prose.
func paddedPrompt(targetTokens int) string {
	question := "Summarise the above in one sentence."
	charsNeeded := int(float64(targetTokens)*3.5) - len(question) - 20
	if charsNeeded < 0 {
		return question
	}

	seed := "The transformer architecture relies on self-attention mechanisms that allow each token in a sequence to attend to every other token, enabling the model to capture long-range dependencies efficiently. "
	var sb strings.Builder
	for sb.Len() < charsNeeded {
		sb.WriteString(seed)
	}
	return sb.String()[:charsNeeded] + "\n\n" + question
}
