package main

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type vllmSnapshot struct {
	running float64
	waiting float64
}

func scrapeMetrics(url string) (vllmSnapshot, error) {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return vllmSnapshot{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return vllmSnapshot{}, err
	}
	return vllmSnapshot{
		running: parseGauge(body, "vllm:num_requests_running"),
		waiting: parseGauge(body, "vllm:num_requests_waiting"),
	}, nil
}

// parseGauge extracts the first matching metric value from Prometheus text format.
func parseGauge(body []byte, name string) float64 {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || !strings.HasPrefix(line, name) {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		v, err := strconv.ParseFloat(parts[len(parts)-1], 64)
		if err == nil {
			return v
		}
	}
	return 0
}
