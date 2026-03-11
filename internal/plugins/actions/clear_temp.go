package actions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

// ClearTempAction clears temporary files from allowed directories.
type ClearTempAction struct {
	logger       *zap.SugaredLogger
	allowedPaths []string
}

func NewClearTempAction(logger *zap.SugaredLogger) *ClearTempAction {
	return &ClearTempAction{
		logger: logger,
		allowedPaths: []string{
			"/tmp", "/var/tmp", "/var/log", "/var/cache",
		},
	}
}

func (a *ClearTempAction) Name() string { return "clear_temp" }

func (a *ClearTempAction) Execute(ctx context.Context, params map[string]string) error {
	path := params["path"]
	if path == "" {
		return fmt.Errorf("clear_temp: 'path' parameter is required")
	}

	// Validate the path is in allowed list
	if err := a.validatePath(path); err != nil {
		return fmt.Errorf("clear_temp: %w", err)
	}

	// Parse age threshold (default: 24 hours)
	age := 24 * time.Hour
	if v, ok := params["age"]; ok {
		if parsed, err := time.ParseDuration(v); err == nil {
			age = parsed
		}
	}

	// Parse pattern (default: all files)
	pattern := params["pattern"]
	if pattern == "" {
		pattern = "*"
	}

	a.logger.Infow("clearing temp files", "path", path, "age", age, "pattern", pattern)

	cutoff := time.Now().Add(-age)
	var cleared int
	var totalBytes int64

	err := filepath.WalkDir(path, func(fpath string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip inaccessible files
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Match pattern
		if pattern != "*" {
			matched, _ := filepath.Match(pattern, d.Name())
			if !matched {
				return nil
			}
		}

		// Check file age
		info, err := d.Info()
		if err != nil {
			return nil
		}

		if info.ModTime().After(cutoff) {
			return nil
		}

		// Remove the file
		size := info.Size()
		if err := os.Remove(fpath); err != nil {
			a.logger.Warnw("failed to remove file", "path", fpath, "error", err)
			return nil
		}

		cleared++
		totalBytes += size
		return nil
	})

	if err != nil {
		return fmt.Errorf("clear_temp: error walking directory: %w", err)
	}

	a.logger.Infow("temp files cleared",
		"path", path,
		"files_removed", cleared,
		"bytes_freed", totalBytes,
	)

	return nil
}

// validatePath ensures the path is within allowed directories.
func (a *ClearTempAction) validatePath(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	for _, allowed := range a.allowedPaths {
		if strings.HasPrefix(absPath, allowed) {
			return nil
		}
	}

	return fmt.Errorf("path %q is not in allowed paths: %v", path, a.allowedPaths)
}
