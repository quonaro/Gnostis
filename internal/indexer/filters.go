package indexer

import (
	"path/filepath"
	"sync"

	gitignore "github.com/sabhiram/go-gitignore"
)

// gitIgnoreCache loads and caches .gitignore rules for a directory tree.
type gitIgnoreCache struct {
	mu      sync.RWMutex
	ignores map[string]*gitignore.GitIgnore
}

func newGitIgnoreCache() *gitIgnoreCache {
	return &gitIgnoreCache{
		ignores: make(map[string]*gitignore.GitIgnore),
	}
}

// isIgnored reports whether absPath is ignored by any .gitignore file
// between root and the file's parent directory.
func (c *gitIgnoreCache) isIgnored(root, absPath string) bool {
	relRoot, err := filepath.Rel(root, absPath)
	if err != nil {
		return false
	}

	if ig := c.ignoreForDir(root); ig != nil && ig.MatchesPath(relRoot) {
		return true
	}

	dir := filepath.Dir(absPath)
	for dir != root && len(dir) > len(root) {
		rel, err := filepath.Rel(dir, absPath)
		if err != nil {
			return false
		}

		if ig := c.ignoreForDir(dir); ig != nil && ig.MatchesPath(rel) {
			return true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return false
}

func (c *gitIgnoreCache) ignoreForDir(dir string) *gitignore.GitIgnore {
	c.mu.RLock()
	ig, ok := c.ignores[dir]
	c.mu.RUnlock()
	if ok {
		return ig
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if ig, ok := c.ignores[dir]; ok {
		return ig
	}

	path := filepath.Join(dir, ".gitignore")
	ig, err := gitignore.CompileIgnoreFile(path)
	if err != nil {
		c.ignores[dir] = nil
		return nil
	}

	c.ignores[dir] = ig
	return ig
}
