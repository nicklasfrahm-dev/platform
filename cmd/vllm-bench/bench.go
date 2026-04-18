package main

import (
	"context"
	"fmt"
	"log"
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

func runBench(ctx context.Context, baseURL string) {
	benchCtx, cancel := context.WithTimeout(ctx, *flagDuration)
	defer cancel()

	st := &stats{}
	var wg sync.WaitGroup
	for i := range *flagWorkers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			benchWorker(benchCtx, id, baseURL, st)
		}(i)
	}

	start := time.Now()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// throughput samples for the end chart
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
			tokPerSec := float64(deltaGen) / 5
			throughputSamples = append(throughputSamples, tokPerSec)
			avgLat := avgLatency(st)
			fmt.Printf("[%5s] requests=%-4d errors=%-3d gen_tok/s=%6.1f req/s=%4.2f avg_lat=%s running=%.0f waiting=%.0f\n",
				elapsed.Round(time.Second),
				reqs, errs,
				tokPerSec,
				float64(reqs)/elapsed.Seconds(),
				avgLat.Round(time.Millisecond),
				snap.running, snap.waiting,
			)

		case <-benchCtx.Done():
			wg.Wait()
			printBenchSummary(start, st, throughputSamples)
			return
		}
	}
}

func benchWorker(ctx context.Context, id int, baseURL string, st *stats) {
	for {
		if ctx.Err() != nil {
			return
		}

		snap, err := scrapeMetrics(baseURL + "/metrics")
		if err == nil && snap.waiting >= float64(*flagMaxQueue) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
			continue
		}

		result, err := sendChatCompletion(ctx, baseURL, *flagModel, *flagPrompt, *flagMaxTokens)
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

	fmt.Printf("\n─── summary ─────────────────────────────────────\n")
	fmt.Printf("duration:       %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("requests:       %d  (errors: %d)\n", reqs, errs)
	fmt.Printf("prompt tokens:  %d\n", prompt)
	fmt.Printf("gen tokens:     %d\n", gen)
	fmt.Printf("gen tok/s:      %.2f\n", float64(gen)/elapsed.Seconds())
	fmt.Printf("avg latency:    %s\n", avgLatency(st).Round(time.Millisecond))

	if len(samples) >= 2 {
		// x-axis: one label every ~10 samples or at most 10 labels
		labels := timeLabels(len(samples), 5*time.Second)
		fmt.Printf("\n")
		fmt.Println(lineChart("Generation throughput (tok/s) over time", labels, samples, "tok/s"))
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
	step := max(1, n/8)
	labels := make([]string, n)
	for i := range n {
		if i%step == 0 {
			labels[i] = (time.Duration(i) * interval).String()
		}
	}
	return labels
}
