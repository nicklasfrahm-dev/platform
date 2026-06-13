package main

import (
	"math"
	"sort"
)

// statSummary holds summary statistics for a set of samples.
type statSummary struct {
	min    float64
	max    float64
	avg    float64
	stddev float64
	p50    float64
	p95    float64
	p99    float64
}

// summarize computes summary statistics over samples, sorting the slice in place.
func summarize(samples []float64) statSummary {
	if len(samples) == 0 {
		return statSummary{}
	}

	sort.Float64s(samples)

	var sum float64

	for _, v := range samples {
		sum += v
	}

	avg := sum / float64(len(samples))

	var sqDiffSum float64

	for _, v := range samples {
		d := v - avg
		sqDiffSum += d * d
	}

	stddev := 0.0
	if len(samples) > 1 {
		stddev = math.Sqrt(sqDiffSum / float64(len(samples)-1))
	}

	return statSummary{
		min:    samples[0],
		max:    samples[len(samples)-1],
		avg:    avg,
		stddev: stddev,
		p50:    percentile(samples, percentileP50),
		p95:    percentile(samples, percentileP95),
		p99:    percentile(samples, percentileP99),
	}
}

// percentile returns the p-th percentile of sorted samples using linear interpolation.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 1 {
		return sorted[0]
	}

	rank := p / percentileScale * float64(len(sorted)-1)
	lowerIdx := int(math.Floor(rank))
	upperIdx := int(math.Ceil(rank))

	if lowerIdx == upperIdx {
		return sorted[lowerIdx]
	}

	frac := rank - float64(lowerIdx)

	return sorted[lowerIdx] + frac*(sorted[upperIdx]-sorted[lowerIdx])
}
