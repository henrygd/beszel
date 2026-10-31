package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/nut"
)

// nutCommandTimeout bounds a single upsc invocation.
const nutCommandTimeout = 10 * time.Second

// NutManager manages collection of UPS/PDU data from Network UPS Tools (NUT).
// It shells out to `upsc` (NUT's CLI), using its legacy text output, mirroring how the
// SmartManager shells out to smartctl.
type NutManager struct {
	sync.Mutex
	NutDataMap        map[string]*nut.NutData
	upscPath          string
	server            string
	authFile          string
	configuredDevices []string
	excludedDevices   map[string]struct{}
	lastListTime      time.Time
	sampledNames      []string // devices collected during the most recent refresh
}

// NewNutManager creates and initializes a NutManager. It returns an error when
// the upsc binary cannot be found, in which case the agent simply does not
// collect NUT data.
func NewNutManager() (*NutManager, error) {
	path, err := utils.LookPathHomebrew("upsc")
	if err != nil {
		return nil, fmt.Errorf("upsc not found: %w", err)
	}

	nm := &NutManager{
		NutDataMap: make(map[string]*nut.NutData),
		upscPath:   path,
		server:     "localhost",
	}

	if server, ok := utils.GetEnv("NUT_SERVER"); ok && strings.TrimSpace(server) != "" {
		nm.server = strings.TrimSpace(server)
	}
	if authFile, ok := utils.GetEnv("NUT_AUTHCONF_FILE"); ok && strings.TrimSpace(authFile) != "" {
		nm.authFile = strings.TrimSpace(authFile)
		// Authentication support is independent of JSON support. Never silently
		// discard explicitly configured connection security settings.
		ctx, cancel := context.WithTimeout(context.Background(), nutCommandTimeout)
		defer cancel()
		if err := exec.CommandContext(ctx, path, "-A", nm.authFile, "-h").Run(); err != nil {
			return nil, fmt.Errorf("NUT_AUTHCONF_FILE requires an upsc client with -A support: %w", err)
		}
	}
	nm.parseConfiguredDevices()
	nm.refreshExcludedDevices()

	slog.Debug("nut manager initialized", "upsc", path, "server", nm.server)
	return nm, nil
}

// parseConfiguredDevices reads the optional NUT_DEVICES env var, which lets the
// operator pin an explicit list of device names instead of relying on discovery.
func (nm *NutManager) parseConfiguredDevices() {
	raw, ok := utils.GetEnv("NUT_DEVICES")
	if !ok {
		return
	}
	separator, _ := utils.GetEnv("NUT_DEVICES_SEPARATOR")
	if separator == "" {
		separator = ","
	}
	for _, entry := range strings.Split(raw, separator) {
		if name := strings.TrimSpace(entry); name != "" {
			nm.configuredDevices = append(nm.configuredDevices, name)
		}
	}
}

// refreshExcludedDevices reads the optional EXCLUDE_NUT env var.
func (nm *NutManager) refreshExcludedDevices() {
	raw, _ := utils.GetEnv("EXCLUDE_NUT")
	nm.excludedDevices = make(map[string]struct{})
	for _, entry := range strings.Split(raw, ",") {
		if name := strings.TrimSpace(entry); name != "" {
			nm.excludedDevices[name] = struct{}{}
		}
	}
}

func (nm *NutManager) isExcluded(name string) bool {
	_, ok := nm.excludedDevices[name]
	return ok
}

// upscArgs builds the argument list for an upsc invocation, prepending the auth
// file flag when configured.
func (nm *NutManager) upscArgs(appendArgs ...string) []string {
	args := make([]string, 0, len(appendArgs)+2)
	if nm.authFile != "" {
		args = append(args, "-A", nm.authFile)
	}
	return append(args, appendArgs...)
}

// runUpsc runs upsc with the given arguments and returns its stdout.
func (nm *NutManager) runUpsc(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), nutCommandTimeout)
	defer cancel()
	return exec.CommandContext(ctx, nm.upscPath, args...).Output()
}

