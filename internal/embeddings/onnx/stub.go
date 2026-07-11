//go:build no_onnx

package onnx

import (
	"context"
	"errors"
)

// DefaultModel is the default Hugging Face ONNX embedding model.
const DefaultModel = "sentence-transformers/all-MiniLM-L6-v2"

// Provider is a stub implementation used when the no_onnx build tag is set.
type Provider struct{}

// New always returns an error because ONNX support is disabled at build time.
func New(_, _, _ string) (*Provider, error) {
	return nil, errors.New("onnx provider is disabled in this build")
}

// Close is a no-op for the stub provider.
func (p *Provider) Close() error { return nil }

// ModelName returns a placeholder identifier.
func (p *Provider) ModelName() string { return "onnx:disabled" }

// Embed always returns an error because ONNX support is disabled at build time.
func (p *Provider) Embed(context.Context, []string) ([][]float32, error) {
	return nil, errors.New("onnx provider is disabled in this build")
}

// EnsureModel always returns an error because ONNX support is disabled at build time.
func EnsureModel(context.Context, string, string) (string, string, error) {
	return "", "", errors.New("onnx provider is disabled in this build")
}

// EnsureModelAt always returns an error because ONNX support is disabled at build time.
func EnsureModelAt(context.Context, string, string, string) (string, string, error) {
	return "", "", errors.New("onnx provider is disabled in this build")
}
