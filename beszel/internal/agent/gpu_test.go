package agent

import (
	"beszel/internal/entities/system"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseNvidiaData(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantData  map[string]system.GPUData
		wantValid bool
	}{
		{
			name:  "valid multi-gpu data",
			input: "0, NVIDIA GeForce RTX 3050 Ti Laptop GPU, 48, 12, 4096, 26.3, 12.73\n1, NVIDIA A100-PCIE-40GB, 38, 74, 40960, [N/A], 36.79",
			wantData: map[string]system.GPUData{
				"0": {
					Name:        "GeForce RTX 3050 Ti",
					Temperature: 48.0,
					MemoryUsed:  12.0 / 1.024,
					MemoryTotal: 4096.0 / 1.024,
					Usage:       26.3,
					Power:       12.73,
					Count:       1,
				},
				"1": {
					Name:        "A100-PCIE-40GB",
					Temperature: 38.0,
					MemoryUsed:  74.0 / 1.024,
					MemoryTotal: 40960.0 / 1.024,
					Usage:       0.0,
					Power:       36.79,
					Count:       1,
				},
			},
			wantValid: true,
		},
		{
			name:      "empty input",
			input:     "",
			wantData:  map[string]system.GPUData{},
			wantValid: false,
		},
		{
			name:      "malformed data",
			input:     "bad, data, here",
			wantData:  map[string]system.GPUData{},
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gm := &GPUManager{
				GpuDataMap: make(map[string]*system.GPUData),
			}
			valid := gm.parseNvidiaData([]byte(tt.input))
			assert.Equal(t, tt.wantValid, valid)

			if tt.wantValid {
				for id, want := range tt.wantData {
					got := gm.GpuDataMap[id]
					require.NotNil(t, got)
					assert.Equal(t, want.Name, got.Name)
					assert.InDelta(t, want.Temperature, got.Temperature, 0.01)
					assert.InDelta(t, want.MemoryUsed, got.MemoryUsed, 0.01)
					assert.InDelta(t, want.MemoryTotal, got.MemoryTotal, 0.01)
					assert.InDelta(t, want.Usage, got.Usage, 0.01)
					assert.InDelta(t, want.Power, got.Power, 0.01)
					assert.Equal(t, want.Count, got.Count)
				}
			}
		})
	}
}

func TestParseAmdData(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantData  map[string]system.GPUData
		wantValid bool
	}{
		{
			name: "valid single gpu data",
			input: `{
				"card0": {
					"GUID": "34756",
					"Temperature (Sensor edge) (C)": "47.0",
					"Current Socket Graphics Package Power (W)": "9.215",
					"GPU use (%)": "0",
					"VRAM Total Memory (B)": "536870912",
					"VRAM Total Used Memory (B)": "482263040",
					"Card Series": "Rembrandt [Radeon 680M]"
				}
			}`,
			wantData: map[string]system.GPUData{
				"34756": {
					Name:        "Rembrandt [Radeon 680M]",
					Temperature: 47.0,
					MemoryUsed:  482263040.0 / (1024 * 1024),
					MemoryTotal: 536870912.0 / (1024 * 1024),
					Usage:       0.0,
					Power:       9.215,
					Count:       1,
				},
			},
			wantValid: true,
		},
		{
			name:      "invalid json",
			input:     "{bad json",
			wantData:  map[string]system.GPUData{},
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gm := &GPUManager{
				GpuDataMap: make(map[string]*system.GPUData),
			}
			valid := gm.parseAmdData([]byte(tt.input))
			assert.Equal(t, tt.wantValid, valid)

			if tt.wantValid {
				for id, want := range tt.wantData {
					got := gm.GpuDataMap[id]
					require.NotNil(t, got)
					assert.Equal(t, want.Name, got.Name)
					assert.InDelta(t, want.Temperature, got.Temperature, 0.01)
					assert.InDelta(t, want.MemoryUsed, got.MemoryUsed, 0.01)
					assert.InDelta(t, want.MemoryTotal, got.MemoryTotal, 0.01)
					assert.InDelta(t, want.Usage, got.Usage, 0.01)
					assert.InDelta(t, want.Power, got.Power, 0.01)
					assert.Equal(t, want.Count, got.Count)
				}
			}
		})
	}
}

