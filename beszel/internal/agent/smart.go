package agent

import (
	"beszel/internal/entities/smart"
	"beszel/internal/entities/system"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"reflect"
	"sync"
	"time"

	"golang.org/x/exp/slog"
)

// SmartManager manages data collection for SMART devices
// TODO: add retry argument
// TODO: add timeout argument
type SmartManager struct {
	SmartDataMap map[string]*system.SmartData
	SmartDevices []*DeviceInfo
	mutex        sync.Mutex
}

type scanOutput struct {
	Devices []struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		InfoName string `json:"info_name"`
		Protocol string `json:"protocol"`
	} `json:"devices"`
}

type DeviceInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	InfoName string `json:"info_name"`
	Protocol string `json:"protocol"`
}

var errNoValidSmartData = fmt.Errorf("no valid GPU data found") // Error for missing data

// Starts the SmartManager
func (sm *SmartManager) Start() {
	sm.SmartDataMap = make(map[string]*system.SmartData)
	for {
		err := sm.ScanDevices()
		if err != nil {
			slog.Warn("smartctl scan failed, stopping", "err", err)
			return
		}
		// TODO: add retry logic
		for _, deviceInfo := range sm.SmartDevices {
			err := sm.CollectSmart(deviceInfo)
			if err != nil {
				slog.Warn("smartctl collect failed, stopping", "err", err)
				return
			}
		}
		// Sleep for 10 seconds before next scan
		time.Sleep(10 * time.Second)
	}
}

// GetCurrentData returns the current SMART data
func (sm *SmartManager) GetCurrentData() map[string]system.SmartData {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	result := make(map[string]system.SmartData)
	for key, value := range sm.SmartDataMap {
		result[key] = *value
	}
	return result
}

// ScanDevices scans for SMART devices
// Scan devices using `smartctl --scan -j`
// If scan fails, return error
// If scan succeeds, parse the output and update the SmartDevices slice
func (sm *SmartManager) ScanDevices() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "smartctl", "--scan", "-j")
	output, err := cmd.Output()

	if err != nil {
		return err
	}

	hasValidData := sm.parseScan(output)
	if !hasValidData {
		return errNoValidSmartData
	}
	return nil
}

// CollectSmart collects SMART data for a device
// Collect data using `smartctl --all -j /dev/sdX` or `smartctl --all -j /dev/nvmeX`
// If collect fails, return error
// If collect succeeds, parse the output and update the SmartDataMap
func (sm *SmartManager) CollectSmart(deviceInfo *DeviceInfo) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "smartctl", "--all", "-j", deviceInfo.Name)

	output, err := cmd.Output()

	if err != nil {
		return err
	}
	
	hasValidData := false
	if deviceInfo.Type == "scsi" {
		// parse scsi devices
		hasValidData = sm.parseSmartForScsi(output)
	} else if deviceInfo.Type == "nvme" {
		// parse nvme devices
		hasValidData = sm.parseSmartForNvme(output)
	}

	if !hasValidData {
		return errNoValidSmartData
	}
	return nil
}

// parseScan parses the output of smartctl --scan -j and updates the SmartDevices slice
func (sm *SmartManager) parseScan(output []byte) bool {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	sm.SmartDevices = make([]*DeviceInfo, 0)
	scan := &scanOutput{}

	if err := json.Unmarshal(output, scan); err != nil {
		fmt.Printf("Failed to parse JSON: %v\n", err)
		return false
	}

	scannedDeviceNameMap := make(map[string]bool)

	for _, device := range scan.Devices {
		deviceInfo := &DeviceInfo{
			Name:     device.Name,
			Type:     device.Type,
			InfoName: device.InfoName,
			Protocol: device.Protocol,
		}
		sm.SmartDevices = append(sm.SmartDevices, deviceInfo)
		scannedDeviceNameMap[device.Name] = true
	}
	// remove devices that are not in the scan
	for key := range sm.SmartDataMap {
		if _, ok := scannedDeviceNameMap[key]; !ok {
			delete(sm.SmartDataMap, key)
		}
	}
	devicesString := ""
	for _, device := range sm.SmartDevices {
		devicesString += device.Name + " "
	}

	return true
}

