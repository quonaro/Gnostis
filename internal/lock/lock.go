package lock

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// Lock is an advisory file lock on a directory.
type Lock struct {
	path string
	file *os.File
}

// New creates a lock for the given directory. The lock file is created inside
// the directory; it is not acquired until TryLock or TryRLock is called.
func New(dir string) *Lock {
	return &Lock{path: filepath.Join(dir, ".lock")}
}

// TryLock acquires an exclusive non-blocking lock. It returns an error if the
// lock is already held by another process.
func (l *Lock) TryLock() error {
	if l.file != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return fmt.Errorf("create lock directory: %w", err)
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("open lock file: %w", err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = f.Close()
		if err == unix.EWOULDBLOCK {
			return fmt.Errorf("another gnostis instance is running")
		}
		return fmt.Errorf("acquire lock: %w", err)
	}
	l.file = f
	return nil
}

// TryRLock acquires a shared non-blocking lock. It returns an error if an
// exclusive lock is already held.
func (l *Lock) TryRLock() error {
	if l.file != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return fmt.Errorf("create lock directory: %w", err)
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("open lock file: %w", err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_SH|unix.LOCK_NB); err != nil {
		_ = f.Close()
		if err == unix.EWOULDBLOCK {
			return fmt.Errorf("another gnostis instance is running")
		}
		return fmt.Errorf("acquire shared lock: %w", err)
	}
	l.file = f
	return nil
}

// Unlock releases the lock and closes the underlying file.
func (l *Lock) Unlock() error {
	if l.file == nil {
		return nil
	}
	if err := unix.Flock(int(l.file.Fd()), unix.LOCK_UN); err != nil {
		return fmt.Errorf("release lock: %w", err)
	}
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("close lock file: %w", err)
	}
	l.file = nil
	return nil
}
