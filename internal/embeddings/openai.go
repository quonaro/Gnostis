package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const debugBodyMaxLen = 2000

func truncateForDebug(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... [truncated]"
}

// openAICompatible is an HTTP client for OpenAI-compatible /v1/embeddings endpoints.
type openAICompatible struct {
	client    *http.Client
	url       string
	model     string
	apiKey    string
	batchSize int
}

func newOpenAICompatible(url, model, apiKey string, batchSize int) *openAICompatible {
	if batchSize <= 0 {
		batchSize = 32
	}
	return &openAICompatible{
		client:    &http.Client{Timeout: 120 * time.Second},
		url:       url,
		model:     model,
		apiKey:    apiKey,
		batchSize: batchSize,
	}
}

func (p *openAICompatible) ModelName() string {
	return p.model
}

func (p *openAICompatible) BatchSize() int {
	return p.batchSize
}

func (p *openAICompatible) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	batches := (len(texts) + p.batchSize - 1) / p.batchSize
	if batches < 1 {
		batches = 1
	}
	slog.DebugContext(ctx, "embedding texts", "count", len(texts), "batches", batches, "batch_size", p.batchSize, "model", p.model, "url", p.url)
	var all [][]float32

	for i := 0; i < len(texts); i += p.batchSize {
		end := i + p.batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batchNum := i/p.batchSize + 1

		previews := make([]string, 0, end-i)
		for _, text := range texts[i:end] {
			preview := strings.ReplaceAll(text, "\n", "\\n")
			previews = append(previews, truncateForDebug(preview, 120))
		}
		slog.DebugContext(ctx, "embedding batch", "batch", batchNum, "of", batches, "size", end-i, "previews", previews)

		batch, err := p.embedBatch(ctx, texts[i:end])
		if err != nil {
			return nil, fmt.Errorf("embed batch %d-%d: %w", i, end, err)
		}
		all = append(all, batch...)
	}

	return all, nil
}

func (p *openAICompatible) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	body := map[string]any{
		"model": p.model,
		"input": texts,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	slog.DebugContext(ctx, "sending embeddings request", "url", p.url+"/embeddings", "model", p.model, "body_bytes", len(jsonBody), "body", truncateForDebug(string(jsonBody), debugBodyMaxLen))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url+"/embeddings", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	slog.DebugContext(ctx, "embeddings response received", "status", resp.StatusCode, "body_bytes", len(respBody), "body", truncateForDebug(string(respBody), debugBodyMaxLen))

	if resp.StatusCode != http.StatusOK {
		slog.ErrorContext(ctx, "embeddings request failed", "status", resp.StatusCode, "body", string(respBody))
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed embeddingsResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	out := make([][]float32, 0, len(parsed.Data))
	for _, item := range parsed.Data {
		vec := make([]float32, len(item.Embedding))
		for i, v := range item.Embedding {
			vec[i] = float32(v)
		}
		out = append(out, vec)
	}

	if len(out) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(out))
	}

	dims := 0
	if len(out) > 0 {
		dims = len(out[0])
	}
	slog.DebugContext(ctx, "embeddings received", "count", len(out), "dimensions", dims)

	return out, nil
}

type embeddingsResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}
