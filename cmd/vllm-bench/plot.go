package main

import (
	"fmt"
	"strings"

	"github.com/guptarohit/asciigraph"
)

// lineChart renders a titled line chart with custom x-axis labels.
func lineChart(title string, xLabels []string, values []float64, _ string) string {
	if len(values) == 0 {
		return ""
	}
	width := max(minChartWidth, len(xLabels)*8)
	plot := asciigraph.Plot(values,
		asciigraph.Caption(title),
		asciigraph.Height(chartHeight),
		asciigraph.Width(width),
	)

	yAxisWidth := yAxisMargin(values)
	step := width / max(len(xLabels)-1, 1)
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

	return len(fmt.Sprintf("%.2f", maxVal)) + yAxisPadding
}
