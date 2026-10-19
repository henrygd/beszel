//go:generate -command fetchsmartctl go run ./tools/fetchsmartctl
//go:generate fetchsmartctl -out ./smartmontools/smartctl.exe -url https://static.beszel.dev/bin/smartctl/smartctl-nc.exe -sha 3912249c3b329249aa512ce796fd1b64d7cbd8378b68ad2756b39163d9c30b47

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/smart"
)

// SmartManager collects SMART data. Every refresh builds the list of targets
// (smartctl scan, SMART_DEVICES, eMMC and mdraid), queries each one and
// replaces the collected data, so no per-device state has to be kept in sync.
type SmartManager struct {
	sync.Mutex                                  // guards data
	data            map[string]*smart.SmartData // collected data keyed by serial (or sysfs key)
	refreshMutex    sync.Mutex                  // serializes refreshes; guards the fields below
	last            map[string]smartResult      // last success per target key
	woken           map[string]struct{}         // target keys already queried once without -n standby; never cleared
	scanned         []smartDevice               // cached smartctl --scan result
	scanErr         error                       // error of the cached scan
	lastScan        time.Time
	smartctlPath    string
	excludedDevices map[string]struct{}
	// run executes smartctl; nil means exec smartctlPath. Overridable for testing.
	run                smartctlRunner
	darwinNvmeOnce     sync.Once
	darwinNvmeCapacity map[string]uint64      // serial → bytes cache, written once via darwinNvmeOnce
	darwinNvmeProvider func() ([]byte, error) // overridable for testing
}

// smartctlRunner runs smartctl and returns its stdout and exit code. err is set
// only when smartctl could not be run or timed out.
type smartctlRunner func(timeout time.Duration, args ...string) (output []byte, exitCode int, err error)

// smartDevice is a device from the smartctl scan, SMART_DEVICES or sysfs.
type smartDevice struct {
	name  string
	dtype string
	// explicit marks a type set in SMART_DEVICES. It is a deliberate override,
	// so it is always passed to smartctl and never second-guessed (#2102).
	explicit bool
}

// smartAttempt is one way to query a device: a path and a -d type. An empty
// type lets smartctl auto-detect.
type smartAttempt struct {
	path  string
	dtype string
}

type smartTarget struct {
	key      string // name and type; stable across refreshes
	dtype    string // emmc and mdraid are read from sysfs instead of smartctl
	name     string
	attempts []smartAttempt
}

type smartResult struct {
	key     string // data map key
	data    *smart.SmartData
	attempt int  // index of the attempt that succeeded, tried first next time
	ata     bool // device speaks ATA, so -l devstat is requested
}

var errNoValidSmartData = fmt.Errorf("no valid SMART data found") // Error for missing data

const smartScanInterval = 30 * time.Minute

// Refresh updates SMART data for all devices and reports whether every device
// was collected, which lets the hub prune records of removed devices.
func (sm *SmartManager) Refresh(forceScan bool) (bool, error) {
	sm.refreshMutex.Lock()
	defer sm.refreshMutex.Unlock()

	targets, scanErr := sm.buildTargets(forceScan)
	if scanErr != nil {
		slog.Debug("smartctl scan failed", "err", scanErr)
	}

	data := make(map[string]*smart.SmartData, len(targets))
	last := make(map[string]smartResult, len(targets))
	complete := scanErr == nil
	var collectErr error
	for _, target := range targets {
		result, err := sm.collectTarget(target)
		if err != nil {
			complete = false
			if !errors.Is(err, errStandby) {
				slog.Info("smartctl failed", "device", target.name, "err", err)
				collectErr = err
			}
			continue
		}
		if existing, ok := data[result.key]; ok {
			slog.Debug("skipping duplicate SMART device", "device", target.name, "serial", result.key, "first", existing.DiskName)
			continue
		}
		data[result.key] = result.data
		last[target.key] = result
	}
	sm.last = last

	sm.Lock()
	sm.data = data
	sm.Unlock()

	if len(data) > 0 {
		return complete, nil
	}
	if len(targets) == 0 && scanErr != nil {
		return false, scanErr
	}
	switch {
	case collectErr != nil:
		return false, collectErr
	case scanErr != nil:
		return false, scanErr
	}
	return false, errNoValidSmartData
}

