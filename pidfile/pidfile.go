package pidfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/kinkist/x-media-downloader/logger"
)

const pidFileName = "dltw.pid"

// pidFilePath returns the path to the PID file.
// Compiled binary: same directory as the executable.
// go run environment: falls back to the current working directory (CWD).
func pidFilePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to determine executable path: %w", err)
	}

	exeDir := filepath.Dir(exe)
	// go run resolves to a temp dir (/tmp, go-build, etc.) so fall back to CWD
	if strings.Contains(exeDir, "go-build") || strings.Contains(exeDir, os.TempDir()) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to determine current directory: %w", err)
		}
		path := filepath.Join(cwd, pidFileName)
		logger.Debug("pidfile path (go run): %s", path)
		return path, nil
	}

	path := filepath.Join(exeDir, pidFileName)
	logger.Debug("pidfile path: %s", path)
	return path, nil
}

// CheckAndCreatePidFile prevents duplicate execution.
//   - No PID file: creates one with the current PID and proceeds normally.
//   - PID file exists and the process is alive: returns an error.
//   - PID file exists but the process is gone (previous crash): deletes and recreates.
func CheckAndCreatePidFile() error {
	path, err := pidFilePath()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err == nil {
		// PID file exists — check whether the process is still alive
		pidStr := strings.TrimSpace(string(data))
		logger.Debug("existing PID file found (content: %q)", pidStr)

		pid, parseErr := strconv.Atoi(pidStr)
		if parseErr == nil {
			proc, findErr := os.FindProcess(pid)
			if findErr == nil {
				// signal 0: only checks process existence, does not send an actual signal
				sigErr := proc.Signal(syscall.Signal(0))
				logger.Debug("signal(0) → PID %d: err=%v", pid, sigErr)
				if sigErr == nil {
					return fmt.Errorf("already running (PID: %d, PID file: %s)", pid, path)
				}
			} else {
				logger.Debug("FindProcess(%d) failed: %v", pid, findErr)
			}
		} else {
			logger.Debug("failed to parse PID file content: %v", parseErr)
		}
		// process not found — previous crash; remove stale file and recreate
		fmt.Printf("[PID] removing stale PID file (PID %s already exited)\n", pidStr)
		os.Remove(path)
	} else {
		logger.Debug("no PID file found (will create): %v", err)
	}

	// write current PID
	currentPID := os.Getpid()
	if err := os.WriteFile(path, []byte(strconv.Itoa(currentPID)), 0644); err != nil {
		return fmt.Errorf("failed to create PID file (%s): %w", path, err)
	}
	fmt.Printf("[PID] started (PID: %d, file: %s)\n", currentPID, path)
	return nil
}

// RemovePidFile deletes the PID file on program exit.
func RemovePidFile() {
	path, err := pidFilePath()
	if err != nil {
		return
	}
	logger.Debug("removing PID file: %s", path)
	if err := os.Remove(path); err == nil {
		fmt.Printf("[PID] PID file removed (%s)\n", path)
	}
}
