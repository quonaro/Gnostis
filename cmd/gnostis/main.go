package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/quonaro/lota/engine"

	"github.com/quonaro/gnostis/internal/log"
)

// version is injected at build time via -ldflags "-X main.version=<hash>".
var version string

//go:embed cli.yml
var cliYAML []byte

func parseLogLevel(level string) (slog.Level, error) {
	switch level {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unsupported log level: %s", level)
	}
}

func main() {
	logger := slog.New(log.NewHandler(os.Stderr, slog.LevelInfo))
	slog.SetDefault(logger)

	builder := engine.NewBuilder("gnostis", cliYAML)
	builder.RegisterNative("run", runHandler)
	builder.RegisterNative("version", versionHandler)
	builder.RegisterNative("index.rebuild", indexRebuildHandler)
	builder.RegisterNative("index.status", indexStatusHandler)
	builder.RegisterNative("config.validate", configValidateHandler)
	builder.RegisterNative("config.show", configShowHandler)
	builder.RegisterNative("config.discover", configDiscoverHandler)

	app, err := builder.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	if len(os.Args) < 2 {
		app.PrintHelp()
		return
	}

	if err := app.Run(context.Background(), os.Args[1:]); err != nil {
		var groupErr *engine.GroupError
		if errors.As(err, &groupErr) {
			app.PrintGroupHelp(groupErr.Groups)
			return
		}
		fmt.Fprintf(os.Stderr, "run: %v\n", err)
		os.Exit(1)
	}
}
