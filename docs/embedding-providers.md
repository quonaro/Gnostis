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

## Model recommendations

| Model                           | Provider | Russian   | Code | Speed  |
| ------------------------------- | -------- | --------- | ---- | ------ |
| `nomic-embed-text`              | Ollama   | good      | good | fast   |
| `intfloat/multilingual-e5-base` | Ollama   | excellent | good | medium |
| `text-embedding-3-small`        | OpenAI   | excellent | good | fast   |
