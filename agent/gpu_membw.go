package agent

import "github.com/henrygd/beszel/agent/utils"

// GPU memory bandwidth helpers shared by the NVML collector. Kept free of build
// tags so the formulas are unit-tested on every platform.

// nvmlPeakMemBandwidthGBps returns theoretical peak device memory bandwidth in
// GB/s (1e9 bytes) from NVML's max memory clock (MHz, NVML_CLOCK_MEM) and the
// memory bus width in bits.
//
// NVML reports the memory clock at half the per-pin data rate for every memory
// type it supports: GDDR6X 19.5 Gbps reads as 9751 MHz, GDDR7 28 Gbps as
// 14001 MHz, and HBM2e/HBM3 report their I/O clock (A100 80GB 1593 MHz, H100 SXM
// 2619 MHz), so bytes/s = clock * 2 transfers * bus width / 8 in all cases.
// This matches nvidia-smi "Max Clocks: Memory" against published specs (see
// TestNvmlPeakMemBandwidth).
func nvmlPeakMemBandwidthGBps(maxMemClockMHz, busWidthBits uint32) float64 {
	if maxMemClockMHz == 0 || busWidthBits == 0 {
		return 0
	}
	bytesPerSecond := float64(maxMemClockMHz) * 1e6 * 2 * float64(busWidthBits) / 8
	return utils.TwoDecimals(bytesPerSecond / 1e9)
}

// gpuMemBandwidthFromUtil converts a 0-100 utilization percentage of peak
// bandwidth into GB/s. Out-of-range input is clamped.
func gpuMemBandwidthFromUtil(utilPct, peakGBps float64) float64 {
	if peakGBps <= 0 || utilPct <= 0 {
		return 0
	}
	return min(utilPct, 100) / 100 * peakGBps
}
