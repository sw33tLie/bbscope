package utils

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

const (
	lockFileSuffix = ".lock"
)

// DBLock manages a file-based lock for the SQLite database.
type DBLock struct {
	lock *flock.Flock
	path string
}

// NewDBLock creates a new lock for the given database path.
func NewDBLock(dbPath string) (*DBLock, error) {
	absPath, err := GetAbsDBPath(dbPath)
	if err != nil {
		return nil, fmt.Errorf("could not get absolute db path: %w", err)
	}
	lockPath := absPath + lockFileSuffix
	return &DBLock{
		lock: flock.New(lockPath),
		path: lockPath,
	}, nil
}

// Lock acquires the database lock, waiting if necessary.
// It will print a message if it has to wait.
func (l *DBLock) Lock() error {
	locked, err := l.lock.TryLock()
	if err != nil {
		return fmt.Errorf("failed to acquire lock on %s: %w", l.path, err)
	}

	if !locked {
		fmt.Fprintf(os.Stderr, "Another bbscope process is writing to the database, waiting for it to finish...\n")
		if err := l.lock.Lock(); err != nil {
			return fmt.Errorf("failed to acquire lock on %s after waiting: %w", l.path, err)
		}
	}
	return nil
}

// Unlock releases the database lock.
func (l *DBLock) Unlock() error {
	if err := l.lock.Unlock(); err != nil {
		// Suppress error if the lock file doesn't exist, as it means we don't hold the lock.
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to release lock on %s: %w", l.path, err)
	}
	// We can optionally remove the lock file on unlock, but it's not strictly
	// necessary as flock handles stale locks.
	// os.Remove(l.path)
	return nil
}

// GetAbsDBPath resolves the database path.
func GetAbsDBPath(dbPath string) (string, error) {
	if dbPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".config", "bbscope", "bbscope.sqlite"), nil
	}
	return filepath.Abs(dbPath)
}
