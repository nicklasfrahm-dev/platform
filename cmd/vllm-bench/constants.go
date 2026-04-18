package main

import "time"

const (
	// Default flag values.
	defaultDuration     = 2 * time.Minute
	defaultMaxTokens    = 512
	defaultMaxQueue     = 3
	defaultPort         = 18000
	defaultSweepReqs    = 3
	defaultSweepOutToks = 64

	// Timing constants.
	tickInterval    = 5 * time.Second
	backoffDuration = 500 * time.Millisecond
	clientTimeout   = 15 * time.Minute
	pfTimeout       = 30 * time.Second

	// Chart constants.
	minChartSamples = 2
	xAxisLabelDiv   = 8
	minChartWidth   = 60
	chartHeight     = 10
	yAxisPadding    = 2

	// Parsing constants.
	minPrometheusFields = 2

	// Prompt padding constants.
	tokensPerK      = 1024
	paddingOverhead = 20
	charsPerToken   = 3.5
)