// GetCurrentData returns the current SMART data
func (sm *SmartManager) GetCurrentData() map[string]smart.SmartData {
	sm.Lock()
	defer sm.Unlock()
	result := make(map[string]smart.SmartData, len(sm.data))
	for key, value := range sm.data {
		if value != nil {
			result[key] = *value
		}
	}
	return result
}

// buildTargets merges the smartctl scan, SMART_DEVICES and sysfs devices into
// the list of targets to query.
func (sm *SmartManager) buildTargets(forceScan bool) ([]smartTarget, error) {
	var configured []smartDevice
	if raw, ok := utils.GetEnv("SMART_DEVICES"); ok {
		slog.Debug("SMART_DEVICES", "value", raw)
		// An empty SMART_DEVICES disables SMART entirely.
		if strings.TrimSpace(raw) == "" {
			return nil, errNoValidSmartData
		}
		var err error
		if configured, err = parseConfiguredDevices(raw); err != nil {
			return nil, err
		}
	}

	scanned, scanErr := sm.scanSmartctl(forceScan)
	scanned = append(slices.Clone(scanned), scanSysfsDevices()...)

	devices := mergeDevices(scanned, configured)
	targets := make([]smartTarget, 0, len(devices))
	for _, device := range devices {
		if sm.isExcludedDevice(device.name) {
			continue
		}
		targets = append(targets, smartTarget{
			key:      device.name + "|" + device.dtype,
			dtype:    device.dtype,
			name:     device.name,
			attempts: sm.attemptsFor(device),
		})
	}
	return targets, scanErr
}

// scanSysfsDevices returns the eMMC and mdraid devices read from sysfs instead
// of smartctl. It is a variable so tests can isolate themselves from the host.
var scanSysfsDevices = func() []smartDevice {
	var devices []smartDevice
	for _, path := range scanEmmcDevices() {
		devices = append(devices, smartDevice{name: path, dtype: "emmc"})
	}
	for _, path := range scanMdraidDevices() {
		devices = append(devices, smartDevice{name: path, dtype: "mdraid"})
	}
	return devices
}

// mergeDevices adds SMART_DEVICES entries to the scanned devices. A configured
// entry with a type replaces the scanned entry for the same path when that path
// was scanned exactly once; otherwise it is added (e.g. extra RAID members that
// share a controller path). Duplicates are dropped.
func mergeDevices(scanned, configured []smartDevice) []smartDevice {
	devices := slices.Clone(scanned)
	nameCounts := make(map[string]int, len(scanned))
	for _, device := range scanned {
		nameCounts[device.name]++
	}
	for _, device := range configured {
		if i := slices.IndexFunc(devices, func(d smartDevice) bool { return d.name == device.name && d.dtype == device.dtype }); i >= 0 {
			devices[i].explicit = devices[i].explicit || device.explicit
			continue
		}
		if nameCounts[device.name] == 1 {
			if device.dtype != "" {
				i := slices.IndexFunc(devices, func(d smartDevice) bool { return d.name == device.name })
				devices[i] = device
				nameCounts[device.name] = 0 // further types for this path are added
			}
			continue
		}
		devices = append(devices, device)
	}

	seen := make(map[smartDevice]struct{}, len(devices))
	return slices.DeleteFunc(devices, func(d smartDevice) bool {
		d.explicit = false
		if _, ok := seen[d]; ok || d.name == "" {
			return true
		}
		seen[d] = struct{}{}
		return false
	})
}