func TestParseJetsonData(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		gm          *GPUManager
		wantMetrics *system.GPUData
	}{
		{
			name:  "valid data",
			input: "RAM 4300/30698MB GR3D_FREQ 45% tj@52.468C VDD_GPU_SOC 2171mW",
			wantMetrics: &system.GPUData{
				Name:        "Jetson",
				MemoryUsed:  4300.0,
				MemoryTotal: 30698.0,
				Usage:       45.0,
				Temperature: 52.468,
				Power:       2.171,
				Count:       1,
			},
		},
		{
			name:  "missing temperature",
			input: "RAM 4300/30698MB GR3D_FREQ 45% VDD_GPU_SOC 2171mW",
			wantMetrics: &system.GPUData{
				Name:        "Jetson",
				MemoryUsed:  4300.0,
				MemoryTotal: 30698.0,
				Usage:       45.0,
				Power:       2.171,
				Count:       1,
			},
		},
		{
			name:  "no gpu defined by nvidia-smi",
			input: "RAM 4300/30698MB GR3D_FREQ 45% VDD_GPU_SOC 2171mW",
			gm: &GPUManager{
				GpuDataMap: map[string]*system.GPUData{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.gm != nil {
				// should return if no gpu set by nvidia-smi
				assert.Empty(t, tt.gm.GpuDataMap)
				return
			}
			tt.gm = &GPUManager{
				GpuDataMap: map[string]*system.GPUData{
					"0": {Name: "Jetson"},
				},
			}
			parser := tt.gm.getJetsonParser()
			valid := parser([]byte(tt.input))
			assert.Equal(t, true, valid)

			got := tt.gm.GpuDataMap["0"]
			require.NotNil(t, got)
			assert.Equal(t, tt.wantMetrics.Name, got.Name)
			assert.InDelta(t, tt.wantMetrics.MemoryUsed, got.MemoryUsed, 0.01)
			assert.InDelta(t, tt.wantMetrics.MemoryTotal, got.MemoryTotal, 0.01)
			assert.InDelta(t, tt.wantMetrics.Usage, got.Usage, 0.01)
			if tt.wantMetrics.Temperature > 0 {
				assert.InDelta(t, tt.wantMetrics.Temperature, got.Temperature, 0.01)
			}
			assert.InDelta(t, tt.wantMetrics.Power, got.Power, 0.01)
			assert.Equal(t, tt.wantMetrics.Count, got.Count)
		})
	}
}

func TestGetCurrentData(t *testing.T) {
	gm := &GPUManager{
		GpuDataMap: map[string]*system.GPUData{
			"0": {
				Name:        "GPU1",
				Temperature: 50,
				MemoryUsed:  2048,
				MemoryTotal: 4096,
				Usage:       100, // 100 over 2 counts = 50 avg
				Power:       200, // 200 over 2 counts = 100 avg
				Count:       2,
			},
			"1": {
				Name:        "GPU1",
				Temperature: 60,
				MemoryUsed:  3072,
				MemoryTotal: 8192,
				Usage:       30,
				Power:       60,
				Count:       1,
			},
		},
	}

	result := gm.GetCurrentData()

	// Verify name disambiguation
	assert.Equal(t, "GPU1 0", result["0"].Name)
	assert.Equal(t, "GPU1 1", result["1"].Name)

	// Check averaged values
	assert.InDelta(t, 50.0, result["0"].Usage, 0.01)
	assert.InDelta(t, 100.0, result["0"].Power, 0.01)
	assert.InDelta(t, 30.0, result["1"].Usage, 0.01)
	assert.InDelta(t, 60.0, result["1"].Power, 0.01)

	// Verify reset counts
	assert.Equal(t, float64(1), gm.GpuDataMap["0"].Count)
	assert.Equal(t, float64(1), gm.GpuDataMap["1"].Count)
}

func TestDetectGPUs(t *testing.T) {
	// Save original PATH
	origPath := os.Getenv("PATH")
	defer os.Setenv("PATH", origPath)

	// Set up temp dir with the commands
	tempDir := t.TempDir()
	os.Setenv("PATH", tempDir)

	tests := []struct {
		name           string
		setupCommands  func() error
		wantNvidiaSmi  bool
		wantRocmSmi    bool
		wantTegrastats bool
		wantErr        bool
	}{
		{
			name: "nvidia-smi not available",
			setupCommands: func() error {
				return nil
			},
			wantNvidiaSmi:  false,
			wantRocmSmi:    false,
			wantTegrastats: false,
			wantErr:        true,
		},
		{
			name: "nvidia-smi available",
			setupCommands: func() error {
				path := filepath.Join(tempDir, "nvidia-smi")
				script := `#!/bin/sh
echo "test"`
				if err := os.WriteFile(path, []byte(script), 0755); err != nil {
					return err
				}
				return nil
			},
			wantNvidiaSmi:  true,
			wantTegrastats: false,
			wantRocmSmi:    false,
			wantErr:        false,
		},
		{
			name: "rocm-smi available",
			setupCommands: func() error {
				path := filepath.Join(tempDir, "rocm-smi")
				script := `#!/bin/sh
echo "test"`
				if err := os.WriteFile(path, []byte(script), 0755); err != nil {
					return err
				}
				return nil
			},
			wantNvidiaSmi:  true,
			wantRocmSmi:    true,
			wantTegrastats: false,
			wantErr:        false,
		},
		{
			name: "tegrastats available",
			setupCommands: func() error {
				path := filepath.Join(tempDir, "tegrastats")
				script := `#!/bin/sh
echo "test"`
				if err := os.WriteFile(path, []byte(script), 0755); err != nil {
					return err
				}
				return nil
			},
			wantNvidiaSmi:  true,
			wantRocmSmi:    true,
			wantTegrastats: true,
			wantErr:        false,
		},
		{
			name: "no gpu tools available",
			setupCommands: func() error {
				os.Setenv("PATH", "")
				return nil
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			if err := tt.setupCommands(); err != nil {
				t.Fatal(err)
			}

			gm := &GPUManager{}
			err := gm.detectGPUs()

			t.Logf("nvidiaSmi: %v, rocmSmi: %v, tegrastats: %v", gm.nvidiaSmi, gm.rocmSmi, gm.tegrastats)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantNvidiaSmi, gm.nvidiaSmi)
			assert.Equal(t, tt.wantRocmSmi, gm.rocmSmi)
			assert.Equal(t, tt.wantTegrastats, gm.tegrastats)
		})
	}
}

