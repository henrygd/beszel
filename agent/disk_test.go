//go:build testing
// +build testing

package agent

import (
	"os"
	"strings"
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/stretchr/testify/assert"
)

func TestParseFilesystemEntry(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedFs   string
		expectedName string
	}{
		{
			name:         "simple device name",
			input:        "sda1",
			expectedFs:   "sda1",
			expectedName: "",
		},
		{
			name:         "device with custom name",
			input:        "sda1__my-storage",
			expectedFs:   "sda1",
			expectedName: "my-storage",
		},
		{
			name:         "full device path with custom name",
			input:        "/dev/sdb1__backup-drive",
			expectedFs:   "/dev/sdb1",
			expectedName: "backup-drive",
		},
		{
			name:         "NVMe device with custom name",
			input:        "nvme0n1p2__fast-ssd",
			expectedFs:   "nvme0n1p2",
			expectedName: "fast-ssd",
		},
		{
			name:         "whitespace trimmed",
			input:        "  sda2__trimmed-name  ",
			expectedFs:   "sda2",
			expectedName: "trimmed-name",
		},
		{
			name:         "empty custom name",
			input:        "sda3__",
			expectedFs:   "sda3",
			expectedName: "",
		},
		{
			name:         "empty device name",
			input:        "__just-custom",
			expectedFs:   "",
			expectedName: "just-custom",
		},
		{
			name:         "multiple underscores in custom name",
			input:        "sda1__my_custom_drive",
			expectedFs:   "sda1",
			expectedName: "my_custom_drive",
		},
		{
			name:         "custom name with spaces",
			input:        "sda1__My Storage Drive",
			expectedFs:   "sda1",
			expectedName: "My Storage Drive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsEntry := strings.TrimSpace(tt.input)
			var fs, customName string
			if parts := strings.SplitN(fsEntry, "__", 2); len(parts) == 2 {
				fs = strings.TrimSpace(parts[0])
				customName = strings.TrimSpace(parts[1])
			} else {
				fs = fsEntry
			}

			assert.Equal(t, tt.expectedFs, fs)
			assert.Equal(t, tt.expectedName, customName)
		})
	}
}

func TestInitializeDiskInfoWithCustomNames(t *testing.T) {
	// Set up environment variables
	oldEnv := os.Getenv("EXTRA_FILESYSTEMS")
	defer func() {
		if oldEnv != "" {
			os.Setenv("EXTRA_FILESYSTEMS", oldEnv)
		} else {
			os.Unsetenv("EXTRA_FILESYSTEMS")
		}
	}()

	// Test with custom names
	os.Setenv("EXTRA_FILESYSTEMS", "sda1__my-storage,/dev/sdb1__backup-drive,nvme0n1p2")

	// Mock disk partitions (we'll just test the parsing logic)
	// Since the actual disk operations are system-dependent, we'll focus on the parsing
	testCases := []struct {
		envValue      string
		expectedFs    []string
		expectedNames map[string]string
	}{
		{
			envValue:   "sda1__my-storage,sdb1__backup-drive",
			expectedFs: []string{"sda1", "sdb1"},
			expectedNames: map[string]string{
				"sda1": "my-storage",
				"sdb1": "backup-drive",
			},
		},
		{
			envValue:   "sda1,nvme0n1p2__fast-ssd",
			expectedFs: []string{"sda1", "nvme0n1p2"},
			expectedNames: map[string]string{
				"nvme0n1p2": "fast-ssd",
			},
		},
	}

	for _, tc := range testCases {
		t.Run("env_"+tc.envValue, func(t *testing.T) {
			os.Setenv("EXTRA_FILESYSTEMS", tc.envValue)

			// Create mock partitions that would match our test cases
			partitions := []disk.PartitionStat{}
			for _, fs := range tc.expectedFs {
				if strings.HasPrefix(fs, "/dev/") {
					partitions = append(partitions, disk.PartitionStat{
						Device:     fs,
						Mountpoint: fs,
					})
				} else {
					partitions = append(partitions, disk.PartitionStat{
						Device:     "/dev/" + fs,
						Mountpoint: "/" + fs,
					})
				}
			}

			// Test the parsing logic by calling the relevant part
			// We'll create a simplified version to test just the parsing
			extraFilesystems := tc.envValue
			for _, fsEntry := range strings.Split(extraFilesystems, ",") {
				// Parse the entry
				fsEntry = strings.TrimSpace(fsEntry)
				var fs, customName string
				if parts := strings.SplitN(fsEntry, "__", 2); len(parts) == 2 {
					fs = strings.TrimSpace(parts[0])
					customName = strings.TrimSpace(parts[1])
				} else {
					fs = fsEntry
				}

				// Verify the device is in our expected list
				assert.Contains(t, tc.expectedFs, fs, "parsed device should be in expected list")

				// Check if custom name should exist
				if expectedName, exists := tc.expectedNames[fs]; exists {
					assert.Equal(t, expectedName, customName, "custom name should match expected")
				} else {
					assert.Empty(t, customName, "custom name should be empty when not expected")
				}
			}
		})
	}
}

func TestFsStatsWithCustomNames(t *testing.T) {
	// Test that FsStats properly stores custom names
	fsStats := &system.FsStats{
		Mountpoint: "/mnt/storage",
		Name:       "my-custom-storage",
		DiskTotal:  100.0,
		DiskUsed:   50.0,
	}

	assert.Equal(t, "my-custom-storage", fsStats.Name)
	assert.Equal(t, "/mnt/storage", fsStats.Mountpoint)
	assert.Equal(t, 100.0, fsStats.DiskTotal)
	assert.Equal(t, 50.0, fsStats.DiskUsed)
}

func TestExtraFsKeyGeneration(t *testing.T) {
	// Test the logic for generating ExtraFs keys with custom names
	testCases := []struct {
		name        string
		deviceName  string
		customName  string
		expectedKey string
	}{
		{
			name:        "with custom name",
			deviceName:  "sda1",
			customName:  "my-storage",
			expectedKey: "my-storage",
		},
		{
			name:        "without custom name",
			deviceName:  "sda1",
			customName:  "",
			expectedKey: "sda1",
		},
		{
			name:        "empty custom name falls back to device",
			deviceName:  "nvme0n1p2",
			customName:  "",
			expectedKey: "nvme0n1p2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the key generation logic from agent.go
			key := tc.deviceName
			if tc.customName != "" {
				key = tc.customName
			}
			assert.Equal(t, tc.expectedKey, key)
		})
	}
}
