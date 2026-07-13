package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/quonaro/lota/engine"

	"github.com/quonaro/gnostis/internal/app"
	"github.com/quonaro/gnostis/internal/progress"
	"github.com/quonaro/gnostis/internal/stats"
)

func indexStatusHandler(ctx context.Context, nctx engine.NativeContext) error {
	cfg, restore, err := loadConfigForCLI()
	if err != nil {
		return err
	}
	defer restore()

	application, err := app.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize app: %w", err)
	}

	names, count := application.Status()
	provider, model, symbols := application.Info()
	_, _ = fmt.Fprintf(nctx.Stdout, "provider: %s\n", provider)
	_, _ = fmt.Fprintf(nctx.Stdout, "model: %s\n", model)
	_, _ = fmt.Fprintf(nctx.Stdout, "chunks: %d\n", count)
	_, _ = fmt.Fprintf(nctx.Stdout, "symbols: %d\n", symbols)

	p, err := application.ProgressState()
	if err != nil {
		return fmt.Errorf("load progress: %w", err)
	}

	switch p.Status {
	case progress.StatusRunning:
		_, _ = fmt.Fprintf(nctx.Stdout, "rebuild: running\n")
		_, _ = fmt.Fprintf(nctx.Stdout, "phase: %s project %q\n", p.Phase, p.Project)
		_, _ = fmt.Fprintf(nctx.Stdout, "files: %d/%d\n", p.DoneFiles, p.TotalFiles)
		_, _ = fmt.Fprintf(nctx.Stdout, "chunks: %d/%d\n", p.DoneChunks, p.TotalChunks)
	case progress.StatusError:
		_, _ = fmt.Fprintf(nctx.Stdout, "rebuild: error: %s\n", p.Error)
	case progress.StatusDone:
		_, _ = fmt.Fprintf(nctx.Stdout, "rebuild: done\n")
	default:
		_, _ = fmt.Fprintf(nctx.Stdout, "rebuild: idle\n")
	}

	projectStats, err := application.ProjectStats(ctx)
	if err != nil {
		return fmt.Errorf("load project stats: %w", err)
	}

	overallLast := latestIndexed(projectStats)
	if overallLast.IsZero() && !p.UpdatedAt.IsZero() {
		overallLast = p.UpdatedAt
	}
	if !overallLast.IsZero() {
		_, _ = fmt.Fprintf(nctx.Stdout, "last indexed: %s\n", overallLast.Format(time.RFC3339))
	}

	_, _ = fmt.Fprintln(nctx.Stdout, "\nprojects:")
	for _, name := range names {
		stat := projectStats[name]
		last := "never"
		if !stat.LastIndexedAt.IsZero() {
			last = stat.LastIndexedAt.Format(time.RFC3339)
		}
		_, _ = fmt.Fprintf(nctx.Stdout, "  %s: chunks=%d last_indexed=%s\n", name, stat.Chunks, last)
	}
	return nil
}

func latestIndexed(projectStats map[string]stats.Project) time.Time {
	var latest time.Time
	for _, s := range projectStats {
		if s.LastIndexedAt.After(latest) {
			latest = s.LastIndexedAt
		}
	}
	return latest
}

func indexRebuildHandler(_ context.Context, nctx engine.NativeContext) error {
	cfg, restore, err := loadConfigForCLI()
	if err != nil {
		return err
	}
	defer restore()

	paths := strings.Fields(nctx.Args["paths"])
	detach := nctx.Args["detach"] == "true"

	if detach {
		running, err := isRebuildRunning(cfg.DataDir)
		if err != nil {
			return err
		}
		if running {
			return fmt.Errorf("a rebuild is already running; check status with 'gnostis status'")
		}
		pid, err := spawnDetachedRebuild(cfg.DataDir, paths...)
		if err != nil {
			return fmt.Errorf("spawn detached rebuild: %w", err)
		}
		_, _ = fmt.Fprintf(nctx.Stdout, "rebuild started in background (pid: %d)\n", pid)
		_, _ = fmt.Fprintf(nctx.Stdout, "log: %s\n", filepath.Join(cfg.DataDir, "rebuild.log"))
		return nil
	}

	running, err := isRebuildRunning(cfg.DataDir)
	if err != nil {
		return err
	}
	if running {
		return fmt.Errorf("a rebuild is already running; use -d to run in background or check status with 'gnostis status'")
	}

	if len(paths) == 0 {
		if isInteractive() && !confirm(nctx.Stdout, "This will delete the existing index and rebuild it. Continue?") {
			_, _ = fmt.Fprintln(nctx.Stdout, "cancelled")
			return nil
		}

		if err := os.RemoveAll(cfg.DataDir); err != nil {
			return fmt.Errorf("remove data dir: %w", err)
		}
	}

	application, err := app.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize app: %w", err)
	}
	if f, ok := nctx.Stdout.(*os.File); ok && isatty.IsTerminal(f.Fd()) {
		application.ProgressWriter = f
	}

	if len(paths) == 0 {
		if err := application.InitialIndex(context.Background()); err != nil {
			application.FailProgress(err)
			return fmt.Errorf("rebuild index: %w", err)
		}

		_, _ = fmt.Fprintln(nctx.Stdout, "index rebuilt")
		return nil
	}

	if err := application.RebuildPaths(context.Background(), paths); err != nil {
		return fmt.Errorf("rebuild paths: %w", err)
	}

	_, _ = fmt.Fprintf(nctx.Stdout, "rebuilt %d path(s)\n", len(paths))
	return nil
}

func isInteractive() bool {
	return isatty.IsTerminal(os.Stdin.Fd())
}

func isRebuildRunning(dataDir string) (bool, error) {
	p := progress.New(filepath.Join(dataDir, "indexing-progress.json"))
	s, err := p.Load()
	if err != nil {
		return false, fmt.Errorf("load progress: %w", err)
	}
	if s.Status != progress.StatusRunning {
		return false, nil
	}
	if s.PID == 0 {
		// Legacy progress file without PID tracking. Reset it rather than
		// blocking forever on a potentially stale lock.
		_ = p.Reset()
		return false, nil
	}
	if s.PID == os.Getpid() {
		return true, nil
	}
	proc, err := os.FindProcess(s.PID)
	if err != nil || proc.Signal(syscall.Signal(0)) != nil {
		_ = p.Reset()
		return false, nil
	}
	return true, nil
}

func spawnDetachedRebuild(dataDir string, paths ...string) (int, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return 0, fmt.Errorf("create data dir: %w", err)
	}

	logPath := filepath.Join(dataDir, "rebuild.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open log file: %w", err)
	}
	defer func() { _ = logFile.Close() }()

	args := append([]string{"rebuild"}, paths...)

	cmd := exec.Command(os.Args[0], args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start detached process: %w", err)
	}

	return cmd.Process.Pid, nil
}
