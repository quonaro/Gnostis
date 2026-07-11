//go:build !no_onnx

package onnx

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	modelFile     = "model.onnx"
	tokenizerFile = "tokenizer.json"

	// DefaultModel is the default Hugging Face ONNX embedding model.
	DefaultModel = "sentence-transformers/all-MiniLM-L6-v2"
)

var defaultHFBaseURL = "https://huggingface.co"

// EnsureModel makes sure the ONNX model and tokenizer files exist in dir.
// If model is empty, DefaultModel is used. Missing files are downloaded from
// Hugging Face.
func EnsureModel(ctx context.Context, model, dir string) (modelPath, tokenizerPath string, err error) {
	return EnsureModelAt(ctx, model, dir, defaultHFBaseURL)
}

// EnsureModelAt is the testable variant of EnsureModel with a configurable
// download base URL.
func EnsureModelAt(ctx context.Context, model, dir, baseURL string) (modelPath, tokenizerPath string, err error) {
	if model == "" {
		model = DefaultModel
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", "", fmt.Errorf("create model directory: %w", err)
	}

	modelPath = filepath.Join(dir, modelFile)
	tokenizerPath = filepath.Join(dir, tokenizerFile)
	base := fmt.Sprintf("%s/%s/resolve/main", baseURL, model)

	if err := ensureFile(ctx, modelPath, base+"/onnx/"+modelFile, 60*time.Minute); err != nil {
		return "", "", fmt.Errorf("download model: %w", err)
	}
	if err := ensureFile(ctx, tokenizerPath, base+"/"+tokenizerFile, 5*time.Minute); err != nil {
		return "", "", fmt.Errorf("download tokenizer: %w", err)
	}
	return modelPath, tokenizerPath, nil
}

func ensureFile(ctx context.Context, path, url string, timeout time.Duration) error {
	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		return nil
	}

	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: %s", url, resp.Status)
	}

	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}
