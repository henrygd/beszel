package agent

import (
	"fmt"
	"strings"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/system"
)

const nvidiaVendorID = 0x10de

// pdhAdapter is a hardware GPU adapter reported by DXGI on Windows.
type pdhAdapter struct {
	id          string
	vendorID    uint32
	deviceID    uint32
	luid        string // "0x<high>_0x<low>" in lowercase, as in performance counter instance names
	name        string
	memoryTotal uint64 // dedicated video memory in bytes
}

// assignPdhAdapterIDs gives each adapter an ID from its PCI vendor and device IDs.
// Unlike the LUID or the DXGI order, these stay the same across reboots, driver
// updates and adapters coming and going. The "p" prefix keeps them apart from the
// IDs of other collectors.
func assignPdhAdapterIDs(adapters []pdhAdapter) {
	seen := make(map[string]int, len(adapters))
	for i := range adapters {
		id := fmt.Sprintf("p%04x-%04x", adapters[i].vendorID, adapters[i].deviceID)
		seen[id]++
		if seen[id] > 1 {
			id = fmt.Sprintf("%s-%d", id, seen[id])
		}
		adapters[i].id = id
	}
}

// pdhSample holds one reading of the Windows GPU performance counters, keyed by counter instance name.
type pdhSample struct {
	engines map[string]float64 // GPU Engine(*)\Utilization Percentage
	memory  map[string]float64 // GPU Adapter Memory(*)\Dedicated Usage, in bytes
}

// parsePdhEngineInstance splits a GPU Engine counter instance name such as
// "pid_1234_luid_0x00000000_0x0000AB6D_phys_0_eng_0_engtype_3D" into the adapter LUID,
// the engine ("0_eng_0", unique within the adapter) and the engine type ("3D").
func parsePdhEngineInstance(instance string) (luid, engine, engType string, ok bool) {
	_, rest, ok := strings.Cut(instance, "_luid_")
	if !ok {
		return "", "", "", false
	}
	rest, engType, ok = strings.Cut(rest, "_engtype_")
	if !ok || engType == "" {
		return "", "", "", false
	}
	luid, engine, ok = strings.Cut(rest, "_phys_")
	if !ok {
		return "", "", "", false
	}
	return strings.ToLower(luid), engine, engType, true
}

// parsePdhMemoryInstance returns the adapter LUID from a GPU Adapter Memory counter
// instance name such as "luid_0x00000000_0x0000AB6D_phys_0".
func parsePdhMemoryInstance(instance string) (luid string, ok bool) {
	rest, ok := strings.CutPrefix(instance, "luid_")
	if !ok {
		return "", false
	}
	luid, _, ok = strings.Cut(rest, "_phys_")
	return strings.ToLower(luid), ok
}

// pdhEngineUsage returns the utilization of each engine type per adapter LUID.
// Windows reports utilization per process and engine, so it sums the processes on
// each engine and, like Task Manager, reports the busiest engine of each type.
func pdhEngineUsage(engines map[string]float64) map[string]map[string]float64 {
	type engineUsage struct {
		engType string
		usage   float64
	}
	perEngine := make(map[string]map[string]*engineUsage)
	for instance, value := range engines {
		luid, engine, engType, ok := parsePdhEngineInstance(instance)
		if !ok {
			continue
		}
		if perEngine[luid] == nil {
			perEngine[luid] = make(map[string]*engineUsage)
		}
		e := perEngine[luid][engine]
		if e == nil {
			e = &engineUsage{engType: engType}
			perEngine[luid][engine] = e
		}
		e.usage += value
	}

	usage := make(map[string]map[string]float64, len(perEngine))
	for luid, engines := range perEngine {
		usage[luid] = make(map[string]float64)
		for _, e := range engines {
			// process samples can overlap and add up to more than 100
			usage[luid][e.engType] = max(usage[luid][e.engType], min(e.usage, 100))
		}
	}
	return usage
}

// pdhEngineCountsTowardUsage reports whether an engine type does 3D or compute work.
// Like nvidia-smi and amdgpu, usage leaves out the video and copy engines, so screen
// capture or video playback alone doesn't make an idle GPU look busy.
func pdhEngineCountsTowardUsage(engType string) bool {
	engType = strings.ToLower(engType)
	for _, kind := range []string{"3d", "compute", "graphics", "cuda"} {
		if strings.Contains(engType, kind) {
			return true
		}
	}
	return false
}

// updatePdhGpuData adds one counter sample to the GPU data map and drops GPUs
// that are no longer listed.
func (gm *GPUManager) updatePdhGpuData(adapters []pdhAdapter, sample pdhSample) {
	engineUsage := pdhEngineUsage(sample.engines)
	memoryUsed := make(map[string]float64, len(sample.memory))
	for instance, value := range sample.memory {
		if luid, ok := parsePdhMemoryInstance(instance); ok {
			memoryUsed[luid] += value
		}
	}

	gm.Lock()
	defer gm.Unlock()

	if gm.ownUsage == nil {
		gm.ownUsage = make(map[string]bool)
	}
	listed := make(map[string]bool, len(adapters))
	for _, adapter := range adapters {
		// the NVIDIA collector already reports these cards
		if gm.nvidiaCollectorStarted && adapter.vendorID == nvidiaVendorID {
			continue
		}
		listed[adapter.id] = true
		gpu, ok := gm.GpuDataMap[adapter.id]
		if !ok {
			gpu = &system.GPUData{}
			gm.GpuDataMap[adapter.id] = gpu
		}
		gpu.Name = adapter.name
		if gpu.Engines == nil {
			gpu.Engines = make(map[string]float64)
		}
		var usage float64
		for engType, engUsage := range engineUsage[adapter.luid] {
			gpu.Engines[engType] += engUsage
			if pdhEngineCountsTowardUsage(engType) {
				usage = max(usage, engUsage)
			}
		}
		gpu.Usage += usage
		gm.ownUsage[adapter.id] = true
		gpu.MemoryUsed = utils.BytesToMegabytes(memoryUsed[adapter.luid])
		gpu.MemoryTotal = utils.BytesToMegabytes(float64(adapter.memoryTotal))
		gpu.Count++
	}
	gm.removePdhGpus(listed)
}

// dropPdhGpus removes every pdh GPU, so a GPU whose counters can't be read
// drops out of the data instead of repeating its last average.
func (gm *GPUManager) dropPdhGpus() {
	gm.Lock()
	defer gm.Unlock()
	gm.removePdhGpus(nil)
}

// removePdhGpus removes the pdh GPUs not in keep, with their averaging state,
// so an adapter that comes back starts fresh. pdh is the only collector that
// sets ownUsage, so ownUsage lists its GPUs. The caller must hold the lock.
func (gm *GPUManager) removePdhGpus(keep map[string]bool) {
	for id := range gm.ownUsage {
		if keep[id] {
			continue
		}
		delete(gm.GpuDataMap, id)
		delete(gm.ownUsage, id)
		delete(gm.lastAvgData, id)
		for _, snapshots := range gm.lastSnapshots {
			delete(snapshots, id)
		}
	}
}
