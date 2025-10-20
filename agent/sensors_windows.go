//go:build windows

//go:generate dotnet build -c Release lhm/beszel_lhm.csproj

package agent

import (
	"bufio"
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/sensors"
)

// Note: This is always called from Agent.gatherStats() which holds Agent.Lock(),
// so no internal concurrency protection is needed.

// lhmProcess is a wrapper around the LHM .NET process.
type lhmProcess struct {
	cmd                  *exec.Cmd
	stdin                io.WriteCloser
	stdout               io.ReadCloser
	scanner              *bufio.Scanner
	isRunning            bool
	stoppedNoSensors     bool
	consecutiveNoSensors uint8
	execPath             string
	tempDir              string
}

//go:embed all:lhm/bin/Release/net48
var lhmFs embed.FS

var (
	beszelLhm     *lhmProcess
	beszelLhmOnce sync.Once
	useLHM        = os.Getenv("LHM") == "true"
)

var errNoSensors = errors.New("no sensors found (try running as admin with LHM=true)")

// newlhmProcess copies the embedded LHM executable to a temporary directory and starts it.
func newlhmProcess() (*lhmProcess, error) {
	destDir := filepath.Join(os.TempDir(), "beszel")
	execPath := filepath.Join(destDir, "beszel_lhm.exe")

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Only copy if executable doesn't exist
	if _, err := os.Stat(execPath); os.IsNotExist(err) {
		if err := copyEmbeddedDir(lhmFs, "lhm/bin/Release/net48", destDir); err != nil {
			return nil, fmt.Errorf("failed to copy embedded directory: %w", err)
		}
	}

	lhm := &lhmProcess{
		execPath: execPath,
		tempDir:  destDir,
	}

	if err := lhm.startProcess(); err != nil {
		return nil, fmt.Errorf("failed to start process: %w", err)
	}

	return lhm, nil
}

// startProcess starts the external LHM process
func (lhm *lhmProcess) startProcess() error {
	// Clean up any existing process
	lhm.cleanupProcess()

	cmd := exec.Command(lhm.execPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return err
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return err
	}

	// Update process state
	lhm.cmd = cmd
	lhm.stdin = stdin
	lhm.stdout = stdout
	lhm.scanner = bufio.NewScanner(stdout)
	lhm.isRunning = true

	// Give process a moment to initialize
	time.Sleep(100 * time.Millisecond)

	return nil
}

// cleanupProcess terminates the process and closes resources but preserves files
func (lhm *lhmProcess) cleanupProcess() {
	lhm.isRunning = false

	if lhm.cmd != nil && lhm.cmd.Process != nil {
		lhm.cmd.Process.Kill()
		lhm.cmd.Wait()
	}

	if lhm.stdin != nil {
		lhm.stdin.Close()
		lhm.stdin = nil
	}
	if lhm.stdout != nil {
		lhm.stdout.Close()
		lhm.stdout = nil
	}

	lhm.cmd = nil
	lhm.scanner = nil
	lhm.stoppedNoSensors = false
	lhm.consecutiveNoSensors = 0
}

func (lhm *lhmProcess) getTemps(ctx context.Context) (temps []sensors.TemperatureStat, err error) {
	if !useLHM || lhm.stoppedNoSensors {
		// Fall back to gopsutil if we can't get sensors from LHM
		return sensors.TemperaturesWithContext(ctx)
	}

	// Start process if it's not running
	if !lhm.isRunning || lhm.stdin == nil || lhm.scanner == nil {
		err := lhm.startProcess()
		if err != nil {
			return temps, err
		}
	}

	// Send command to process
	_, err = fmt.Fprintln(lhm.stdin, "getTemps")
	if err != nil {
		lhm.isRunning = false
		return temps, fmt.Errorf("failed to send command: %w", err)
	}

	// Read all sensor lines until we hit an empty line or EOF
	for lhm.scanner.Scan() {
		line := strings.TrimSpace(lhm.scanner.Text())
		if line == "" {
			break
		}

		parts := strings.Split(line, "|")
		if len(parts) != 2 {
			slog.Debug("Invalid sensor format", "line", line)
			continue
		}

		name := strings.TrimSpace(parts[0])
		valueStr := strings.TrimSpace(parts[1])

		value, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			slog.Debug("Failed to parse sensor", "err", err, "line", line)
			continue
		}

		if name == "" || value <= 0 || value > 150 {
			slog.Debug("Invalid sensor", "name", name, "val", value, "line", line)
			continue
		}

		temps = append(temps, sensors.TemperatureStat{
			SensorKey:   name,
			Temperature: value,
		})
	}

	if err := lhm.scanner.Err(); err != nil {
		lhm.isRunning = false
		return temps, err
	}

	// Handle no sensors case
	if len(temps) == 0 {
		lhm.consecutiveNoSensors++
		if lhm.consecutiveNoSensors >= 3 {
			lhm.stoppedNoSensors = true
			slog.Warn(errNoSensors.Error())
			lhm.cleanup()
		}
		return sensors.TemperaturesWithContext(ctx)
	}

	lhm.consecutiveNoSensors = 0

	return temps, nil
}

// getSensorTemps attempts to pull sensor temperatures from the embedded LHM process.
// NB: LibreHardwareMonitorLib requires admin privileges to access all available sensors.
func getSensorTemps(ctx context.Context) (temps []sensors.TemperatureStat, err error) {
	defer func() {
		if err != nil {
			slog.Debug("Error reading sensors", "err", err)
		}
	}()

	if !useLHM {
		return sensors.TemperaturesWithContext(ctx)
	}

	// Initialize process once
	beszelLhmOnce.Do(func() {
		beszelLhm, err = newlhmProcess()
	})

	if err != nil {
		return temps, fmt.Errorf("failed to initialize lhm: %w", err)
	}

	if beszelLhm == nil {
		return temps, fmt.Errorf("lhm not available")
	}

	return beszelLhm.getTemps(ctx)
}

// cleanup terminates the process and closes resources
func (lhm *lhmProcess) cleanup() {
	lhm.cleanupProcess()
	if lhm.tempDir != "" {
		os.RemoveAll(lhm.tempDir)
	}
}

// copyEmbeddedDir copies the embedded directory to the destination path
func copyEmbeddedDir(fs embed.FS, srcPath, destPath string) error {
	entries, err := fs.ReadDir(srcPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destPath, 0755); err != nil {
		return err
	}

	for _, entry := range entries {
		srcEntryPath := path.Join(srcPath, entry.Name())
		destEntryPath := filepath.Join(destPath, entry.Name())

		if entry.IsDir() {
			if err := copyEmbeddedDir(fs, srcEntryPath, destEntryPath); err != nil {
				return err
			}
			continue
		}

		data, err := fs.ReadFile(srcEntryPath)
		if err != nil {
			return err
		}

		if err := os.WriteFile(destEntryPath, data, 0755); err != nil {
			return err
		}
	}

	return nil
}