// attemptsFor returns the smartctl queries to try for a device, in order. All
// fallback behavior lives here:
//   - an explicit SMART_DEVICES type is used as-is (#2102)
//   - scan types ata and scsi are often wrong (e.g. SATA disks behind a SAS
//     HBA or in Docker), so smartctl auto-detects first (#1345); scsi then
//     falls back to sat for SATA disks that auto-detection misses (#2295)
//   - NVMe controller paths fall back to their first namespace (#1504)
func (sm *SmartManager) attemptsFor(device smartDevice) []smartAttempt {
	dtype := strings.ToLower(device.dtype)
	var attempts []smartAttempt
	switch {
	case device.explicit:
		attempts = []smartAttempt{{device.name, device.dtype}}
	case dtype == "scsi":
		attempts = []smartAttempt{{device.name, ""}, {device.name, "sat"}}
	case dtype == "ata":
		attempts = []smartAttempt{{device.name, ""}}
	default:
		attempts = []smartAttempt{{device.name, device.dtype}}
	}
	if isNvmeControllerPath(device.name) && (dtype == "" || dtype == "nvme") {
		if namespace := device.name + "n1"; !sm.isExcludedDevice(namespace) {
			attempts = append(attempts, smartAttempt{namespace, device.dtype})
		}
	}
	return attempts
}

// scanSmartctl returns the devices found by `smartctl --scan -j`. The result,
// including a failure, is cached for smartScanInterval.
func (sm *SmartManager) scanSmartctl(force bool) ([]smartDevice, error) {
	if sm.smartctlPath == "" && sm.run == nil {
		return nil, nil
	}
	if !force && !sm.lastScan.IsZero() && time.Since(sm.lastScan) < smartScanInterval {
		return sm.scanned, sm.scanErr
	}
	sm.lastScan = time.Now()

	output, _, err := sm.runSmartctl(10*time.Second, "--scan", "-j")
	if err == nil {
		var devices []smartDevice
		if devices, err = parseScan(output); err == nil {
			sm.scanned = devices
		}
	}
	// Keep the previous scan on failure so devices don't disappear.
	sm.scanErr = err
	return sm.scanned, err
}

// parseScan parses the output of smartctl --scan -j.
func parseScan(output []byte) ([]smartDevice, error) {
	var scan struct {
		Devices []struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			Protocol string `json:"protocol"`
		} `json:"devices"`
	}
	if err := json.Unmarshal(output, &scan); err != nil {
		return nil, fmt.Errorf("invalid smartctl scan output: %w", err)
	}
	devices := make([]smartDevice, 0, len(scan.Devices))
	for _, device := range scan.Devices {
		slog.Debug("smartctl scan", "name", device.Name, "type", device.Type, "protocol", device.Protocol)
		devices = append(devices, smartDevice{name: device.Name, dtype: device.Type})
	}
	return devices, nil
}

// parseConfiguredDevices parses SMART_DEVICES entries of the form path[:type].
func parseConfiguredDevices(config string) ([]smartDevice, error) {
	splitChar, _ := utils.GetEnv("SMART_DEVICES_SEPARATOR")
	if splitChar == "" {
		splitChar = ","
	}
	var devices []smartDevice
	for entry := range strings.SplitSeq(config, splitChar) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		name, dtype, _ := strings.Cut(entry, ":")
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, fmt.Errorf("invalid SMART_DEVICES entry %q", entry)
		}
		dtype = strings.ToLower(strings.TrimSpace(dtype))
		devices = append(devices, smartDevice{name: name, dtype: dtype, explicit: dtype != ""})
	}
	if len(devices) == 0 {
		return nil, errNoValidSmartData
	}
	return devices, nil
}

var errStandby = errors.New("device is in standby")

