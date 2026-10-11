package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/system"
)

const (
	intelGpuStatsCmd      string = "intel_gpu_top"
	intelGpuStatsInterval string = "3300" // in milliseconds
)

type intelGpuStats struct {
	PowerGPU float64
	PowerPkg float64
	Engines  map[string]float64
}

// updateIntelFromStats updates aggregated GPU data from a single intelGpuStats sample
func (gm *GPUManager) updateIntelFromStats(sample *intelGpuStats) bool {
	gm.Lock()
	defer gm.Unlock()

	// only one gpu for now - cmd doesn't provide all by default
	id := "i0" // prefix with i to avoid conflicts with nvidia card ids
	gpuData, ok := gm.GpuDataMap[id]
	if !ok {
		gpuData = &system.GPUData{Name: "GPU", Engines: make(map[string]float64)}
		gm.GpuDataMap[id] = gpuData
	}

	gpuData.Power += sample.PowerGPU
	gpuData.PowerPkg += sample.PowerPkg

	if gpuData.Engines == nil {
		gpuData.Engines = make(map[string]float64, len(sample.Engines))
	}
	for name, engine := range sample.Engines {
		gpuData.Engines[name] += engine
	}

	gpuData.Count++
	return true
}

// collectIntelStats executes intel_gpu_top in JSON mode (-J) and parses the output.
func (gm *GPUManager) collectIntelStats() (err error) {
	// Build command arguments, optionally selecting a device via -d
	args := []string{"-s", intelGpuStatsInterval, "-J"}
	if dev, ok := utils.GetEnv("INTEL_GPU_DEVICE"); ok && dev != "" {
		args = append(args, "-d", dev)
	}
	cmd := exec.Command(intelGpuStatsCmd, args...)
	// Avoid blocking if intel_gpu_top writes to stderr
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	// Ensure we always reap the child to avoid zombies on any return path and
	// propagate a non-zero exit code if no other error was set.
	defer func() {
		// Best-effort close of the pipe (unblock the child if it writes)
		_ = stdout.Close()
		if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
			_ = cmd.Process.Kill()
		}
		if waitErr := cmd.Wait(); err == nil && waitErr != nil {
			err = waitErr
		}
	}()

	if err := gm.parseIntelJSONStream(stdout); err != nil {
		return err
	}
	// The closing "]" is printed as the process exits, so read to EOF to let
	// it finish instead of killing it.
	_, _ = io.Copy(io.Discard, stdout)
	return nil
}

// parseIntelJSONStream decodes samples from intel_gpu_top -J output and
// aggregates them. Since v1.28 the samples are wrapped in an array ("[", then
// objects, and "]" only when the process exits). Older versions print objects
// without the opening "[". Some builds (seen on i915 / Arrow Lake) omit the
// commas between objects inside the array; those are tolerated by reading
// successive objects rather than decoding the whole stream as one JSON array.
func (gm *GPUManager) parseIntelJSONStream(r io.Reader) error {
	er := &eofReader{r: r}
	br := bufio.NewReader(er)

	var hadDataRow bool
	// skip first data row because it sometimes has erroneous data
	var skippedFirstDataRow bool
	for {
		obj, err := nextIntelJSONObject(br)
		if err != nil {
			if err == io.EOF {
				break
			}
			// A truncated final sample when the process is killed is normal.
			if er.eof {
				break
			}
			return err
		}
		if obj == nil { // closing "]"
			break
		}
		var sample intelGpuJSONSample
		if err := json.Unmarshal(obj, &sample); err != nil {
			if er.eof {
				break
			}
			return err
		}
		if !skippedFirstDataRow {
			skippedFirstDataRow = true
			continue
		}
		stats := parseIntelJSONSample(sample)
		if !validIntelPower(stats.PowerGPU) || !validIntelPower(stats.PowerPkg) {
			slog.Debug("Skipping intel_gpu_top sample with invalid power", "gpu", stats.PowerGPU, "pkg", stats.PowerPkg)
			continue
		}
		hadDataRow = true
		gm.updateIntelFromStats(&stats)
	}
	if !hadDataRow {
		return errNoValidData
	}
	return nil
}

