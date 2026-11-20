package records

import (
	"github.com/henrygd/beszel/internal/entities/system"
	"maps"
)

// mergeChartMax owns its maps/slices and never modifies input records. Signed
// sensor values use presence checks, so missing samples cannot introduce zero peaks.
func mergeChartMax(dst *system.ChartMax, src *system.ChartMax) {
	if len(src.Temperatures) > 0 && dst.Temperatures == nil {
		dst.Temperatures = make(map[string]float64)
	}
	for k, v := range src.Temperatures {
		if prev, ok := dst.Temperatures[k]; !ok || v > prev {
			dst.Temperatures[k] = v
		}
	}
	if len(src.Fans) > 0 && dst.Fans == nil {
		dst.Fans = make(map[string]uint16)
	}
	for k, v := range src.Fans {
		if prev, ok := dst.Fans[k]; !ok || v > prev {
			dst.Fans[k] = v
		}
	}
	for i, v := range src.LoadAvg {
		dst.LoadAvg[i] = max(dst.LoadAvg[i], v)
	}
	if len(dst.CpuBreakdown) < len(src.CpuBreakdown) {
		dst.CpuBreakdown = append(dst.CpuBreakdown, make([]float64, len(src.CpuBreakdown)-len(dst.CpuBreakdown))...)
	}
	for i, v := range src.CpuBreakdown {
		dst.CpuBreakdown[i] = max(dst.CpuBreakdown[i], v)
	}
	if len(dst.CpuCoresUsage) < len(src.CpuCoresUsage) {
		dst.CpuCoresUsage = append(dst.CpuCoresUsage, make(system.Uint8Slice, len(src.CpuCoresUsage)-len(dst.CpuCoresUsage))...)
	}
	for i, v := range src.CpuCoresUsage {
		dst.CpuCoresUsage[i] = max(dst.CpuCoresUsage[i], v)
	}
	for i, v := range src.MemBytes {
		dst.MemBytes[i] = max(dst.MemBytes[i], v)
	}
	for i, v := range src.SwapBytes {
		dst.SwapBytes[i] = max(dst.SwapBytes[i], v)
	}
	dst.DiskUsed = max(dst.DiskUsed, src.DiskUsed)
	dst.Battery[0] = max(dst.Battery[0], src.Battery[0])
	dst.Battery[1] = src.Battery[1]
	if len(src.Batteries) > 0 && dst.Batteries == nil {
		dst.Batteries = make(map[string]uint8)
	}
	for k, v := range src.Batteries {
		if prev, ok := dst.Batteries[k]; !ok || v > prev {
			dst.Batteries[k] = v
		}
	}
	if len(src.WiFi) > 0 && dst.WiFi == nil {
		dst.WiFi = make(map[string]int8)
	}
	for k, v := range src.WiFi {
		if prev, ok := dst.WiFi[k]; !ok || v > prev {
			dst.WiFi[k] = v
		}
	}
	if len(src.NetworkInterfaces) > 0 && dst.NetworkInterfaces == nil {
		dst.NetworkInterfaces = make(map[string][4]uint64)
	}
	for k, v := range src.NetworkInterfaces {
		prev := dst.NetworkInterfaces[k]
		for i := range v {
			v[i] = max(v[i], prev[i])
		}
		dst.NetworkInterfaces[k] = v
	}
	if len(src.ExtraFs) > 0 && dst.ExtraFs == nil {
		dst.ExtraFs = make(map[string]*system.FsStats)
	}
	for k, v := range src.ExtraFs {
		if v == nil {
			continue
		}
		next := *v
		if prev := dst.ExtraFs[k]; prev != nil {
			next.DiskUsed = max(prev.DiskUsed, v.DiskUsed)
		}
		dst.ExtraFs[k] = &next
	}
	if len(src.GPUData) > 0 && dst.GPUData == nil {
		dst.GPUData = make(map[string]system.GPUData)
	}
	for k, v := range src.GPUData {
		prev := dst.GPUData[k]
		v.Usage = max(v.Usage, prev.Usage)
		v.MemoryUsed = max(v.MemoryUsed, prev.MemoryUsed)
		v.Power = max(v.Power, prev.Power)
		v.PowerPkg = max(v.PowerPkg, prev.PowerPkg)
		engines := maps.Clone(prev.Engines)
		if len(v.Engines) > 0 && engines == nil {
			engines = make(map[string]float64)
		}
		for name, val := range v.Engines {
			engines[name] = max(engines[name], val)
		}
		v.Engines = engines
		dst.GPUData[k] = v
	}
	if len(src.ZfsPools) > 0 && dst.ZfsPools == nil {
		dst.ZfsPools = make(map[string]*system.ZfsPool)
	}
	for k, v := range src.ZfsPools {
		if v == nil {
			continue
		}
		next := *v
		if prev := dst.ZfsPools[k]; prev != nil {
			if prev.Raw == v.Raw {
				next.Used = max(prev.Used, v.Used)
			}
			next.ReadBytes = max(prev.ReadBytes, v.ReadBytes)
			next.WriteBytes = max(prev.WriteBytes, v.WriteBytes)
		}
		dst.ZfsPools[k] = &next
	}
}

func chartMaxSample(s *system.Stats) *system.ChartMax {
	mem, swap, _ := memoryBytes(s)
	sample := &system.ChartMax{
		Temperatures:  s.Temperatures,
		Fans:          s.Fans,
		LoadAvg:       s.LoadAvg,
		CpuBreakdown:  s.CpuBreakdown,
		CpuCoresUsage: s.CpuCoresUsage,
		MemBytes:      mem,
		SwapBytes:     swap,
		DiskUsed:      s.DiskUsed,
		Battery:       s.Battery,
		Batteries:     s.Batteries,
		WiFi:          s.WiFi,
	}

	if len(s.ExtraFs) > 0 {
		sample.ExtraFs = make(map[string]*system.FsStats, len(s.ExtraFs))
		for k, v := range s.ExtraFs {
			if v != nil {
				sample.ExtraFs[k] = &system.FsStats{DiskUsed: v.DiskUsed}
			}
		}
	}
	if len(s.GPUData) > 0 {
		sample.GPUData = make(map[string]system.GPUData, len(s.GPUData))
		for k, v := range s.GPUData {
			sample.GPUData[k] = system.GPUData{Usage: v.Usage, MemoryUsed: v.MemoryUsed, Power: v.Power, PowerPkg: v.PowerPkg, Engines: v.Engines}
		}
	}
	if len(s.ZfsPools) > 0 {
		sample.ZfsPools = make(map[string]*system.ZfsPool, len(s.ZfsPools))
		for k, v := range s.ZfsPools {
			if v != nil {
				sample.ZfsPools[k] = &system.ZfsPool{Used: v.Used, ReadBytes: v.ReadBytes, WriteBytes: v.WriteBytes, Raw: v.Raw}
			}
		}
	}
	if len(s.NetworkInterfaces) > 0 {
		sample.NetworkInterfaces = make(map[string][4]uint64, len(s.NetworkInterfaces))
		for k, v := range s.NetworkInterfaces {
			sample.NetworkInterfaces[k] = [4]uint64{v[0], v[1]}
		}
	}
	return sample
}
