package main

import (
	"fmt"
	"math"
	"strings"

	"github.com/guptarohit/asciigraph"
)

// lineChart renders a titled line chart with custom x-axis labels.
func lineChart(title string, xLabels []string, values []float64, yUnit string) string {
	if len(values) == 0 {
		return ""
	}
	plot := asciigraph.Plot(values,
		asciigraph.Caption(title),
		asciigraph.Height(10),
		asciigraph.Width(max(60, len(xLabels)*8)),
	)

	// Build x-axis label row aligned under each data point.
	// asciigraph leaves a left margin for the y-axis; approximate it.
	yAxisWidth := yAxisMargin(values)
	step := max(60, len(xLabels)*8) / max(len(xLabels)-1, 1)
	var axis strings.Builder
	axis.WriteString(strings.Repeat(" ", yAxisWidth))
	for i, l := range xLabels {
		pad := step - len(l)
		if i == 0 {
			axis.WriteString(l)
		} else {
			axis.WriteString(strings.Repeat(" ", max(pad, 1)))
			axis.WriteString(l)
		}
	}
	_ = yUnit
	return plot + "\n" + axis.String()
}

// yAxisMargin estimates the left-margin width asciigraph uses for y-axis labels.
func yAxisMargin(values []float64) int {
	maxVal := 0.0
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}
	digits := len(fmt.Sprintf("%.2f", maxVal))
	return digits + 2 // asciigraph pads with " " + value + " ┤"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// roundSigFig rounds v to 3 significant figures for axis labels.
func roundSigFig(v float64) float64 {
	if v == 0 {
		return 0
	}
	p := math.Pow(10, math.Floor(math.Log10(math.Abs(v)))-2)
	return math.Round(v/p) * p
}