// nextIntelJSONObject returns the next top-level JSON object from br, skipping
// an optional opening "[", commas, and whitespace. A closing "]" returns
// (nil, nil). EOF with no further object returns (nil, io.EOF).
func nextIntelJSONObject(br *bufio.Reader) ([]byte, error) {
	for {
		b, err := peekNonSpace(br)
		if err != nil {
			return nil, err
		}
		switch b {
		case '[', ',':
			_, _ = br.ReadByte()
			continue
		case ']':
			_, _ = br.ReadByte()
			return nil, nil
		case '{':
			return readJSONObject(br)
		default:
			return nil, fmt.Errorf("invalid character %q looking for beginning of intel_gpu_top sample", b)
		}
	}
}

// readJSONObject reads one JSON object from br, respecting strings and nesting.
func readJSONObject(br *bufio.Reader) ([]byte, error) {
	var buf []byte
	depth := 0
	inString := false
	escape := false
	for {
		c, err := br.ReadByte()
		if err != nil {
			if err == io.EOF {
				return nil, io.ErrUnexpectedEOF
			}
			return nil, err
		}
		buf = append(buf, c)
		if inString {
			if escape {
				escape = false
				continue
			}
			switch c {
			case '\\':
				escape = true
			case '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return buf, nil
			}
		}
	}
}

// eofReader records whether the underlying reader has returned io.EOF. The
// json decoder reports a stream ending mid-value as a syntax error, so this
// is how a truncated final sample is told apart from invalid output.
type eofReader struct {
	r   io.Reader
	eof bool
}

func (e *eofReader) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	if err == io.EOF {
		e.eof = true
	}
	return n, err
}

// peekNonSpace discards leading JSON whitespace and returns the next byte without consuming it.
func peekNonSpace(br *bufio.Reader) (byte, error) {
	for {
		b, err := br.Peek(1)
		if err != nil {
			return 0, err
		}
		switch b[0] {
		case ' ', '\t', '\n', '\r':
			_, _ = br.ReadByte()
		default:
			return b[0], nil
		}
	}
}

// intelGpuJSONSample is a single sample from intel_gpu_top -J output. Only the
// needed fields are mapped.
type intelGpuJSONSample struct {
	Power *struct {
		GPU     float64 `json:"GPU"`
		Package float64 `json:"Package"`
	} `json:"power"`
	Engines map[string]struct {
		Busy float64 `json:"busy"`
	} `json:"engines"`
}

// validIntelPower reports whether a power reading from intel_gpu_top is plausible.
func validIntelPower(watts float64) bool {
	// 5000 is well above any real GPU or package draw. intel_gpu_top
	// computes power from unsigned energy counter deltas, so a counter that reads
	// lower than the previous sample produces an enormous value for that period.
	return watts >= 0 && watts <= 5000
}

// parseIntelJSONSample converts one intel_gpu_top JSON sample into intelGpuStats.
func parseIntelJSONSample(sample intelGpuJSONSample) (stats intelGpuStats) {
	if sample.Power != nil {
		stats.PowerGPU = sample.Power.GPU
		stats.PowerPkg = sample.Power.Package
	}
	if len(sample.Engines) > 0 {
		stats.Engines = make(map[string]float64, len(sample.Engines))
		for key, engine := range sample.Engines {
			stats.Engines[intelEngineClass(key)] += engine.Busy
		}
	}
	return stats
}

// intelEngineClass returns the engine class name for an engine key. Keys are
// class names ("Render/3D", "Video") in class view, which JSON output uses by
// default since v1.28, and instance names ("Render/3D/0", "Video/1") in
// physical view, which older versions use.
func intelEngineClass(key string) string {
	if i := strings.LastIndexByte(key, '/'); i >= 0 {
		if _, err := strconv.ParseUint(key[i+1:], 10, 32); err == nil {
			return key[:i]
		}
	}
	return key
}
