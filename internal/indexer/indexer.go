package indexer

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/quonaro/gnostis/internal/directory"
	"github.com/quonaro/gnostis/internal/project"
)

// FileInfo represents an indexed file with its content and metadata.
type FileInfo struct {
	ProjectID string
	Path      string
	RelPath   string
	Content   string
	ModTime   time.Time
	Hash      string
}

// Indexer walks directories and collects indexable files.
type Indexer struct {
	gitIgnore *gitIgnoreCache
}

// New creates a new Indexer.
func New() *Indexer {
	return &Indexer{
		gitIgnore: newGitIgnoreCache(),
	}
}

// Index walks the directory and returns files matching the indexing rules.
func (idx *Indexer) Index(ctx context.Context, dir directory.Directory, proj project.Project) ([]FileInfo, error) {
	var files []FileInfo

	err := filepath.WalkDir(dir.Path, func(absPath string, d os.DirEntry, err error) error {
		if err != nil {
			slog.Warn("walk directory entry", "path", absPath, "error", err)
			return err
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if d.IsDir() {
			return nil
		}

		if idx.gitIgnore.isIgnored(dir.Path, absPath) {
			return nil
		}

		relPath, err := filepath.Rel(dir.Path, absPath)
		if err != nil {
			return fmt.Errorf("relative path %s: %w", absPath, err)
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", absPath, err)
		}

		if !dir.ShouldIndex(relPath, info.Size()) {
			return nil
		}

		content, err := os.ReadFile(absPath)
		if err != nil {
			slog.Warn("read file", "path", absPath, "error", err)
			return fmt.Errorf("read %s: %w", absPath, err)
		}

		files = append(files, FileInfo{
			ProjectID: proj.ID,
			Path:      absPath,
			RelPath:   relPath,
			Content:   string(content),
			ModTime:   info.ModTime(),
			Hash:      hashContent(content),
		})

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", dir.Path, err)
	}

	slog.InfoContext(ctx, "indexed directory", "path", dir.Path, "files", len(files))
	return files, nil
}

func hashContent(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}
