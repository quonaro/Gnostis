package log

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestHandler(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewHandler(&buf, slog.LevelInfo))

	logger.Info("hello", slog.String("key", "value"), slog.Int("count", 17))

	got := buf.String()
	if strings.Contains(got, "time=") {
		t.Errorf("output should not contain time: %q", got)
	}
	if !strings.Contains(got, "hello key=value count=17") {
		t.Errorf("expected message and attrs, got %q", got)
	}
}

func TestHandlerLevelColors(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewHandler(&buf, slog.LevelDebug))

	logger.Debug("debug")
	logger.Info("info")
	logger.Warn("warn")
	logger.Error("error")

	got := buf.String()
	for _, want := range []string{"DBG", "INF", "WRN", "ERR"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %s in output, got %q", want, got)
		}
	}
}
