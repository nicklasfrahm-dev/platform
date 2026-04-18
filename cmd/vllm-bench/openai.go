package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

var errUnexpectedStatus = errors.New("unexpected HTTP status")

type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	MaxTokens int           `json:"maxTokens"`
	Stream    bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Usage struct {
		PromptTokens     int `json:"promptTokens"`
		CompletionTokens int `json:"completionTokens"`
	} `json:"usage"`
}

type inferenceResult struct {
	promptTokens     int
	completionTokens int
	latency          time.Duration
}

func sendChatCompletion(
	ctx context.Context, client *http.Client,
	baseURL, model, prompt string, maxTokens int,
) (inferenceResult, error) {
	body, err := json.Marshal(chatRequest{
		Model:     model,
		Messages:  []chatMessage{{Role: "user", Content: prompt}},
		MaxTokens: maxTokens,
		Stream:    false,
	})
	if err != nil {
		return inferenceResult{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return inferenceResult{}, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	start := time.Now()

	resp, err := client.Do(req)
	if err != nil {
		return inferenceResult{}, fmt.Errorf("do request: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return inferenceResult{}, fmt.Errorf("read error body: %w", err)
		}

		return inferenceResult{}, fmt.Errorf("%w: %d %s", errUnexpectedStatus, resp.StatusCode, bytes.TrimSpace(b))
	}

	var chatResp chatResponse

	err = json.NewDecoder(resp.Body).Decode(&chatResp)
	if err != nil {
		return inferenceResult{}, fmt.Errorf("decode response: %w", err)
	}

	return inferenceResult{
		promptTokens:     chatResp.Usage.PromptTokens,
		completionTokens: chatResp.Usage.CompletionTokens,
		latency:          time.Since(start),
	}, nil
}