// listDevices returns the discovered NUT device names and their descriptions.
// When NUT_DEVICES is configured it takes precedence over upsc discovery.
func (nm *NutManager) listDevices() (map[string]string, error) {
	if len(nm.configuredDevices) > 0 {
		result := make(map[string]string, len(nm.configuredDevices))
		for _, name := range nm.configuredDevices {
			result[name] = ""
		}
		return result, nil
	}

	if time.Since(nm.lastListTime) < 30*time.Minute {
		nm.Mutex.Lock()
		cached := make(map[string]string, len(nm.NutDataMap))
		for name := range nm.NutDataMap {
			cached[name] = ""
		}
		nm.Mutex.Unlock()
		if len(cached) > 0 {
			return cached, nil
		}
	}

	output, err := nm.runUpsc(nm.upscArgs("-l", nm.server)...)
	if err != nil {
		return nil, err
	}

	devices := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		if len(strings.Fields(name)) != 1 || strings.HasPrefix(name, "-") {
			return nil, fmt.Errorf("invalid upsc device name: %q", name)
		}
		devices[name] = ""
	}
	if len(devices) == 0 {
		return nil, errors.New("no NUT devices found")
	}

	nm.lastListTime = time.Now()
	return devices, nil
}

// Refresh collects NUT data for all discovered devices and reports whether
// every device was collected successfully.
func (nm *NutManager) Refresh() (bool, error) {
	devices, listErr := nm.listDevices()
	if listErr != nil {
		slog.Debug("nut device list failed", "err", listErr)
		return false, listErr
	}

	var collectErr error
	collected := 0
	sampled := make([]string, 0, len(devices))
	for name := range devices {
		if nm.isExcluded(name) {
			continue
		}
		if err := nm.collectDevice(name); err != nil {
			slog.Debug("nut collect failed", "device", name, "err", err)
			collectErr = err
			continue
		}
		sampled = append(sampled, name)
		collected++
	}

	nm.Mutex.Lock()
	nm.sampledNames = sampled
	nm.Mutex.Unlock()

	complete := listErr == nil && collectErr == nil
	if !complete && collected == 0 {
		if listErr != nil {
			return false, listErr
		}
		return false, collectErr
	}
	return complete, nil
}

// collectDevice fetches and stores the variables for a single NUT device.
func (nm *NutManager) collectDevice(name string) error {
	output, err := nm.runUpsc(nm.upscArgs(name + "@" + nm.server)...)
	if err != nil {
		return err
	}

	vars, err := parseUpscVariables(string(output))
	if err != nil {
		return fmt.Errorf("failed to parse upsc vars for %s: %w", name, err)
	}
	if msg, ok := vars["error"]; ok {
		return fmt.Errorf("upsc error for %s: %s", name, msg)
	}

	data := parseNutData(name, vars)

	nm.Mutex.Lock()
	defer nm.Mutex.Unlock()
	nm.NutDataMap[name] = &data
	return nil
}

// parseUpscVariables parses the long-standing "variable: value" output. Split
// only at the first colon so descriptions, timestamps and URLs survive intact.
func parseUpscVariables(output string) (map[string]string, error) {
	vars := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		key = strings.TrimSpace(key)
		if !ok || key == "" || len(strings.Fields(key)) != 1 {
			return nil, fmt.Errorf("invalid variable line: %q", line)
		}
		vars[key] = strings.TrimSpace(value)
	}
	if len(vars) == 0 {
		return nil, errors.New("empty upsc variable response")
	}
	return vars, nil
}

// GetCurrentData returns a copy of the collected NUT data.
func (nm *NutManager) GetCurrentData() map[string]nut.NutData {
	nm.Mutex.Lock()
	defer nm.Mutex.Unlock()
	result := make(map[string]nut.NutData, len(nm.NutDataMap))
	for name, data := range nm.NutDataMap {
		if data != nil {
			result[name] = *data
		}
	}
	return result
}

// GetSampledNames returns the device names collected during the most recent
// refresh. Devices present only in the persistent cache are not included, so
// the hub can tell a fresh measurement from a cached one.
func (nm *NutManager) GetSampledNames() []string {
	nm.Mutex.Lock()
	defer nm.Mutex.Unlock()
	return slices.Clone(nm.sampledNames)
}

// parseNutData maps the flat NUT variable set to a NutData record.
func parseNutData(name string, vars map[string]string) nut.NutData {
	data := nut.NutData{
		Model:        vars["ups.model"],
		Manufacturer: firstNonEmpty(vars["ups.manufacturer"], vars["ups.mfr"], vars["device.mfr"]),
		Serial:       vars["ups.serial"],
		Firmware:     vars["ups.firmware"],
		Driver:       firstNonEmpty(vars["driver"], vars["driver.name"]),
		Status:       vars["ups.status"],
		Health:       deriveHealth(vars["ups.status"]),
	}

	data.BatteryCharge = parseFloat(vars["battery.charge"])
	data.BatteryVoltage = parseFloat(vars["battery.voltage"])
	data.BatteryRuntime = parseInt64(vars["battery.runtime"])
	data.InputVoltage = parseFloat(vars["input.voltage"])
	data.OutputVoltage = parseFloat(vars["output.voltage"])
	data.InputNominal = parseFloat(vars["input.voltage.nominal"])
	data.Load = parseFloat(vars["ups.load"])
	data.OutputCurrent = parseFloat(vars["output.current"])
	data.OutputPower, data.OutputPowerSource = nutRealPower(vars)

	data.DeviceType = detectDeviceType(vars)
	data.Outlets = parseOutlets(vars)

	// Prefer the driver-reported model when the generic one is empty.
	if data.Model == "" {
		data.Model = name
	}
	return data
}

