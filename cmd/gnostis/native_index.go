package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/quonaro/lota/engine"

	"github.com/quonaro/gnostis/internal/app"
)

func indexStatusHandler(_ context.Context, nctx engine.NativeContext) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	application, err := app.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize app: %w", err)
	}

	names, count := application.Status()
	provider, model, symbols := application.Info()
	_, _ = fmt.Fprintf(nctx.Stdout, "provider: %s\n", provider)
	_, _ = fmt.Fprintf(nctx.Stdout, "model: %s\n", model)
	_, _ = fmt.Fprintf(nctx.Stdout, "projects: %s\n", strings.Join(names, ", "))
	_, _ = fmt.Fprintf(nctx.Stdout, "chunks: %d\n", count)
	_, _ = fmt.Fprintf(nctx.Stdout, "symbols: %d\n", symbols)
	return nil
}

func indexRebuildHandler(_ context.Context, nctx engine.NativeContext) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	if !confirm(nctx.Stdout, "This will delete the existing index and rebuild it. Continue?") {
		_, _ = fmt.Fprintln(nctx.Stdout, "cancelled")
		return nil
	}

	if err := os.RemoveAll(cfg.DataDir); err != nil {
		return fmt.Errorf("remove data dir: %w", err)
	}

	application, err := app.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize app: %w", err)
	}

	if err := application.InitialIndex(context.Background()); err != nil {
		return fmt.Errorf("rebuild index: %w", err)
	}

	_, _ = fmt.Fprintln(nctx.Stdout, "index rebuilt")
	return nil
}
