package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
)

const (
	reset  = "\033[0m"
	blue   = "\033[34m"
	green  = "\033[32m"
	yellow = "\033[33m"
	red    = "\033[31m"
)

// Handler is a colorful, concise slog handler.
// Format: INF | message key=value ...
type Handler struct {
	writer io.Writer
	level  slog.Level
	mu     sync.Mutex
	attrs  []slog.Attr
	groups []string
}

// NewHandler creates a new handler that writes to w and filters by level.
func NewHandler(w io.Writer, level slog.Level) *Handler {
	return &Handler{writer: w, level: level}
}

// Enabled reports whether the handler handles records at the given level.
func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle formats and writes a log record.
func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	color, label := formatLevel(r.Level)

	buf := make([]byte, 0, 128+len(r.Message))
	buf = fmt.Appendf(buf, "%s%s%s | %s", color, label, reset, r.Message)

	r.Attrs(func(a slog.Attr) bool {
		buf = appendAttr(buf, a, h.groups)
		return true
	})

	for _, a := range h.attrs {
		buf = appendAttr(buf, a, h.groups)
	}

	buf = append(buf, '\n')

	h.mu.Lock()
	defer h.mu.Unlock()

	_, err := h.writer.Write(buf)
	return err
}

// WithAttrs returns a new handler with the given attributes appended.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}

	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)

	return &Handler{
		writer: h.writer,
		level:  h.level,
		attrs:  newAttrs,
		groups: h.groups,
	}
}

// WithGroup returns a new handler with the given group appended.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name

	return &Handler{
		writer: h.writer,
		level:  h.level,
		attrs:  h.attrs,
		groups: newGroups,
	}
}

func formatLevel(level slog.Level) (string, string) {
	switch {
	case level >= slog.LevelError:
		return red, "ERR"
	case level >= slog.LevelWarn:
		return yellow, "WRN"
	case level >= slog.LevelInfo:
		return green, "INF"
	default:
		return blue, "DBG"
	}
}

func appendAttr(buf []byte, a slog.Attr, groups []string) []byte {
	if a.Equal(slog.Attr{}) {
		return buf
	}

	if a.Value.Kind() == slog.KindGroup {
		if a.Key != "" {
			groups = append(groups, a.Key)
		}
		for _, attr := range a.Value.Group() {
			buf = appendAttr(buf, attr, groups)
		}
		return buf
	}

	key := a.Key
	for _, g := range groups {
		key = g + "." + key
	}

	return fmt.Appendf(buf, " %s=%s", key, formatValue(a.Value))
}

func formatValue(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		s := v.String()
		if strings.ContainsAny(s, " \t\n\r\"=") {
			return strconv.Quote(s)
		}
		return s
	case slog.KindInt64, slog.KindUint64, slog.KindFloat64, slog.KindBool, slog.KindDuration, slog.KindTime:
		return v.String()
	case slog.KindAny:
		a := v.Any()
		if s, ok := a.(fmt.Stringer); ok {
			return s.String()
		}
		return fmt.Sprintf("%v", a)
	case slog.KindLogValuer:
		return formatValue(v.Resolve())
	default:
		return v.String()
	}
}
