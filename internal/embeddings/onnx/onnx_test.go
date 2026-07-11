//go:build !no_onnx

package onnx

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureModelAt_DownloadsMissingFiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/test/model/resolve/main/onnx/model.onnx", "/test/model/resolve/main/tokenizer.json":
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "dummy")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	modelPath, tokenizerPath, err := EnsureModelAt(context.Background(), "test/model", dir, srv.URL)
	if err != nil {
		t.Fatalf("EnsureModelAt: %v", err)
	}

	if _, err := os.Stat(modelPath); err != nil {
		t.Errorf("model file missing: %v", err)
	}
	if _, err := os.Stat(tokenizerPath); err != nil {
		t.Errorf("tokenizer file missing: %v", err)
	}

	// Existing files should be reused without another download.
	_, _, err = EnsureModelAt(context.Background(), "test/model", dir, srv.URL)
	if err != nil {
		t.Fatalf("EnsureModelAt second call: %v", err)
	}
}

func TestEnsureModelAt_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	if _, _, err := EnsureModelAt(context.Background(), "test/model", dir, srv.URL); err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestNew_MissingFiles(t *testing.T) {
	if !runtimeAvailable(t) {
		t.Skip("onnx runtime not available")
	}

	dir := t.TempDir()
	_, err := New(filepath.Join(dir, "model.onnx"), filepath.Join(dir, "tokenizer.json"), "")
	if err == nil {
		t.Fatal("expected error for missing files")
	}
}

func TestNew_InvalidModel(t *testing.T) {
	if !runtimeAvailable(t) {
		t.Skip("onnx runtime not available")
	}

	dir := t.TempDir()
	modelPath := filepath.Join(dir, "model.onnx")
	tokenizerPath := filepath.Join(dir, "tokenizer.json")
	if err := os.WriteFile(modelPath, []byte("not a model"), 0o600); err != nil {
		t.Fatalf("write model: %v", err)
	}
	if err := os.WriteFile(tokenizerPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write tokenizer: %v", err)
	}

	_, err := New(modelPath, tokenizerPath, "")
	if err == nil {
		t.Fatal("expected error for invalid model")
	}
}

func TestProvider_ModelName(t *testing.T) {
	p := &Provider{modelName: "onnx:test"}
	if got := p.ModelName(); got != "onnx:test" {
		t.Errorf("ModelName = %q, want onnx:test", got)
	}
}

func TestPoolAndNormalize(t *testing.T) {
	batchSize := 2
	seqLength := 3
	dim := 4

	// First batch: all tokens active. Second batch: one token padded.
	hidden := []float32{
		// Batch 0: tokens [1,1,1,1], [2,2,2,2], [3,3,3,3]
		1, 1, 1, 1,
		2, 2, 2, 2,
		3, 3, 3, 3,
		// Batch 1: tokens [4,4,4,4], [5,5,5,5], [0,0,0,0] (padded)
		4, 4, 4, 4,
		5, 5, 5, 5,
		0, 0, 0, 0,
	}
	mask := []int64{
		1, 1, 1,
		1, 1, 0,
	}

	results := poolAndNormalize(batchSize, seqLength, dim, hidden, mask)
	if len(results) != batchSize {
		t.Fatalf("got %d results, want %d", len(results), batchSize)
	}

	// Batch 0 mean is [2,2,2,2]; after L2 norm each value is 2/sqrt(16)=0.5.
	for _, v := range results[0] {
		if v != 0.5 {
			t.Errorf("batch 0 value = %v, want 0.5", v)
		}
	}

	// Batch 1 mean is [4.5,4.5,4.5,4.5]; after L2 norm each value is 4.5/sqrt(81)=0.5.
	for _, v := range results[1] {
		if v != 0.5 {
			t.Errorf("batch 1 value = %v, want 0.5", v)
		}
	}
}

func runtimeAvailable(t *testing.T) bool {
	t.Helper()
	if err := initialize(""); err != nil {
		t.Logf("onnx runtime not available: %v", err)
		return false
	}
	return true
}