// collectTarget reads one target. smartctl targets try each attempt in turn,
// starting with the one that worked last time.
func (sm *SmartManager) collectTarget(target smartTarget) (smartResult, error) {
	switch target.dtype {
	case "emmc", "mmc":
		if key, data, ok := readEmmcSmartData(target.name); ok {
			return smartResult{key: key, data: data}, nil
		}
		return smartResult{}, errNoValidSmartData
	case "mdraid":
		if key, data, ok := readMdraidSmartData(target.name); ok {
			return smartResult{key: key, data: data}, nil
		}
		return smartResult{}, errNoValidSmartData
	}
	if sm.smartctlPath == "" && sm.run == nil {
		return smartResult{}, errNoValidSmartData
	}

	prev, hasPrev := sm.last[target.key]
	order := make([]int, 0, len(target.attempts))
	if hasPrev && prev.attempt < len(target.attempts) {
		order = append(order, prev.attempt)
	}
	for i := range target.attempts {
		if !hasPrev || i != prev.attempt {
			order = append(order, i)
		}
	}

	var err error
	for _, i := range order {
		attempt := target.attempts[i]
		args := smartctlArgs(attempt, hasPrev && prev.ata, true)
		output, exitCode, runErr := sm.runSmartctl(15*time.Second, args...)
		if runErr != nil {
			err = runErr
			slog.Debug("smartctl attempt failed", "device", attempt.path, "type", attempt.dtype, "err", runErr)
			continue
		}
		data, protocol, parseErr := sm.parseSmartctl(output, attempt.dtype)
		if parseErr == nil {
			return smartResult{key: data.SerialNumber, data: data, attempt: i, ata: protocol == "ata"}, nil
		}
		// Don't wake a sleeping disk that was collected before: reuse its last data.
		if isStandby(output, exitCode) {
			if hasPrev {
				slog.Debug("smartctl device in standby", "device", attempt.path)
				return prev, nil
			}
			// Wake a disk once per agent run so it is collected at least once. This
			// is tracked apart from last, so a disk that fails after waking is not
			// woken again on every refresh.
			if _, ok := sm.woken[target.key]; !ok {
				if sm.woken == nil {
					sm.woken = make(map[string]struct{})
				}
				sm.woken[target.key] = struct{}{}
				slog.Info("smartctl waking device in standby for first collection", "device", attempt.path)
				result, wakeErr := sm.wakeTarget(attempt, i)
				if wakeErr == nil {
					return result, nil
				}
				// The disk is awake now, so the remaining attempts can run with
				// -n standby (e.g. sat after scsi auto-detection, #2295).
				err = wakeErr
				continue
			}
			// Already woken once: skip it until it wakes, so the refresh is
			// incomplete and the hub keeps its record. smartctl also reports SLEEP
			// when it lacks permission to check the power mode, so say so.
			if bytes.Contains(bytes.ToUpper(output), []byte("SLEEP MODE")) {
				slog.Info("smartctl reports device asleep, skipping until it wakes; this also happens when smartctl lacks permission", "device", attempt.path)
			} else {
				slog.Info("smartctl reports device in standby, skipping until it wakes", "device", attempt.path)
			}
			return smartResult{}, errStandby
		}
		err = fmt.Errorf("%w (exit status %d)", parseErr, exitCode)
		slog.Debug("smartctl attempt failed", "device", attempt.path, "type", attempt.dtype, "args", args, "exit", exitCode, "err", parseErr)
	}
	return smartResult{}, err
}

// wakeTarget queries an attempt without -n standby, spinning up the disk.
// A failure is returned as a real error rather than errStandby, which also
// surfaces the permission problem that smartctl otherwise reports as SLEEP.
func (sm *SmartManager) wakeTarget(attempt smartAttempt, i int) (smartResult, error) {
	args := smartctlArgs(attempt, false, false)
	output, exitCode, err := sm.runSmartctl(15*time.Second, args...)
	if err != nil {
		return smartResult{}, err
	}
	data, protocol, err := sm.parseSmartctl(output, attempt.dtype)
	if err != nil {
		slog.Debug("smartctl wake attempt failed", "device", attempt.path, "type", attempt.dtype, "args", args, "exit", exitCode, "err", err)
		return smartResult{}, fmt.Errorf("%w (exit status %d)", err, exitCode)
	}
	return smartResult{key: data.SerialNumber, data: data, attempt: i, ata: protocol == "ata"}, nil
}

