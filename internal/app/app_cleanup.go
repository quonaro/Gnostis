package app

import (
	"context"
	"log/slog"
	"os"
)

func (a *App) cleanupDeletedFiles(ctx context.Context) {
	for _, path := range a.store.Paths() {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			slog.InfoContext(ctx, "removing deleted file from index", "path", path)
			_ = a.store.DeleteByPath(ctx, path)
			a.symbolIndex.RemoveByPath(path)
		}
	}
}
