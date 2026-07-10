package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/quonaro/lota/engine"

	"github.com/quonaro/gnostis/internal/app"
	"github.com/quonaro/gnostis/internal/config"
	"github.com/quonaro/gnostis/internal/log"
)

func runHandler(_ context.Context, nctx engine.NativeContext) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	return runApp(cfg, nctx.Stdout)
}

func versionHandler(_ context.Context, nctx engine.NativeContext) error {
	_, _ = fmt.Fprintln(nctx.Stdout, version)
	return nil
}

func loadConfig() (config.Config, error) {
	cfgPath := os.Getenv("GNOSTIS_CONFIG")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %w", err)
	}

	if cfg.LogLevel != "" {
		level, err := parseLogLevel(cfg.LogLevel)
		if err != nil {
			return config.Config{}, fmt.Errorf("parse log level: %w", err)
		}
		slog.SetDefault(slog.New(log.NewHandler(os.Stderr, level)))
	}

	return cfg, nil
}

func runApp(cfg config.Config, _ io.Writer) error {
	application, err := app.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize app: %w", err)
	}

	ctx := context.Background()
	if err := application.Run(ctx); err != nil {
		return fmt.Errorf("run app: %w", err)
	}
	return nil
}

func confirm(writer io.Writer, prompt string) bool {
	_, _ = fmt.Fprintf(writer, "%s [y/n]: ", prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes"
}
