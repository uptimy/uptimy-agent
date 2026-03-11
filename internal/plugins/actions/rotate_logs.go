package actions

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

// RotateLogsAction rotates log files using logrotate or safe truncation.
type RotateLogsAction struct {
	logger       *zap.SugaredLogger
	allowedPaths []string
}

func NewRotateLogsAction(logger *zap.SugaredLogger) *RotateLogsAction {
	return &RotateLogsAction{
		logger: logger,
		allowedPaths: []string{
			"/var/log", "/opt/app/logs", "/var/cache",
		},
	}
}

func (a *RotateLogsAction) Name() string { return "rotate_logs" }

func (a *RotateLogsAction) Execute(ctx context.Context, params map[string]string) error {
	path := params["path"]
	if path == "" {
		return fmt.Errorf("rotate_logs: 'path' parameter is required")
	}

	// Validate the path is in allowed list
	if err := a.validatePath(path); err != nil {
		return fmt.Errorf("rotate_logs: %w", err)
	}

	// Check if logrotate is available
	if _, err := exec.LookPath("logrotate"); err == nil {
		return a.rotateWithLogrotate(ctx, path)
	}

	// Fallback to safe truncation
	return a.rotateBySafeTruncation(ctx, path, params)
}

// rotateWithLogrotate uses the system logrotate command.
func (a *RotateLogsAction) rotateWithLogrotate(ctx context.Context, path string) error {
	a.logger.Infow("rotating logs with logrotate", "path", path)

	// Create temporary logrotate config
	config := fmt.Sprintf(`%s {
    rotate 5
    compress
    missingok
    notifempty
    copytruncate
}`, path)

	tmpFile, err := os.CreateTemp("", "logrotate-*.conf")
	if err != nil {
		return fmt.Errorf("failed to create temp config: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	tmpFile.Close()

	cmd := exec.CommandContext(ctx, "logrotate", "-f", tmpFile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("logrotate failed: %w (output: %s)", err, strings.TrimSpace(string(output)))
	}

	a.logger.Infow("logs rotated successfully", "path", path)
	return nil
}

// rotateBySafeTruncation rotates logs by truncation (keeping recent content).
func (a *RotateLogsAction) rotateBySafeTruncation(ctx context.Context, path string, params map[string]string) error {
	a.logger.Infow("rotating logs by truncation", "path", path)

	// Get max size to keep (default: 10MB)
	maxSize := int64(10 * 1024 * 1024)
	if v, ok := params["max_size"]; ok {
		if parsed, err := parseSize(v); err == nil {
			maxSize = parsed
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	if info.Size() <= maxSize {
		a.logger.Infow("log file within size limit, skipping rotation", "path", path, "size", info.Size())
		return nil
	}

	// Create backup
	backupPath := path + "." + time.Now().Format("20060102-150405")
	if err := os.Rename(path, backupPath); err != nil {
		// If rename fails, try truncation
		return a.truncateFile(path, maxSize)
	}

	// Create new empty file
	f, err := os.Create(path)
	if err != nil {
		// Restore backup if we can't create new file
		os.Rename(backupPath, path)
		return fmt.Errorf("failed to create new log file: %w", err)
	}
	f.Close()

	a.logger.Infow("logs rotated by backup", "path", path, "backup", backupPath)
	return nil
}

// truncateFile truncates a file keeping the last maxSize bytes.
func (a *RotateLogsAction) truncateFile(path string, maxSize int64) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	if err := f.Truncate(maxSize); err != nil {
		return fmt.Errorf("failed to truncate file: %w", err)
	}

	a.logger.Infow("log file truncated", "path", path, "max_size", maxSize)
	return nil
}

// validatePath ensures the path is within allowed directories.
func (a *RotateLogsAction) validatePath(path string) error {
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

// parseSize parses size strings like "10MB", "1GB".
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	s = strings.ToUpper(s)

	multiplier := int64(1)
	if strings.HasSuffix(s, "KB") {
		multiplier = 1024
		s = strings.TrimSuffix(s, "KB")
	} else if strings.HasSuffix(s, "MB") {
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "MB")
	} else if strings.HasSuffix(s, "GB") {
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "GB")
	}

	var size int64
	if _, err := fmt.Sscanf(s, "%d", &size); err != nil {
		return 0, fmt.Errorf("invalid size: %s", s)
	}

	return size * multiplier, nil
}
