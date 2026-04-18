package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type stats struct {
	requests  atomic.Int64
	promptTok atomic.Int64
	genTok    atomic.Int64
	errors    atomic.Int64
	latencyNs atomic.Int64
}

func runBench(ctx context.Context, baseURL string, cfg config) {
	benchCtx, cancel := context.WithTimeout(ctx, cfg.duration)
	defer cancel()

	st := &stats{}
	var wg sync.WaitGroup
	for i := range cfg.workers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			benchWorker(benchCtx, id, baseURL, cfg, st)
		}(i)
	}

	start := time.Now()
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	var throughputSamples []float64
	var lastGenTok int64

	for {
		select {
		case <-ticker.C:
			snap, _ := scrapeMetrics(baseURL + "/metrics")
			elapsed := time.Since(start)
			reqs := st.requests.Load()
			gen := st.genTok.Load()
			errs := st.errors.Load()
			deltaGen := gen - lastGenTok
			lastGenTok = gen
			tokPerSec := float64(deltaGen) / tickInterval.Seconds()
			throughputSamples = append(throughputSamples, tokPerSec)
			printTickLine(elapsed, reqs, errs, tokPerSec, float64(reqs)/elapsed.Seconds(), snap)

		case <-benchCtx.Done():
			wg.Wait()
			printBenchSummary(start, st, throughputSamples)

			return
		}
	}
}

func printTickLine(elapsed time.Duration, reqs, errs int64, tokPerSec, reqPerSec float64, snap vllmSnapshot) {
	_, _ = fmt.Fprintf(os.Stdout,
		"[%5s] requests=%-4d errors=%-3d gen_tok/s=%6.1f req/s=%4.2f avg_lat=? running=%.0f waiting=%.0f\n",
		elapsed.Round(time.Second), reqs, errs, tokPerSec, reqPerSec, snap.running, snap.waiting,
	)
}

func benchWorker(ctx context.Context, id int, baseURL string, cfg config, st *stats) {
	for {
		if ctx.Err() != nil {
			return
		}

		snap, err := scrapeMetrics(baseURL + "/metrics")
		if err == nil && snap.waiting >= float64(cfg.maxQueue) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoffDuration):
			}

			continue
		}

		result, err := sendChatCompletion(ctx, cfg.client, baseURL, cfg.model, cfg.prompt, cfg.maxTokens)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("worker %d: %v", id, err)
				st.errors.Add(1)
			}

			continue
		}
		st.requests.Add(1)
		st.promptTok.Add(int64(result.promptTokens))
		st.genTok.Add(int64(result.completionTokens))
		st.latencyNs.Add(result.latency.Nanoseconds())
	}
}

func printBenchSummary(start time.Time, st *stats, samples []float64) {
	elapsed := time.Since(start)
	reqs := st.requests.Load()
	gen := st.genTok.Load()
	prompt := st.promptTok.Load()
	errs := st.errors.Load()

	_, _ = fmt.Fprintf(os.Stdout, "\n─── summary ─────────────────────────────────────\n")
	_, _ = fmt.Fprintf(os.Stdout, "duration:       %s\n", elapsed.Round(time.Millisecond))
	_, _ = fmt.Fprintf(os.Stdout, "requests:       %d  (errors: %d)\n", reqs, errs)
	_, _ = fmt.Fprintf(os.Stdout, "prompt tokens:  %d\n", prompt)
	_, _ = fmt.Fprintf(os.Stdout, "gen tokens:     %d\n", gen)
	_, _ = fmt.Fprintf(os.Stdout, "gen tok/s:      %.2f\n", float64(gen)/elapsed.Seconds())
	_, _ = fmt.Fprintf(os.Stdout, "avg latency:    %s\n", avgLatency(st).Round(time.Millisecond))

	if len(samples) >= minChartSamples {
		labels := timeLabels(len(samples), tickInterval)
		_, _ = fmt.Fprintln(os.Stdout)
		_, _ = fmt.Fprintln(os.Stdout, lineChart("Generation throughput (tok/s) over time", labels, samples, "tok/s"))
	}
}

func avgLatency(st *stats) time.Duration {
	reqs := st.requests.Load()
	if reqs == 0 {
		return 0
	}

	return time.Duration(st.latencyNs.Load() / reqs)
}

// timeLabels builds evenly spaced x-axis labels for a chart of n samples at interval d.
func timeLabels(n int, interval time.Duration) []string {
	step := max(1, n/xAxisLabelDiv)
	labels := make([]string, n)
	for i := range n {
		if i%step == 0 {
			labels[i] = (time.Duration(i) * interval).String()
		}
	}

	return labels
}