// smartctlArgs returns the smartctl arguments for one attempt. standby adds
// -n standby so smartctl skips a sleeping disk instead of waking it.
func smartctlArgs(attempt smartAttempt, ata, standby bool) []string {
	args := make([]string, 0, 9)
	if attempt.dtype != "" {
		args = append(args, "-d", attempt.dtype)
	}
	args = append(args, "-a", "--json=c")
	if dtype := strings.ToLower(attempt.dtype); ata || dtype == "sat" || dtype == "ata" {
		args = append(args, "-l", "devstat")
	}
	if standby {
		args = append(args, "-n", "standby")
	}
	return append(args, attempt.path)
}

// isStandby reports whether smartctl skipped the device because -n standby
// found it in a low-power mode: exit bit 1 plus its "Device is in X mode" message.
func isStandby(output []byte, exitCode int) bool {
	if exitCode&2 == 0 {
		return false
	}
	upper := bytes.ToUpper(output)
	return bytes.Contains(upper, []byte("DEVICE IS IN ")) && bytes.Contains(upper, []byte(" MODE"))
}

// runSmartctl runs smartctl with a timeout.
func (sm *SmartManager) runSmartctl(timeout time.Duration, args ...string) ([]byte, int, error) {
	if sm.run != nil {
		return sm.run(timeout, args...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, sm.smartctlPath, args...).Output()
	// smartctl reports device problems through its exit status bitmask while
	// still printing JSON, so a non-zero exit is not an error here.
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && ctx.Err() == nil {
		return output, exitErr.ExitCode(), nil
	}
	return output, 0, err
}

func (sm *SmartManager) refreshExcludedDevices() {
	rawValue, _ := utils.GetEnv("EXCLUDE_SMART")
	sm.excludedDevices = make(map[string]struct{})

	for entry := range strings.SplitSeq(rawValue, ",") {
		device := strings.TrimSpace(entry)
		if device == "" {
			continue
		}
		sm.excludedDevices[device] = struct{}{}
	}
}

func (sm *SmartManager) isExcludedDevice(deviceName string) bool {
	_, exists := sm.excludedDevices[deviceName]
	return exists
}

// detectSmartctl checks if smartctl is installed, returns an error if not
func (sm *SmartManager) detectSmartctl() (string, error) {
	if runtime.GOOS == "windows" {
		// Load embedded smartctl.exe for Windows amd64 builds.
		if runtime.GOARCH == "amd64" {
			if path, err := ensureEmbeddedSmartctl(); err == nil {
				return path, nil
			}
		}
		// Try to find smartctl in the default installation location
		const location = "C:\\Program Files\\smartmontools\\bin\\smartctl.exe"
		if _, err := os.Stat(location); err == nil {
			return location, nil
		}
	}

	return utils.LookPathHomebrew("smartctl")
}

// isNvmeControllerPath checks if the path matches an NVMe controller pattern
// like /dev/nvme0, /dev/nvme1, etc. (without namespace suffix like n1)
func isNvmeControllerPath(path string) bool {
	base := filepath.Base(path)
	if !strings.HasPrefix(base, "nvme") {
		return false
	}
	suffix := strings.TrimPrefix(base, "nvme")
	if suffix == "" {
		return false
	}
	// Controller paths are just "nvme" + digits (e.g., nvme0, nvme1)
	// Namespace paths have "n" after the controller number (e.g., nvme0n1)
	for _, c := range suffix {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// NewSmartManager creates and initializes a new SmartManager
func NewSmartManager() (*SmartManager, error) {
	sm := &SmartManager{
		data: make(map[string]*smart.SmartData),
	}
	sm.refreshExcludedDevices()
	path, err := sm.detectSmartctl()
	slog.Debug("smartctl", "path", path, "err", err)
	if err != nil {
		// Keep the previous fail-fast behavior unless this Linux host exposes
		// eMMC or mdraid health via sysfs, in which case smartctl is optional.
		if runtime.GOOS == "linux" {
			if len(scanEmmcDevices()) > 0 || len(scanMdraidDevices()) > 0 {
				return sm, nil
			}
		}
		return nil, err
	}
	sm.smartctlPath = path
	return sm, nil
}
