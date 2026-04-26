package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type hfFileMeta struct {
	RFilename string `json:"rFilename"`
}

type hfModelMeta struct {
	Siblings []hfFileMeta `json:"siblings"`
}

// hfListFiles returns the list of files in a HuggingFace model repository.
func hfListFiles(ctx context.Context, token, repo string) ([]string, error) {
	url := fmt.Sprintf("https://huggingface.co/api/models/%s", repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build HF list request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch model metadata: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HuggingFace API returned status %d", resp.StatusCode)
	}

	var meta hfModelMeta
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, fmt.Errorf("decode model metadata: %w", err)
	}

	files := make([]string, len(meta.Siblings))
	for idx, sibling := range meta.Siblings {
		files[idx] = sibling.RFilename
	}

	return files, nil
}

// hfOpenFile opens a file from a HuggingFace repository for streaming download.
// The caller is responsible for closing the returned ReadCloser.
func hfOpenFile(ctx context.Context, token, repo, file string) (io.ReadCloser, error) {
	url := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", repo, file)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build HF download request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", file, err)
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()

		return nil, fmt.Errorf("HuggingFace returned status %d for %s", resp.StatusCode, file)
	}

	return resp.Body, nil
}
