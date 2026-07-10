package onnx

// Embedding inference logic adapted from github.com/clems4ever/all-minilm-l6-v2-go
// (Apache-2.0 license).

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"
	rt "github.com/yalue/onnxruntime_go"
)

const (
	defaultHiddenSize = 384

	inputIDsName      = "input_ids"
	attentionMaskName = "attention_mask"
	tokenTypeIDsName  = "token_type_ids"
	outputName        = "sentence_embedding"
)

var (
	initOnce sync.Once
	initErr  error
)

// Provider implements embeddings.Provider using a local ONNX model.
type Provider struct {
	modelName string
	session   *rt.DynamicAdvancedSession
	tokenizer tokenizer.Tokenizer
	dim       int
}

// New loads an ONNX model and its tokenizer from the local filesystem.
// If runtimePath is empty, the ONNXRUNTIME_LIB_PATH environment variable is used.
func New(modelPath, tokenizerPath, runtimePath string) (*Provider, error) {
	if err := initialize(runtimePath); err != nil {
		return nil, fmt.Errorf("initialize onnx runtime: %w", err)
	}

	tk, err := pretrained.FromFile(tokenizerPath)
	if err != nil {
		return nil, fmt.Errorf("load tokenizer: %w", err)
	}

	session, err := rt.NewDynamicAdvancedSession(
		modelPath,
		[]string{inputIDsName, attentionMaskName, tokenTypeIDsName},
		[]string{outputName},
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("load onnx session: %w", err)
	}

	return &Provider{
		modelName: "onnx:" + filepath.Base(filepath.Dir(modelPath)),
		session:   session,
		tokenizer: *tk,
		dim:       defaultHiddenSize,
	}, nil
}

func destroyTensor(t interface{ Destroy() error }) {
	_ = t.Destroy()
}

func initialize(runtimePath string) error {
	initOnce.Do(func() {
		if runtimePath != "" {
			rt.SetSharedLibraryPath(runtimePath)
		} else if p, ok := os.LookupEnv("ONNXRUNTIME_LIB_PATH"); ok {
			rt.SetSharedLibraryPath(p)
		}
		initErr = rt.InitializeEnvironment()
	})
	return initErr
}

// Close releases the ONNX session.
func (p *Provider) Close() error {
	if p.session != nil {
		return p.session.Destroy()
	}
	return nil
}

// ModelName returns the provider model identifier.
func (p *Provider) ModelName() string {
	return p.modelName
}

// Embed converts a batch of texts into embedding vectors.
func (p *Provider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	inputBatch := make([]tokenizer.EncodeInput, len(texts))
	for i, s := range texts {
		inputBatch[i] = tokenizer.NewSingleEncodeInput(tokenizer.NewInputSequence(s))
	}

	encodings, err := p.tokenizer.EncodeBatch(inputBatch, true)
	if err != nil {
		return nil, fmt.Errorf("tokenize: %w", err)
	}
	if len(encodings) == 0 {
		return nil, fmt.Errorf("empty encodings")
	}

	batchSize := len(encodings)
	seqLength := len(encodings[0].Ids)
	inputShape := rt.NewShape(int64(batchSize), int64(seqLength))

	inputIDs := make([]int64, batchSize*seqLength)
	attentionMask := make([]int64, batchSize*seqLength)
	tokenTypeIDs := make([]int64, batchSize*seqLength)

	for b := range batchSize {
		for i, id := range encodings[b].Ids {
			inputIDs[b*seqLength+i] = int64(id)
		}
		for i, mask := range encodings[b].AttentionMask {
			attentionMask[b*seqLength+i] = int64(mask)
		}
		for i, typeID := range encodings[b].TypeIds {
			tokenTypeIDs[b*seqLength+i] = int64(typeID)
		}
	}

	inputIDsTensor, err := rt.NewTensor(inputShape, inputIDs)
	if err != nil {
		return nil, fmt.Errorf("create input_ids tensor: %w", err)
	}
	defer destroyTensor(inputIDsTensor)

	attentionMaskTensor, err := rt.NewTensor(inputShape, attentionMask)
	if err != nil {
		return nil, fmt.Errorf("create attention_mask tensor: %w", err)
	}
	defer destroyTensor(attentionMaskTensor)

	tokenTypeIDsTensor, err := rt.NewTensor(inputShape, tokenTypeIDs)
	if err != nil {
		return nil, fmt.Errorf("create token_type_ids tensor: %w", err)
	}
	defer destroyTensor(tokenTypeIDsTensor)

	outputShape := rt.NewShape(int64(batchSize), int64(p.dim))
	outputTensor, err := rt.NewEmptyTensor[float32](outputShape)
	if err != nil {
		return nil, fmt.Errorf("create output tensor: %w", err)
	}
	defer destroyTensor(outputTensor)

	if err := p.session.Run(
		[]rt.Value{inputIDsTensor, attentionMaskTensor, tokenTypeIDsTensor},
		[]rt.Value{outputTensor},
	); err != nil {
		return nil, fmt.Errorf("run session: %w", err)
	}

	flat := outputTensor.GetData()
	expected := batchSize * p.dim
	if len(flat) != expected {
		return nil, fmt.Errorf("unexpected output size: got %d, want %d", len(flat), expected)
	}

	results := make([][]float32, batchSize)
	for i := range batchSize {
		start := i * p.dim
		end := start + p.dim
		results[i] = make([]float32, p.dim)
		copy(results[i], flat[start:end])
	}
	return results, nil
}
