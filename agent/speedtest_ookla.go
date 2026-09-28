package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/henrygd/beszel/internal/entities/speedtest"
)

const (
	speedtestCmd     = "speedtest"
	speedtestTimeout = 2 * time.Minute
)

// speedtestRunner performs one speedtest. Implementations must honor cancellation.
type speedtestRunner func(ctx context.Context, serverID uint32) (speedtest.Result, error)

// ooklaResult is the subset of the Ookla CLI `--format=json` result we use.
type ooklaResult struct {
	Type string `json:"type"`
	Ping struct {
		Jitter  float64 `json:"jitter"`
		Latency float64 `json:"latency"`
	} `json:"ping"`
	Download   ooklaTransfer `json:"download"`
	Upload     ooklaTransfer `json:"upload"`
	PacketLoss *float64      `json:"packetLoss"`
	ISP        string        `json:"isp"`
	Server     struct {
		ID       uint32 `json:"id"`
		Name     string `json:"name"`
		Location string `json:"location"`
		Country  string `json:"country"`
	} `json:"server"`
	Result struct {
		URL string `json:"url"`
	} `json:"result"`
}

// ooklaTransfer is a download or upload phase of an Ookla CLI result.
type ooklaTransfer struct {
	Bandwidth uint64 `json:"bandwidth"`
	Latency   struct {
		IQM    float64 `json:"iqm"`
		Jitter float64 `json:"jitter"`
	} `json:"latency"`
}

func (t ooklaTransfer) latency() speedtest.Latency {
	return speedtest.Latency{IQM: t.Latency.IQM, Jitter: t.Latency.Jitter}
}

// ooklaLog is an Ookla CLI log line, written to stderr in JSON format.
type ooklaLog struct {
	Type    string `json:"type"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

// runOoklaSpeedtest runs the Ookla speedtest CLI and parses its JSON result.
func runOoklaSpeedtest(ctx context.Context, serverID uint32) (speedtest.Result, error) {
	path, err := exec.LookPath(speedtestCmd)
	if err != nil {
		return speedtest.Result{}, errors.New("Ookla speedtest CLI not found in PATH")
	}
	ctx, cancel := context.WithTimeout(ctx, speedtestTimeout)
	defer cancel()

	args := []string{"--format=json", "--accept-license", "--accept-gdpr"}
	if serverID > 0 {
		args = append(args, "--server-id="+strconv.FormatUint(uint64(serverID), 10))
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return speedtest.Result{}, fmt.Errorf("speedtest canceled: %w", ctx.Err())
	}
	result, parseErr := parseOoklaOutput(stdout.Bytes())
	if parseErr == nil {
		return result, nil
	}
	// Ookla writes error log lines to stderr, but check stdout as well to be safe.
	for _, output := range [][]byte{stderr.Bytes(), stdout.Bytes()} {
		if msg := ooklaErrorMessage(output); msg != "" {
			return speedtest.Result{}, errors.New(msg)
		}
	}
	if runErr != nil {
		return speedtest.Result{}, fmt.Errorf("speedtest failed: %w", runErr)
	}
	return speedtest.Result{}, parseErr
}

// parseOoklaOutput extracts the result line from Ookla CLI JSON output.
func parseOoklaOutput(output []byte) (speedtest.Result, error) {
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var res ooklaResult
		if err := json.Unmarshal(line, &res); err != nil || res.Type != "result" {
			continue
		}
		location := res.Server.Location
		if res.Server.Country != "" {
			location = strings.TrimPrefix(location+", "+res.Server.Country, ", ")
		}
		loss := -1.0
		if res.PacketLoss != nil {
			loss = *res.PacketLoss
		}
		return speedtest.Result{
			Download:        res.Download.Bandwidth,
			Upload:          res.Upload.Bandwidth,
			Ping:            res.Ping.Latency,
			Jitter:          res.Ping.Jitter,
			DownloadLatency: res.Download.latency(),
			UploadLatency:   res.Upload.latency(),
			Loss:            loss,
			ServerID:        res.Server.ID,
			ServerName:      res.Server.Name,
			ServerLocation:  location,
			ISP:             res.ISP,
			URL:             res.Result.URL,
		}, nil
	}
	return speedtest.Result{}, errors.New("no result in speedtest output (is the Ookla speedtest CLI installed?)")
}

// ooklaErrorMessage returns the last error message from Ookla CLI output, if any.
func ooklaErrorMessage(stderr []byte) string {
	var msg string
	for line := range strings.SplitSeq(string(stderr), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var log ooklaLog
		if json.Unmarshal([]byte(line), &log) == nil {
			if log.Level == "error" && log.Message != "" {
				msg = log.Message
			}
			continue
		}
		// Plain text format, e.g. "[error] Configuration - Could not retrieve or read configuration"
		if after, ok := strings.CutPrefix(line, "[error]"); ok {
			msg = strings.TrimSpace(after)
		}
	}
	return msg
}
