package main

import (
	"context"
	"fmt"

	"github.com/quonaro/lota/engine"
	"gopkg.in/yaml.v3"
)

func configShowHandler(_ context.Context, nctx engine.NativeContext) error {
	cfg, restore, err := loadConfigForCLI()
	if err != nil {
		return err
	}
	defer restore()

	cfg.Embeddings.APIKey = maskValue(cfg.Embeddings.APIKey)

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	_, _ = nctx.Stdout.Write(data)
	return nil
}

func maskValue(v string) string {
	if v == "" {
		return ""
	}
	return "***"
}
