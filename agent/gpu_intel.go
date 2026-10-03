package agent

import (
	"bufio"
	"encoding/json"
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
// comma separated objects, and "]" only when the process exits). Older
// versions print the same comma separated objects without the opening "[", so
// it is added here to let both formats decode as an array.
//
// The elements are run through intelCommaInserter first, because some builds
// print the array elements with no separator at all.
func (gm *GPUManager) parseIntelJSONStream(r io.Reader) error {
	er := &eofReader{r: r}
	br := bufio.NewReader(er)
	first, err := peekNonSpace(br)
	if err != nil {
		if err == io.EOF {
			return errNoValidData
		}
		return err
	}
	var src io.Reader = br
	if first != '[' {
		src = io.MultiReader(strings.NewReader("["), br)
	}

	dec := json.NewDecoder(&intelCommaInserter{src: src})
	if _, err := dec.Token(); err != nil { // opening "["
		return err
	}
	var hadDataRow bool
	// skip first data row because it sometimes has erroneous data
	var skippedFirstDataRow bool
	// Decode reads one object and skips the commas between them. The array is
	// usually never closed, so output ending mid-array or mid-sample (the
	// process was killed) is the normal end of the stream rather than an error.
	for dec.More() {
		var sample intelGpuJSONSample
		if err := dec.Decode(&sample); err != nil {
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

// intelCommaInserter passes intel_gpu_top -J output through to the json decoder
// one element at a time, adding the separator between elements that some
// builds omit.
//
// Since v1.28 the tool wraps its samples in a JSON array, but builds older than
// igt 2.4 print the elements back to back with no separator:
//
//	[
//	{ ... "clients": {
//	}
//	}
//	{ ...
//
// encoding/json rejects that, which made every Intel GPU reading fail in agent
// 0.21.0 - the beszel-agent-intel image installs igt-gpu-tools from alpine
// edge/testing, which still packages igt 2.3. Upstream fixed the missing
// separator in
// https://gitlab.freedesktop.org/drm/igt-gpu-tools/-/work_items/199, but the
// parser still has to cope with the builds that are already deployed.
//
// Bytes are forwarded unchanged apart from the separators: the two elements of
// a source pair are joined with exactly one ',' whether the source wrote one or
// not. Braces inside a string are data rather than framing, so the depth
// counter tracks string literals and escapes.
type intelCommaInserter struct {
	src io.Reader
	// out holds the bytes produced for the source byte being read: the byte
	// itself, a separator followed by the byte, or nothing if the byte is
	// dropped.
	out []byte
	// depth is the brace nesting level, 0 between samples.
	depth int
	// samples counts how many top level elements have been started.
	samples  int
	inString bool
	escaped  bool
	// idle counts consecutive (0, nil) reads from src, which a well behaved
	// io.Reader does not return.
	idle int
}

func (c *intelCommaInserter) Read(p []byte) (int, error) {
	for n := 0; ; {
		if len(c.out) == 0 {
			var one [1]byte
			read, err := c.src.Read(one[:])
			if read > 0 {
				c.idle = 0
				c.out = c.translate(one[0])
				continue
			}
			if err == nil {
				if c.idle++; c.idle > maxIdleReads {
					err = io.ErrNoProgress
				} else {
					continue
				}
			}
			if n > 0 {
				return n, nil
			}
			return 0, err
		}
		written := copy(p[n:], c.out)
		n += written
		c.out = c.out[written:]
		if n == len(p) {
			return n, nil
		}
	}
}

// maxIdleReads bounds how many (0, nil) reads are tolerated from src before
// giving up, so a misbehaving reader cannot spin forever.
const maxIdleReads = 100

// translate returns the bytes to emit for one source byte, inserting the
// element separator when the byte opens a new top level element. A nil result
// drops the byte: a ',' the source already supplied, because exactly one is
// written when the next element opens.
func (c *intelCommaInserter) translate(b byte) []byte {
	switch {
	case c.escaped:
		c.escaped = false
	case c.inString && b == '\\':
		c.escaped = true
	case b == '"':
		c.inString = !c.inString
	case c.inString:
		// Braces inside a string are data, not framing.
	case b == '{':
		if c.depth > 0 {
			c.depth++ // nested object inside a sample
			break
		}
		c.depth++
		if c.samples++; c.samples > 1 {
			return []byte{',', b}
		}
		return []byte{b}
	case b == ',' && c.depth == 0:
		return nil
	case b == '}':
		if c.depth > 0 {
			c.depth--
		}
	}
	return []byte{b}
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
