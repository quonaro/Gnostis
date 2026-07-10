# Embedding Providers

Gnostis uses a provider interface so you can switch between local and remote embedding models without changing the core code.

## Interface

```go
type Provider interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    ModelName() string
}
```

## Ollama

Recommended for local-first usage. Any model exposed through Ollama's OpenAI-compatible endpoint works.

```yaml
embeddings:
  provider: ollama
  url: http://localhost:11434/v1
  model: nomic-embed-text
```

Install and run:

```bash
ollama pull nomic-embed-text
ollama serve
```

## OpenAI-compatible

Works with OpenAI, OpenRouter, LocalAI, LM Studio, and any other `/v1/embeddings` endpoint.

```yaml
embeddings:
  provider: openai
  url: https://api.openai.com/v1
  model: text-embedding-3-small
  api_key: ${OPENAI_API_KEY}
```

## ONNX (local, no external server)

Runs a Hugging Face ONNX model directly using the local ONNX Runtime. This removes the dependency on Ollama or a cloud API after the first model download.

```yaml
embeddings:
  provider: onnx
  model: sentence-transformers/all-MiniLM-L6-v2
  model_path: ${HOME}/.gnostis/models/all-MiniLM-L6-v2
  runtime_path: /usr/lib/libonnxruntime.so
  batch_size: 32
```

- `model` is a Hugging Face repository that exposes `onnx/model.onnx` and `tokenizer.json`.
- `model_path` is the directory where the model files are cached (defaults to `${data_dir}/models/{sanitized_model}`).
- `runtime_path` is the path to the ONNX Runtime shared library. If omitted, the `ONNXRUNTIME_LIB_PATH` environment variable is used.

On first run Gnostis downloads the model and tokenizer from Hugging Face. The default model (`all-MiniLM-L6-v2`) produces 384-dimensional vectors. Changing the model requires a full index rebuild because the vector dimensions differ.

## Model recommendations

| Model                           | Provider | Russian   | Code | Speed  |
| ------------------------------- | -------- | --------- | ---- | ------ |
| `nomic-embed-text`              | Ollama   | good      | good | fast   |
| `intfloat/multilingual-e5-base` | Ollama   | excellent | good | medium |
| `text-embedding-3-small`        | OpenAI   | excellent | good | fast   |