// parseSmartForScsi parses the output of smartctl --all -j /dev/sdX and updates the SmartDataMap
func (sm *SmartManager) parseSmartForScsi(output []byte) bool {
	data := &smart.SmartInfoForSata{}

	if err := json.Unmarshal(output, &data); err != nil {
		return false
	}

	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// get device name (e.g. /dev/sda)
	keyName := data.SerialNumber

	// if device does not exist in SmartDataMap, initialize it
	if _, ok := sm.SmartDataMap[keyName]; !ok {
		sm.SmartDataMap[keyName] = &system.SmartData{}
	}

	// update SmartData
	smartData := sm.SmartDataMap[keyName]
	smartData.ModelFamily = data.ModelFamily
	smartData.ModelName = data.ModelName
	smartData.SerialNumber = data.SerialNumber
	smartData.FirmwareVersion = data.FirmwareVersion
	smartData.Capacity = data.UserCapacity.Bytes
	if data.SmartStatus.Passed {
		smartData.SmartStatus = "PASSED"
	} else {
		smartData.SmartStatus = "FAILED"
	}
	smartData.DiskName = data.Device.Name
	smartData.DiskType = data.Device.Type

	// update SmartAttributes
	smartData.Attributes = make([]*system.SmartAttribute, 0, len(data.AtaSmartAttributes.Table))
	for _, attr := range data.AtaSmartAttributes.Table {
		smartAttr := &system.SmartAttribute{
			Id:         attr.ID,
			Name:       attr.Name,
			Value:      attr.Value,
			Worst:      attr.Worst,
			Threshold:  attr.Thresh,
			RawValue:   attr.Raw.Value,
			RawString:  attr.Raw.String,
			Flags:      attr.Flags.String,
			WhenFailed: attr.WhenFailed,
		}
		smartData.Attributes = append(smartData.Attributes, smartAttr)
	}
	smartData.Temperature = data.Temperature.Current
	sm.SmartDataMap[keyName] = smartData

	return true
}

// parseSmartForNvme parses the output of smartctl --all -j /dev/nvmeX and updates the SmartDataMap
func (sm *SmartManager) parseSmartForNvme(output []byte) bool {
	data := &smart.SmartInfoForNvme{}

	if err := json.Unmarshal(output, &data); err != nil {
		return false
	}

	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// get device name (e.g. /dev/nvme0)
	keyName := data.SerialNumber

	// if device does not exist in SmartDataMap, initialize it
	if _, ok := sm.SmartDataMap[keyName]; !ok {
		sm.SmartDataMap[keyName] = &system.SmartData{}
	}

	// update SmartData
	smartData := sm.SmartDataMap[keyName]
	smartData.ModelName = data.ModelName
	smartData.SerialNumber = data.SerialNumber
	smartData.FirmwareVersion = data.FirmwareVersion
	smartData.Capacity = data.UserCapacity.Bytes
	if data.SmartStatus.Passed {
		smartData.SmartStatus = "PASSED"
	} else {
		smartData.SmartStatus = "FAILED"
	}
	smartData.DiskName = data.Device.Name
	smartData.DiskType = data.Device.Type

	v := reflect.ValueOf(data.NVMeSmartHealthInformationLog)
	t := v.Type()
	smartData.Attributes = make([]*system.SmartAttribute, 0, v.NumField())

	// nvme attributes does not follow the same format as ata attributes,
	// so we have to manually iterate over the fields abd update SmartAttributes
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i)
		key := field.Name
		val := value.Interface()
		// drop non int values
		if _, ok := val.(int); !ok {
			continue
		}
		smartAttr := &system.SmartAttribute{
			Name:  key,
			Value: val.(int),
		}
		smartData.Attributes = append(smartData.Attributes, smartAttr)
	}
	smartData.Temperature = data.NVMeSmartHealthInformationLog.Temperature

	sm.SmartDataMap[keyName] = smartData

	return true
}

// detectSmartctl checks if smartctl is installed, returns an error if not
func (sm *SmartManager) detectSmartctl() error {
	if _, err := exec.LookPath("smartctl"); err == nil {
		return nil
	}
	return fmt.Errorf("no smartctl found - install smartctl")
}

// NewGPUManager creates and initializes a new GPUManager
func NewSmartManager() (*SmartManager, error) {
	var sm SmartManager
	if err := sm.detectSmartctl(); err != nil {
		return nil, err
	}

	go sm.Start()

	return &sm, nil
}
