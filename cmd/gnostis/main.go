package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/quonaro/lota/engine"

	"github.com/quonaro/gnostis/internal/log"
)

//go:embed cli.yml
var cliYAML []byte

// version is set by the build linker to the short git commit hash.
var version string

// logOutput is the current shared log destination; it is set to both stderr and
// ~/.gnostis/gnostis.log when the binary starts.
var logOutput io.Writer = os.Stderr

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

func setupLogOutput() io.Writer {
	home, err := os.UserHomeDir()
	if err != nil {
		return os.Stderr
	}
	dir := filepath.Join(home, ".gnostis")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return os.Stderr
	}
	path := filepath.Join(dir, "gnostis.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return os.Stderr
	}
	return io.MultiWriter(os.Stderr, f)
}

func main() {
	logOutput = setupLogOutput()
	logger := slog.New(log.NewHandler(logOutput, slog.LevelInfo))
	slog.SetDefault(logger)

	builder := engine.NewBuilder("gnostis", cliYAML)
	builder.RegisterNative("run", runHandler)
	builder.RegisterNative("status", indexStatusHandler)
	builder.RegisterNative("rebuild", indexRebuildHandler)
	builder.RegisterNative("validate", configValidateHandler)
	builder.RegisterNative("show", configShowHandler)
	builder.RegisterNative("discover", configDiscoverHandler)

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
