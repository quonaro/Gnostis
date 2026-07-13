package app

import (
	"context"
	"log/slog"
	"os"
)

func (a *App) cleanupDeletedFiles(ctx context.Context) {
	var toDelete []string
	for _, path := range a.store.Paths() {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			slog.InfoContext(ctx, "removing deleted file from index", "path", path)
			toDelete = append(toDelete, path)
			a.symbolIndex.RemoveByPath(path)
		}
	}
	_ = a.store.DeleteByPaths(ctx, toDelete)
}