// nutRealPower uses explicit real-power fields, never ups.power (VA) or the
// driver-dependent output.power field. Load-based power is only an estimate.
func nutRealPower(vars map[string]string) (float64, string) {
	read := func(key string) (float64, bool) {
		v, err := strconv.ParseFloat(strings.TrimSpace(vars[key]), 64)
		return v, err == nil && !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0
	}
	for _, key := range []string{"ups.realpower", "output.realpower"} {
		if value, ok := read(key); ok {
			return value, "measured"
		}
	}
	load, hasLoad := read("ups.load")
	nominal, hasNominal := read("ups.realpower.nominal")
	if hasLoad && hasNominal && nominal > 0 {
		power := load / 100 * nominal
		if !math.IsInf(power, 0) {
			return power, "estimated"
		}
	}
	return 0, "unknown"
}

// detectDeviceType classifies a device as ups, pdu, or other based on the
// variables it exposes.
func detectDeviceType(vars map[string]string) string {
	_, hasBattery := vars["battery.charge"]
	_, hasBatteryVoltage := vars["battery.voltage"]
	hasOutlet := hasOutletStatus(vars)

	switch {
	case hasBattery || hasBatteryVoltage:
		return nut.DeviceTypeUPS
	case hasOutlet:
		return nut.DeviceTypePDU
	default:
		return nut.DeviceTypeOther
	}
}

// hasOutletStatus reports whether the variable set contains any outlet status.
func hasOutletStatus(vars map[string]string) bool {
	for key := range vars {
		if strings.HasPrefix(key, "outlet.") && strings.HasSuffix(key, ".status") {
			return true
		}
	}
	return false
}

// parseOutlets extracts PDU outlet records from the flat variable set.
func parseOutlets(vars map[string]string) []*nut.NutOutlet {
	ids := map[string]struct{}{}
	for key := range vars {
		if strings.HasPrefix(key, "outlet.") && strings.HasSuffix(key, ".status") {
			id := strings.TrimSuffix(strings.TrimPrefix(key, "outlet."), ".status")
			ids[id] = struct{}{}
		}
	}
	if len(ids) == 0 {
		return nil
	}

	outlets := make([]*nut.NutOutlet, 0, len(ids))
	for id := range ids {
		outlets = append(outlets, &nut.NutOutlet{
			ID:      id,
			Desc:    vars["outlet."+id+".desc"],
			Status:  normalizeOutletStatus(vars["outlet."+id+".status"]),
			Current: parseFloat(vars["outlet."+id+".current"]),
			Power:   parseFloat(vars["outlet."+id+".power"]),
			Energy:  parseFloat(vars["outlet."+id+".energy"]),
			Voltage: parseFloat(vars["outlet."+id+".voltage"]),
		})
	}
	return outlets
}

// normalizeOutletStatus maps NUT outlet status values to on/off/unknown.
func normalizeOutletStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "on", "1":
		return "on"
	case "off", "0":
		return "off"
	default:
		return "unknown"
	}
}

// deriveHealth maps the NUT ups.status flags to a single health state.
func deriveHealth(status string) string {
	tokens := make(map[string]struct{})
	for _, t := range strings.Fields(strings.ToUpper(status)) {
		tokens[t] = struct{}{}
	}
	has := func(flag string) bool {
		_, ok := tokens[flag]
		return ok
	}
	// Most severe state wins: a committed alarm or forced shutdown must not
	// stay green, and an overload must not be masked by the battery flag.
	switch {
	case has("SD"), has("OFF"), has("OFFB"), has("COMM"), has("FSD"), has("ALARM"):
		return nut.HealthFault
	case has("LB"):
		return nut.HealthLowBattery
	case has("OVER"):
		return nut.HealthOverload
	case has("OB"):
		return nut.HealthOnBattery
	case has("OL"):
		return nut.HealthOnline
	default:
		return nut.HealthUnknown
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func parseFloat(s string) float64 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}

func parseInt64(s string) int64 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return v
}
