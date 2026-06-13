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

	const labelMultiplier = 8

	width := max(minChartWidth, len(xLabels)*labelMultiplier)
	plot := asciigraph.Plot(values,
		asciigraph.Caption(title),
		asciigraph.Height(chartHeight),
		asciigraph.Width(width),
	)

	yAxisWidth := yAxisMargin(values)
	step := width / max(len(xLabels)-1, 1)

	var axis strings.Builder

	axis.WriteString(strings.Repeat(" ", yAxisWidth))

	for labelIdx, labelName := range xLabels {
		pad := step - len(labelName)

		if labelIdx == 0 {
			axis.WriteString(labelName)
		} else {
			axis.WriteString(strings.Repeat(" ", max(pad, 1)))
			axis.WriteString(labelName)
		}
	}

	return plot + "\n" + axis.String()
}

// errorBarChart renders a chart with vertical bars from p50 (marked with *) to p95.
func errorBarChart(title string, xLabels []string, p50s, p95s []float64) string {
	numPoints := len(p50s)
	if numPoints == 0 {
		return ""
	}

	const labelMultiplier = 8

	width := max(minChartWidth, numPoints*labelMultiplier)
	globalMin, globalMax := yRange(p50s, p95s)
	cols := columnPositions(numPoints, width)
	grid := makeEmptyGrid(chartHeight, width)

	fillErrorBars(grid, cols, p50s, p95s, globalMin, globalMax)

	return renderErrorBarChart(title, xLabels, grid, globalMin, globalMax, width)
}

// yRange returns the overall (min, max) across both value sets, assuming los[idx] <= his[idx].
func yRange(los, his []float64) (float64, float64) {
	globalMin, globalMax := los[0], his[0]

	for idx := range los {
		if los[idx] < globalMin {
			globalMin = los[idx]
		}

		if his[idx] > globalMax {
			globalMax = his[idx]
		}
	}

	return globalMin, globalMax
}

func columnPositions(count, width int) []int {
	cols := make([]int, count)

	if count > 1 {
		for i := range count {
			cols[i] = i * (width - 1) / (count - 1)
		}

		return cols
	}

	cols[0] = width / halfDivisor

	return cols
}

func makeEmptyGrid(rows, cols int) [][]rune {
	grid := make([][]rune, rows)

	for r := range rows {
		grid[r] = make([]rune, cols)

		for c := range cols {
			grid[r][c] = ' '
		}
	}

	return grid
}

func valueToRow(v, globalMin, globalMax float64) int {
	if globalMax == globalMin {
		return chartHeight / halfDivisor
	}

	row := int((globalMax-v)/(globalMax-globalMin)*float64(chartHeight-1) + roundingHalf)

	if row < 0 {
		return 0
	}

	if row >= chartHeight {
		return chartHeight - 1
	}

	return row
}

func fillErrorBars(grid [][]rune, cols []int, p50s, p95s []float64, globalMin, globalMax float64) {
	for i := range cols {
		col := cols[i]
		topRow := valueToRow(p95s[i], globalMin, globalMax)
		botRow := valueToRow(p50s[i], globalMin, globalMax)

		for r := topRow; r <= botRow; r++ {
			grid[r][col] = '│'
		}

		grid[botRow][col] = '*'
	}
}

func renderErrorBarChart(
	title string, xLabels []string, grid [][]rune, globalMin, globalMax float64, width int,
) string {
	yAxisWidth := len(fmt.Sprintf("%.2f", globalMax)) + yAxisPadding
	labelFmt := fmt.Sprintf("%%%d.2f ┤", yAxisWidth-yAxisPadding)

	var output strings.Builder

	for r := range chartHeight {
		yVal := globalMax - float64(r)/float64(chartHeight-1)*(globalMax-globalMin)

		fmt.Fprintf(&output, labelFmt, yVal)
		output.WriteString(string(grid[r]))
		output.WriteByte('\n')
	}

	titlePad := max((yAxisWidth+width-len(title))/halfDivisor, 0)
	output.WriteString(strings.Repeat(" ", titlePad))
	output.WriteString(title)
	output.WriteByte('\n')

	writeXAxisLabels(&output, xLabels, yAxisWidth, width)

	return output.String()
}

func writeXAxisLabels(output *strings.Builder, xLabels []string, yAxisWidth, width int) {
	output.WriteString(strings.Repeat(" ", yAxisWidth))

	step := width / max(len(xLabels)-1, 1)

	for labelIdx, label := range xLabels {
		if labelIdx == 0 {
			output.WriteString(label)

			continue
		}

		pad := step - len(label)
		output.WriteString(strings.Repeat(" ", max(pad, 1)))
		output.WriteString(label)
	}
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
