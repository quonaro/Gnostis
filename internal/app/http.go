package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func (a *App) runHTTP(ctx context.Context, ready chan<- struct{}) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	startErr := make(chan error, 1)
	go func() {
		if err := a.mcp.StartHTTP(ctx, a.cfg.MCP.Address, a.cfg.MCP.Token); err != nil {
			slog.ErrorContext(ctx, "mcp http server stopped", "error", err)
			startErr <- err
			cancel()
		}
	}()

	if err := a.waitForListener(ctx, a.cfg.MCP.Address, startErr, ready); err != nil {
		return err
	}

	select {
	case err := <-startErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("mcp http server: %w", err)
		}
	case <-ctx.Done():
	case <-sigChan:
		slog.InfoContext(ctx, "shutting down")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := a.mcp.StopHTTP(shutdownCtx); err != nil {
		slog.ErrorContext(ctx, "stop mcp http server", "error", err)
	}
	return nil
}

func (a *App) waitForListener(ctx context.Context, addr string, startErr <-chan error, ready chan<- struct{}) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			select {
			case err := <-startErr:
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					return fmt.Errorf("mcp http server: %w", err)
				}
			default:
			}
			return fmt.Errorf("mcp http server not ready on %s: %w", addr, ctx.Err())
		case err := <-startErr:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("mcp http server: %w", err)
			}
			return nil
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				close(ready)
				slog.InfoContext(ctx, "mcp http server ready", "address", addr)
				return nil
			}
		}
	}
}
