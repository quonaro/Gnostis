package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/quonaro/lota/engine"

	"github.com/quonaro/gnostis/internal/chat_providers"
	"github.com/quonaro/gnostis/internal/chat_providers/all"
)

func decryptCascadeHandler(_ context.Context, nctx engine.NativeContext) error {
	_, restore, err := loadConfigForCLI()
	if err != nil {
		return err
	}
	defer restore()

	outputDir := nctx.Vars["OUTPUT_DIR"]
	if outputDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("user home dir: %w", err)
		}
		outputDir = filepath.Join(home, ".gnostis", "data", "dialogues")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	registry := all.NewRegistry()
	exporter := chat_providers.Exporter{MinUserMessageLength: 10}

	exported, err := exportAllChat(registry, outputDir, exporter, nctx.Stdout)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(nctx.Stdout, "Exported %d session(s) to %s\n", exported, outputDir)
	return nil
}

func exportAllChat(registry *all.Registry, outputDir string, exporter chat_providers.Exporter, out io.Writer) (int, error) {
	exported := 0
	for _, p := range registry.Providers() {
		for _, src := range p.Discover() {
			entries, err := os.ReadDir(src)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return exported, fmt.Errorf("read source dir %s: %w", src, err)
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				path := filepath.Join(src, entry.Name())
				plaintext, err := p.Decrypt(path)
				if err != nil {
					_, _ = fmt.Fprintf(out, "[error] %s: %v\n", path, err)
					continue
				}
				mdPath, err := exporter.ExportSession(p, path, outputDir, plaintext)
				if err != nil {
					_, _ = fmt.Fprintf(out, "[error] %s: %v\n", path, err)
					continue
				}
				_, _ = fmt.Fprintf(out, "[ok] %s -> %s\n", path, mdPath)
				exported++
			}
		}
	}
	return exported, nil
}
