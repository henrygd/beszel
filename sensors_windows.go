//go:build windows

package agent

import (
	"bufio"
	"context"
	"fmt"
	"github.com/Microsoft/go-winio"
	"github.com/shirou/gopsutil/v4/sensors"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// Note: This is always called from Agent.gatherStats() which holds Agent.Lock(),
// so no internal concurrency protection is needed.

const lhmPipeName = `\\.\pipe\beszel_lhm`

// lhmClient communicates with the beszel_lhm Windows service via named pipe.
type lhmClient struct {
	scanner *bufio.Scanner
	writer  *bufio.Writer
	conn    interface {
		Close() error
	}
	isRunning bool
}

var (
	beszelLhm *lhmClient
	useLHM    = true
)

// newLhmClient connects to the named pipe exposed by the beszel_lhm service.
func newLhmClient() (*lhmClient, error) {
	c := &lhmClient{}
	if err := c.connect(); err != nil {
		return nil, err
	}
	return c, nil
}

// connect (re)establishes the named pipe connection.
func (c *lhmClient) connect() error {
	timeout := 3 * time.Second
	conn, err := winio.DialPipe(lhmPipeName, &timeout)
	if err != nil {
		return fmt.Errorf("cannot connect to beszel_lhm pipe: %w", err)
	}
	c.conn = conn
	c.scanner = bufio.NewScanner(conn)
	c.writer = bufio.NewWriter(conn)
	c.isRunning = true
	return nil
}

// close tears down the pipe connection.
func (c *lhmClient) close() {
	c.isRunning = false
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.scanner = nil
	c.writer = nil
}

// getTemps sends "getTemps" to the service and parses the response lines.
func (c *lhmClient) getTemps(ctx context.Context) ([]sensors.TemperatureStat, error) {
	// Reconnect if needed
	if !c.isRunning {
		if err := c.connect(); err != nil {
			slog.Debug("LHM pipe reconnect failed", "err", err)
			return sensors.TemperaturesWithContext(ctx)
		}
	}

	// Send command
	if _, err := fmt.Fprintln(c.writer, "getTemps"); err != nil {
		c.close()
		slog.Debug("LHM pipe write failed", "err", err)
		return sensors.TemperaturesWithContext(ctx)
	}
	if err := c.writer.Flush(); err != nil {
		c.close()
		slog.Debug("LHM pipe flush failed", "err", err)
		return sensors.TemperaturesWithContext(ctx)
	}

	// Read response lines until empty line (end-of-data marker)
	var temps []sensors.TemperatureStat
	for c.scanner.Scan() {
		line := strings.TrimSpace(c.scanner.Text())
		if line == "" {
			break
		}

		parts := strings.Split(line, "|")
		if len(parts) != 2 {
			slog.Debug("LHM: invalid sensor format", "line", line)
			continue
		}

		name := strings.TrimSpace(parts[0])
		value, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil {
			slog.Debug("LHM: failed to parse value", "line", line, "err", err)
			continue
		}

		if name == "" || value <= 0 || value > 150 {
			slog.Debug("LHM: invalid sensor", "name", name, "value", value)
			continue
		}

		slog.Info("LHM sensor", "name", name, "value", value)
		temps = append(temps, sensors.TemperatureStat{
			SensorKey:   name,
			Temperature: value,
		})
	}

	if err := c.scanner.Err(); err != nil {
		c.close()
		slog.Debug("LHM pipe read error", "err", err)
		return sensors.TemperaturesWithContext(ctx)
	}

	if len(temps) == 0 {
		slog.Warn("LHM: no sensors received, falling back to gopsutil")
		return sensors.TemperaturesWithContext(ctx)
	}

	return temps, nil
}

// getSensorTemps connects to the beszel_lhm named pipe service and retrieves temperatures.
// Falls back to gopsutil if the service is unavailable.
func getSensorTemps(ctx context.Context) (temps []sensors.TemperatureStat, err error) {
	if !useLHM {
		return sensors.TemperaturesWithContext(ctx)
	}

	if beszelLhm == nil {
		beszelLhm, err = newLhmClient()
		if err != nil {
			slog.Debug("LHM pipe unavailable, falling back to gopsutil", "err", err)
			return sensors.TemperaturesWithContext(ctx)
		}
	}

	return beszelLhm.getTemps(ctx)
}
