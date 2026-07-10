package embeddings

import (
	"context"
	"fmt"
	"strings"

	"github.com/quonaro/gnostis/internal/config"
	"github.com/quonaro/gnostis/internal/embeddings/onnx"
)

// Provider converts texts into embedding vectors.
type Provider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	ModelName() string
}

// New creates a Provider from the embeddings configuration.
func New(cfg config.Embeddings) (Provider, error) {
	switch strings.ToLower(cfg.Provider) {
	case "ollama":
		return newOpenAICompatible(cfg.URL, cfg.Model, "", cfg.BatchSize), nil
	case "openai":
		return newOpenAICompatible(cfg.URL, cfg.Model, cfg.APIKey, cfg.BatchSize), nil
	case "onnx":
		modelPath, tokenizerPath, err := onnx.EnsureModel(context.Background(), cfg.Model, cfg.ModelPath)
		if err != nil {
			return nil, fmt.Errorf("ensure onnx model: %w", err)
		}
		return onnx.New(modelPath, tokenizerPath, cfg.RuntimePath)
	default:
		return nil, fmt.Errorf("unknown embeddings provider: %s", cfg.Provider)
	}
}