func TestStartCollector(t *testing.T) {
	// Save original PATH
	origPath := os.Getenv("PATH")
	defer os.Setenv("PATH", origPath)

	// Set up temp dir with the commands
	dir := t.TempDir()
	os.Setenv("PATH", dir)

	tests := []struct {
		name     string
		command  string
		setup    func(t *testing.T) error
		validate func(t *testing.T, gm *GPUManager)
		gm       *GPUManager
	}{
		{
			name:    "nvidia-smi collector",
			command: "nvidia-smi",
			setup: func(t *testing.T) error {
				path := filepath.Join(dir, "nvidia-smi")
				script := `#!/bin/sh
echo "0, NVIDIA Test GPU, 50, 1024, 4096, 25, 100"`
				if err := os.WriteFile(path, []byte(script), 0755); err != nil {
					return err
				}
				return nil
			},
			validate: func(t *testing.T, gm *GPUManager) {
				gpu, exists := gm.GpuDataMap["0"]
				assert.True(t, exists)
				if exists {
					assert.Equal(t, "Test GPU", gpu.Name)
					assert.Equal(t, 50.0, gpu.Temperature)

				}
			},
		},
		{
			name:    "rocm-smi collector",
			command: "rocm-smi",
			setup: func(t *testing.T) error {
				path := filepath.Join(dir, "rocm-smi")
				script := `#!/bin/sh
echo '{"card0": {"Temperature (Sensor edge) (C)": "49.0", "Current Socket Graphics Package Power (W)": "28.159", "GPU use (%)": "0", "VRAM Total Memory (B)": "536870912", "VRAM Total Used Memory (B)": "445550592", "Card Series": "Rembrandt [Radeon 680M]", "Card Model": "0x1681", "Card Vendor": "Advanced Micro Devices, Inc. [AMD/ATI]", "Card SKU": "REMBRANDT", "Subsystem ID": "0x8a22", "Device Rev": "0xc8", "Node ID": "1", "GUID": "34756", "GFX Version": "gfx1035"}}'`
				if err := os.WriteFile(path, []byte(script), 0755); err != nil {
					return err
				}
				return nil
			},
			validate: func(t *testing.T, gm *GPUManager) {
				gpu, exists := gm.GpuDataMap["34756"]
				assert.True(t, exists)
				if exists {
					assert.Equal(t, "Rembrandt [Radeon 680M]", gpu.Name)
					assert.InDelta(t, 49.0, gpu.Temperature, 0.01)
					assert.InDelta(t, 28.159, gpu.Power, 0.01)
				}
			},
		},
		{
			name:    "tegrastats collector",
			command: "tegrastats",
			setup: func(t *testing.T) error {
				path := filepath.Join(dir, "tegrastats")
				script := `#!/bin/sh
echo "RAM 1024/4096MB GR3D_FREQ 80% tj@70C VDD_GPU_SOC 1000mW"`
				if err := os.WriteFile(path, []byte(script), 0755); err != nil {
					return err
				}
				return nil
			},
			validate: func(t *testing.T, gm *GPUManager) {
				gpu, exists := gm.GpuDataMap["0"]
				assert.True(t, exists)
				if exists {
					assert.InDelta(t, 70.0, gpu.Temperature, 0.1)
				}
			},
			gm: &GPUManager{
				GpuDataMap: map[string]*system.GPUData{
					"0": {},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.setup(t); err != nil {
				t.Fatal(err)
			}
			if tt.gm == nil {
				tt.gm = &GPUManager{
					GpuDataMap: make(map[string]*system.GPUData),
				}
			}
			tt.gm.startCollector(tt.command)
			time.Sleep(50 * time.Millisecond) // Give collector time to run
			tt.validate(t, tt.gm)
		})
	}
}
