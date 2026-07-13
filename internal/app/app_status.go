package app

import (
	"context"
	"fmt"

	"github.com/quonaro/gnostis/internal/progress"
	"github.com/quonaro/gnostis/internal/project"
	"github.com/quonaro/gnostis/internal/stats"
)

// Status returns the configured project names and current chunk count.
func (a *App) Status() ([]string, int) {
	a.rebuildMu.RLock()
	names := make([]string, len(a.projects))
	for i, p := range a.projects {
		names[i] = p.Name
	}
	a.rebuildMu.RUnlock()
	return names, a.store.Count()
}

// Info returns runtime metadata about the active provider and index.
func (a *App) Info() (provider, model string, symbols int) {
	a.rebuildMu.RLock()
	model = a.cfg.Embeddings.Model
	a.rebuildMu.RUnlock()
	return a.provider.ModelName(), model, a.symbolIndex.Count()
}

// ProgressState returns the persisted rebuild progress state.
func (a *App) ProgressState() (progress.State, error) {
	if a.progress == nil {
		return progress.State{Status: progress.StatusIdle}, nil
	}
	return a.progress.Load()
}

// ProjectStats returns the current chunk count and last indexed time for each
// configured project.
func (a *App) ProjectStats(ctx context.Context) (map[string]stats.Project, error) {
	loaded, err := a.indexingStats.Load()
	if err != nil {
		return nil, fmt.Errorf("load stats: %w", err)
	}

	a.rebuildMu.RLock()
	projects := make([]project.Project, len(a.projects))
	copy(projects, a.projects)
	a.rebuildMu.RUnlock()

	out := make(map[string]stats.Project, len(projects))
	for _, p := range projects {
		count, err := a.store.CountByProject(ctx, p.ID)
		if err != nil {
			return nil, fmt.Errorf("count project %q: %w", p.Name, err)
		}
		stat := stats.Project{Chunks: count}
		if s, ok := loaded[p.Name]; ok {
			stat.LastIndexedAt = s.LastIndexedAt
		}
		out[p.Name] = stat
	}
	return out, nil
}

// FailProgress marks the current rebuild as failed.
func (a *App) FailProgress(err error) {
	if a.progress != nil {
		_ = a.progress.Fail(err)
	}
}
