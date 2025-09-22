//go:build testing
// +build testing

package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"

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
			name: "more valid multi-gpu data",
			input: `0, NVIDIA A10, 45, 19676, 23028, 0, 58.98
1, NVIDIA A10, 45, 19638, 23028, 0, 62.35
2, NVIDIA A10, 44, 21700, 23028, 0, 59.57
3, NVIDIA A10, 45, 18222, 23028, 0, 61.76`,
			wantData: map[string]system.GPUData{
				"0": {
					Name:        "A10",
					Temperature: 45.0,
					MemoryUsed:  19676.0 / 1.024,
					MemoryTotal: 23028.0 / 1.024,
					Usage:       0.0,
					Power:       58.98,
					Count:       1,
				},
				"1": {
					Name:        "A10",
					Temperature: 45.0,
					MemoryUsed:  19638.0 / 1.024,
					MemoryTotal: 23028.0 / 1.024,
					Usage:       0.0,
					Power:       62.35,
					Count:       1,
				},
				"2": {
					Name:        "A10",
					Temperature: 44.0,
					MemoryUsed:  21700.0 / 1.024,
					MemoryTotal: 23028.0 / 1.024,
					Usage:       0.0,
					Power:       59.57,
					Count:       1,
				},
				"3": {
					Name:        "A10",
					Temperature: 45.0,
					MemoryUsed:  18222.0 / 1.024,
					MemoryTotal: 23028.0 / 1.024,
					Usage:       0.0,
					Power:       61.76,
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
			name: "valid multi gpu data",
			input: `{
				"card0": {
					"GUID": "34756",
					"Temperature (Sensor edge) (C)": "47.0",
					"Current Socket Graphics Package Power (W)": "9.215",
					"GPU use (%)": "0",
					"VRAM Total Memory (B)": "536870912",
					"VRAM Total Used Memory (B)": "482263040",
					"Card Series": "Rembrandt [Radeon 680M]"
				},
				"card1": {
					"GUID": "38294",
					"Temperature (Sensor edge) (C)": "49.0",
					"Temperature (Sensor junction) (C)": "49.0",
					"Temperature (Sensor memory) (C)": "62.0",
					"Average Graphics Package Power (W)": "19.0",
					"GPU use (%)": "20.3",
					"VRAM Total Memory (B)": "25753026560",
					"VRAM Total Used Memory (B)": "794341376",
					"Card Series": "Navi 31 [Radeon RX 7900 XT]"
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
				"38294": {
					Name:        "Navi 31 [Radeon RX 7900 XT]",
					Temperature: 49.0,
					MemoryUsed:  794341376.0 / (1024 * 1024),
					MemoryTotal: 25753026560.0 / (1024 * 1024),
					Usage:       20.3,
					Power:       19.0,
					Count:       1,
				},
			},
			wantValid: true,
		},
		{
			name:  "invalid json",
			input: "{bad json",
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
		wantMetrics *system.GPUData
	}{
		{
			name:  "valid data",
			input: "11-14-2024 22:54:33 RAM 4300/30698MB GR3D_FREQ 45% tj@52.468C VDD_GPU_SOC 2171mW",
			wantMetrics: &system.GPUData{
				Name:        "GPU",
				MemoryUsed:  4300.0,
				MemoryTotal: 30698.0,
				Usage:       45.0,
				Temperature: 52.468,
				Power:       2.171,
				Count:       1,
			},
		},
		{
			name:  "more valid data",
			input: "11-15-2024 08:38:09 RAM 6185/7620MB (lfb 8x2MB) SWAP 851/3810MB (cached 1MB) CPU [15%@729,11%@729,14%@729,13%@729,11%@729,8%@729] EMC_FREQ 43%@2133 GR3D_FREQ 63%@[621] NVDEC off NVJPG off NVJPG1 off VIC off OFA off APE 200 cpu@53.968C soc2@52.437C soc0@50.75C gpu@53.343C tj@53.968C soc1@51.656C VDD_IN 12479mW/12479mW VDD_CPU_GPU_CV 4667mW/4667mW VDD_SOC 2817mW/2817mW",
			wantMetrics: &system.GPUData{
				Name:        "GPU",
				MemoryUsed:  6185.0,
				MemoryTotal: 7620.0,
				Usage:       63.0,
				Temperature: 53.968,
				Power:       4.667,
				Count:       1,
			},
		},
		{
			name:  "orin nano",
			input: "06-18-2025 11:25:24 RAM 3452/7620MB (lfb 25x4MB) SWAP 1518/16384MB (cached 174MB) CPU [1%@1420,2%@1420,0%@1420,2%@1420,2%@729,1%@729] GR3D_FREQ 0% cpu@50.031C soc2@49.031C soc0@50C gpu@49.031C tj@50.25C soc1@50.25C VDD_IN 4824mW/4824mW VDD_CPU_GPU_CV 518mW/518mW VDD_SOC 1475mW/1475mW",
			wantMetrics: &system.GPUData{
				Name:        "GPU",
				MemoryUsed:  3452.0,
				MemoryTotal: 7620.0,
				Usage:       0.0,
				Temperature: 50.25,
				Power:       0.518,
				Count:       1,
			},
		},
		{
			name:  "missing temperature",
			input: "11-14-2024 22:54:33 RAM 4300/30698MB GR3D_FREQ 45% VDD_GPU_SOC 2171mW",
			wantMetrics: &system.GPUData{
				Name:        "GPU",
				MemoryUsed:  4300.0,
				MemoryTotal: 30698.0,
				Usage:       45.0,
				Power:       2.171,
				Count:       1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gm := &GPUManager{
				GpuDataMap: make(map[string]*system.GPUData),
			}
			parser := gm.getJetsonParser()
			valid := parser([]byte(tt.input))
			assert.Equal(t, true, valid)

			got := gm.GpuDataMap["0"]
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
	t.Run("calculates averages and resets accumulators", func(t *testing.T) {
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
				"2": {
					Name:        "GPU 2",
					Temperature: 70,
					MemoryUsed:  4096,
					MemoryTotal: 8192,
					Usage:       200,
					Power:       400,
					Count:       1,
				},
			},
		}

		result := gm.GetCurrentData()

		// Verify name disambiguation
		assert.Equal(t, "GPU1 0", result["0"].Name)
		assert.Equal(t, "GPU1 1", result["1"].Name)
		assert.Equal(t, "GPU 2", result["2"].Name)

		// Check averaged values in the result
		assert.InDelta(t, 50.0, result["0"].Usage, 0.01)
		assert.InDelta(t, 100.0, result["0"].Power, 0.01)
		assert.InDelta(t, 30.0, result["1"].Usage, 0.01)
		assert.InDelta(t, 60.0, result["1"].Power, 0.01)

		// Verify that accumulators in the original map are reset
		assert.Equal(t, float64(0), gm.GpuDataMap["0"].Count, "GPU 0 Count should be reset")
		assert.Equal(t, float64(0), gm.GpuDataMap["0"].Usage, "GPU 0 Usage should be reset")
		assert.Equal(t, float64(0), gm.GpuDataMap["0"].Power, "GPU 0 Power should be reset")
		assert.Equal(t, float64(0), gm.GpuDataMap["1"].Count, "GPU 1 Count should be reset")
		assert.Equal(t, float64(0), gm.GpuDataMap["1"].Usage, "GPU 1 Usage should be reset")
		assert.Equal(t, float64(0), gm.GpuDataMap["1"].Power, "GPU 1 Power should be reset")
	})

	t.Run("handles zero count without panicking", func(t *testing.T) {
		gm := &GPUManager{
			GpuDataMap: map[string]*system.GPUData{
				"0": {
					Name:  "TestGPU",
					Count: 0,
					Usage: 0,
					Power: 0,
				},
			},
		}

		var result map[string]system.GPUData
		assert.NotPanics(t, func() {
			result = gm.GetCurrentData()
		})

		// Check that usage and power are 0
		assert.Equal(t, 0.0, result["0"].Usage)
		assert.Equal(t, 0.0, result["0"].Power)

		// Verify reset count
		assert.Equal(t, float64(0), gm.GpuDataMap["0"].Count)
	})
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
			wantNvidiaSmi:  false,
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
echo "11-14-2024 22:54:33 RAM 1024/4096MB GR3D_FREQ 80% tj@70C VDD_GPU_SOC 1000mW"`
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

// TestAccumulationTableDriven tests the accumulation behavior for all three GPU types
func TestAccumulation(t *testing.T) {
	type expectedGPUValues struct {
		temperature float64
		memoryUsed  float64
		memoryTotal float64
		usage       float64
		power       float64
		count       float64
		avgUsage    float64
		avgPower    float64
	}

	tests := []struct {
		name           string
		initialGPUData map[string]*system.GPUData
		dataSamples    [][]byte
		parser         func(*GPUManager) func([]byte) bool
		expectedValues map[string]expectedGPUValues
	}{
		{
			name: "Jetson GPU accumulation",
			initialGPUData: map[string]*system.GPUData{
				"0": {
					Name:        "Jetson",
					Temperature: 0,
					Usage:       0,
					Power:       0,
					Count:       0,
				},
			},
			dataSamples: [][]byte{
				[]byte("11-14-2024 22:54:33 RAM 1024/4096MB GR3D_FREQ 30% tj@50.5C VDD_GPU_SOC 1000mW"),
				[]byte("11-14-2024 22:54:33 RAM 1024/4096MB GR3D_FREQ 40% tj@60.5C VDD_GPU_SOC 1200mW"),
				[]byte("11-14-2024 22:54:33 RAM 1024/4096MB GR3D_FREQ 50% tj@70.5C VDD_GPU_SOC 1400mW"),
			},
			parser: func(gm *GPUManager) func([]byte) bool {
				return gm.getJetsonParser()
			},
			expectedValues: map[string]expectedGPUValues{
				"0": {
					temperature: 70.5,  // Last value
					memoryUsed:  1024,  // Last value
					memoryTotal: 4096,  // Last value
					usage:       120.0, // Accumulated: 30 + 40 + 50
					power:       3.6,   // Accumulated: 1.0 + 1.2 + 1.4
					count:       3,
					avgUsage:    40.0, // 120 / 3
					avgPower:    1.2,  // 3.6 / 3
				},
			},
		},
		{
			name:           "NVIDIA GPU accumulation",
			initialGPUData: map[string]*system.GPUData{
				// NVIDIA parser will create the GPU data entries
			},
			dataSamples: [][]byte{
				[]byte("0, NVIDIA GeForce RTX 3080, 50, 5000, 10000, 30, 200"),
				[]byte("0, NVIDIA GeForce RTX 3080, 60, 6000, 10000, 40, 250"),
				[]byte("0, NVIDIA GeForce RTX 3080, 70, 7000, 10000, 50, 300"),
			},
			parser: func(gm *GPUManager) func([]byte) bool {
				return gm.parseNvidiaData
			},
			expectedValues: map[string]expectedGPUValues{
				"0": {
					temperature: 70.0,            // Last value
					memoryUsed:  7000.0 / 1.024,  // Last value
					memoryTotal: 10000.0 / 1.024, // Last value
					usage:       120.0,           // Accumulated: 30 + 40 + 50
					power:       750.0,           // Accumulated: 200 + 250 + 300
					count:       3,
					avgUsage:    40.0,  // 120 / 3
					avgPower:    250.0, // 750 / 3
				},
			},
		},
		{
			name:           "AMD GPU accumulation",
			initialGPUData: map[string]*system.GPUData{
				// AMD parser will create the GPU data entries
			},
			dataSamples: [][]byte{
				[]byte(`{"card0": {"GUID": "34756", "Temperature (Sensor edge) (C)": "50.0", "Current Socket Graphics Package Power (W)": "100.0", "GPU use (%)": "30", "VRAM Total Memory (B)": "10737418240", "VRAM Total Used Memory (B)": "1073741824", "Card Series": "Radeon RX 6800"}}`),
				[]byte(`{"card0": {"GUID": "34756", "Temperature (Sensor edge) (C)": "60.0", "Current Socket Graphics Package Power (W)": "150.0", "GPU use (%)": "40", "VRAM Total Memory (B)": "10737418240", "VRAM Total Used Memory (B)": "2147483648", "Card Series": "Radeon RX 6800"}}`),
				[]byte(`{"card0": {"GUID": "34756", "Temperature (Sensor edge) (C)": "70.0", "Current Socket Graphics Package Power (W)": "200.0", "GPU use (%)": "50", "VRAM Total Memory (B)": "10737418240", "VRAM Total Used Memory (B)": "3221225472", "Card Series": "Radeon RX 6800"}}`),
			},
			parser: func(gm *GPUManager) func([]byte) bool {
				return gm.parseAmdData
			},
			expectedValues: map[string]expectedGPUValues{
				"34756": {
					temperature: 70.0,                          // Last value
					memoryUsed:  3221225472.0 / (1024 * 1024),  // Last value
					memoryTotal: 10737418240.0 / (1024 * 1024), // Last value
					usage:       120.0,                         // Accumulated: 30 + 40 + 50
					power:       450.0,                         // Accumulated: 100 + 150 + 200
					count:       3,
					avgUsage:    40.0,  // 120 / 3
					avgPower:    150.0, // 450 / 3
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new GPUManager for each test
			gm := &GPUManager{
				GpuDataMap: tt.initialGPUData,
			}

			// Get the parser function
			parser := tt.parser(gm)

			// Process each data sample
			for i, sample := range tt.dataSamples {
				valid := parser(sample)
				assert.True(t, valid, "Sample %d should be valid", i)
			}

			// Check accumulated values
			for id, expected := range tt.expectedValues {
				gpu, exists := gm.GpuDataMap[id]
				assert.True(t, exists, "GPU with ID %s should exist", id)
				if !exists {
					continue
				}

				assert.InDelta(t, expected.temperature, gpu.Temperature, 0.01, "Temperature should match")
				assert.InDelta(t, expected.memoryUsed, gpu.MemoryUsed, 0.01, "Memory used should match")
				assert.InDelta(t, expected.memoryTotal, gpu.MemoryTotal, 0.01, "Memory total should match")
				assert.InDelta(t, expected.usage, gpu.Usage, 0.01, "Usage should match")
				assert.InDelta(t, expected.power, gpu.Power, 0.01, "Power should match")
				assert.Equal(t, expected.count, gpu.Count, "Count should match")
			}

			// Verify average calculation in GetCurrentData
			result := gm.GetCurrentData()
			for id, expected := range tt.expectedValues {
				gpu, exists := result[id]
				assert.True(t, exists, "GPU with ID %s should exist in GetCurrentData result", id)
				if !exists {
					continue
				}

				assert.InDelta(t, expected.temperature, gpu.Temperature, 0.01, "Temperature in GetCurrentData should match")
				assert.InDelta(t, expected.avgUsage, gpu.Usage, 0.01, "Average usage in GetCurrentData should match")
				assert.InDelta(t, expected.avgPower, gpu.Power, 0.01, "Average power in GetCurrentData should match")
			}

			// Verify that accumulators in the original map are reset
			for id := range tt.expectedValues {
				gpu, exists := gm.GpuDataMap[id]
				assert.True(t, exists, "GPU with ID %s should still exist after GetCurrentData", id)
				if !exists {
					continue
				}
				assert.Equal(t, float64(0), gpu.Count, "Count should be reset for GPU ID %s", id)
				assert.Equal(t, float64(0), gpu.Usage, "Usage should be reset for GPU ID %s", id)
				assert.Equal(t, float64(0), gpu.Power, "Power should be reset for GPU ID %s", id)
			}
		})
	}
}

func TestIntelUpdateFromStats(t *testing.T) {
	gm := &GPUManager{
		GpuDataMap: make(map[string]*system.GPUData),
	}

	// First sample with power and two engines
	sample1 := intelGpuStats{
		Engines: map[string]struct {
			Busy float64 `json:"busy"`
		}{
			"Render/3D": {Busy: 20.0},
			"Video":     {Busy: 5.0},
		},
	}
	sample1.Power.GPU = 10.5

	ok := gm.updateIntelFromStats(&sample1)
	assert.True(t, ok)

	gpu := gm.GpuDataMap["0"]
	require.NotNil(t, gpu)
	assert.Equal(t, "GPU", gpu.Name)
	assert.InDelta(t, 10.5, gpu.Power, 0.001)
	assert.InDelta(t, 20.0, gpu.Engines["Render/3D"], 0.001)
	assert.InDelta(t, 5.0, gpu.Engines["Video"], 0.001)
	assert.Equal(t, float64(1), gpu.Count)

	// Second sample with zero power (should not add) and additional engine busy
	sample2 := intelGpuStats{
		Engines: map[string]struct {
			Busy float64 `json:"busy"`
		}{
			"Render/3D": {Busy: 10.0},
			"Video":     {Busy: 2.5},
			"Blitter":   {Busy: 1.0},
		},
	}
	// zero power should not increment power accumulator
	sample2.Power.GPU = 0.0

	ok = gm.updateIntelFromStats(&sample2)
	assert.True(t, ok)

	gpu = gm.GpuDataMap["0"]
	require.NotNil(t, gpu)
	assert.InDelta(t, 10.5, gpu.Power, 0.001)
	assert.InDelta(t, 30.0, gpu.Engines["Render/3D"], 0.001) // 20 + 10
	assert.InDelta(t, 7.5, gpu.Engines["Video"], 0.001)      // 5 + 2.5
	assert.InDelta(t, 1.0, gpu.Engines["Blitter"], 0.001)
	assert.Equal(t, float64(2), gpu.Count)
}

func TestIntelCollectorStreaming(t *testing.T) {
	// Save and override PATH
	origPath := os.Getenv("PATH")
	defer os.Setenv("PATH", origPath)

	dir := t.TempDir()
	os.Setenv("PATH", dir)

	// Create a fake intel_gpu_top that prints a JSON array with two samples and exits
	scriptPath := filepath.Join(dir, "intel_gpu_top")
	script := `#!/bin/sh
# Ignore args -s and -J
# Emit a JSON array with two objects, separated by a comma, then exit
(echo '['; \
 echo '{"power":{"GPU":1.5},"engines":{"Render/3D":{"busy":12.34}}},'; \
 echo '{"power":{"GPU":2.0},"engines":{"Video":{"busy":5}}}'; \
 echo ']')`
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	gm := &GPUManager{
		GpuDataMap: make(map[string]*system.GPUData),
	}

	// Run the collector once; it should read two samples and return
	if err := gm.collectIntelStats(); err != nil {
		t.Fatalf("collectIntelStats error: %v", err)
	}

	gpu := gm.GpuDataMap["0"]
	require.NotNil(t, gpu)
	// Power should be sum of non-zero samples: 1.5 + 2.0 = 3.5
	assert.InDelta(t, 3.5, gpu.Power, 0.001)
	// Engines aggregated
	assert.InDelta(t, 12.34, gpu.Engines["Render/3D"], 0.001)
	assert.InDelta(t, 5.0, gpu.Engines["Video"], 0.001)
	// Count should be 2 samples
	assert.Equal(t, float64(2), gpu.Count)
}
