//go:build windows && (amd64 || arm64)

package agent

import (
	"fmt"
	"log/slog"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	pdhPollInterval  = 3 * time.Second
	pdhEngineCounter = `\GPU Engine(*)\Utilization Percentage`
	pdhMemoryCounter = `\GPU Adapter Memory(*)\Dedicated Usage`

	pdhFmtDouble      = 0x00000200
	pdhMoreData       = 0x800007D2
	pdhNoData         = 0x800007D5
	pdhCstatusNewData = 1 // statuses above this mark an invalid value

	dxgiErrorNotFound       = 0x887A0002
	dxgiAdapterFlagSoftware = 2
	microsoftVendorID       = 0x1414 // Basic Render Driver and remote display adapters

	// vtable indexes of the COM methods we call
	comRelease          = 2
	dxgiEnumAdapters1   = 12
	dxgiAdapterGetDesc1 = 10
)

var (
	modPdh                           = windows.NewLazySystemDLL("pdh.dll")
	procPdhOpenQueryW                = modPdh.NewProc("PdhOpenQueryW")
	procPdhAddEnglishCounterW        = modPdh.NewProc("PdhAddEnglishCounterW")
	procPdhCollectQueryData          = modPdh.NewProc("PdhCollectQueryData")
	procPdhGetFormattedCounterArrayW = modPdh.NewProc("PdhGetFormattedCounterArrayW")
	procPdhCloseQuery                = modPdh.NewProc("PdhCloseQuery")

	modDxgi                = windows.NewLazySystemDLL("dxgi.dll")
	procCreateDXGIFactory1 = modDxgi.NewProc("CreateDXGIFactory1")

	iidIDXGIFactory1 = windows.GUID{Data1: 0x770aae78, Data2: 0xf26f, Data3: 0x4dba, Data4: [8]byte{0xa8, 0x29, 0x25, 0x3c, 0x83, 0xd1, 0xb3, 0x87}}
)

// pdhCounterItem is PDH_FMT_COUNTERVALUE_ITEM_W read with PDH_FMT_DOUBLE.
type pdhCounterItem struct {
	name   uintptr // *uint16 into the same buffer
	status uint32
	_      uint32
	value  float64
}

// dxgiAdapterDesc1 is DXGI_ADAPTER_DESC1.
type dxgiAdapterDesc1 struct {
	Description           [128]uint16
	VendorID              uint32
	DeviceID              uint32
	SubSysID              uint32
	Revision              uint32
	DedicatedVideoMemory  uintptr
	DedicatedSystemMemory uintptr
	SharedSystemMemory    uintptr
	AdapterLuid           windows.LUID
	Flags                 uint32
}

// The structs must match the Windows SDK layouts on 64-bit Windows.
var (
	_ [24]byte  = [unsafe.Sizeof(pdhCounterItem{})]byte{}
	_ [16]byte  = [unsafe.Offsetof(pdhCounterItem{}.value)]byte{}
	_ [312]byte = [unsafe.Sizeof(dxgiAdapterDesc1{})]byte{}
	_ [296]byte = [unsafe.Offsetof(dxgiAdapterDesc1{}.AdapterLuid)]byte{}
)

// pdhGpuQuery is an open PDH query for the GPU engine and memory counters.
type pdhGpuQuery struct {
	query   uintptr
	engines uintptr
	memory  uintptr
}

// hasPdhGpu reports whether DXGI lists a hardware GPU adapter. Virtual machine
// display adapters (VMware, VirtualBox) and the basic graphics of server management
// controllers count too, so those hosts also get GPU data, which stays near zero.
func (gm *GPUManager) hasPdhGpu() bool {
	adapters, err := listDxgiAdapters()
	if err != nil {
		slog.Debug("Failed to list GPU adapters", "err", err)
		return false
	}
	return len(adapters) > 0
}

// startPdhCollector starts GPU collection from Windows performance counters.
func (gm *GPUManager) startPdhCollector() bool {
	query, err := newPdhGpuQuery()
	if err != nil {
		slog.Warn("Failed to open GPU performance counters", "err", err)
		return false
	}
	go gm.collectPdhStats(query)
	return true
}

func (gm *GPUManager) collectPdhStats(query *pdhGpuQuery) {
	slog.Debug("Using Windows performance counters for GPU data collection")
	failing := false
	for {
		time.Sleep(pdhPollInterval)
		if err := gm.readPdhGpuStats(query); err != nil {
			gm.dropPdhGpus()
			if !failing {
				slog.Warn("Error collecting GPU data from performance counters", "err", err)
			}
			failing = true
			continue
		}
		if failing {
			slog.Info("Collecting GPU data from performance counters again")
		}
		failing = false
	}
}

func (gm *GPUManager) readPdhGpuStats(query *pdhGpuQuery) error {
	// an adapter gets a new LUID when its driver restarts, so list them on every read
	adapters, err := listDxgiAdapters()
	if err != nil {
		return err
	}
	if err := query.collect(); err != nil {
		return err
	}
	engines, err := query.values(query.engines)
	if err != nil {
		return err
	}
	memory, err := query.values(query.memory)
	if err != nil {
		return err
	}
	gm.updatePdhGpuData(adapters, pdhSample{engines: engines, memory: memory})
	return nil
}

func newPdhGpuQuery() (*pdhGpuQuery, error) {
	if err := findProcs(procPdhOpenQueryW, procPdhAddEnglishCounterW, procPdhCollectQueryData, procPdhGetFormattedCounterArrayW, procPdhCloseQuery); err != nil {
		return nil, err
	}
	q := &pdhGpuQuery{}
	if ret, _, _ := procPdhOpenQueryW.Call(0, 0, uintptr(unsafe.Pointer(&q.query))); ret != 0 {
		return nil, pdhError("PdhOpenQuery", ret)
	}
	if err := q.addCounter(pdhEngineCounter, &q.engines); err != nil {
		q.close()
		return nil, err
	}
	if err := q.addCounter(pdhMemoryCounter, &q.memory); err != nil {
		q.close()
		return nil, err
	}
	// utilization is a rate, so it needs a first sample to compare the next one against
	if err := q.collect(); err != nil {
		q.close()
		return nil, err
	}
	return q, nil
}

func (q *pdhGpuQuery) addCounter(path string, counter *uintptr) error {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	if ret, _, _ := procPdhAddEnglishCounterW.Call(q.query, uintptr(unsafe.Pointer(pathPtr)), 0, uintptr(unsafe.Pointer(counter))); ret != 0 {
		return pdhError("PdhAddEnglishCounter "+path, ret)
	}
	return nil
}

func (q *pdhGpuQuery) collect() error {
	if ret, _, _ := procPdhCollectQueryData.Call(q.query); ret != 0 {
		return pdhError("PdhCollectQueryData", ret)
	}
	return nil
}

// values returns the counter's value for each instance, skipping invalid values.
func (q *pdhGpuQuery) values(counter uintptr) (map[string]float64, error) {
	var size, count uint32
	ret, _, _ := procPdhGetFormattedCounterArrayW.Call(counter, pdhFmtDouble, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
	switch ret {
	case pdhMoreData:
	case 0, pdhNoData:
		return map[string]float64{}, nil
	default:
		return nil, pdhError("PdhGetFormattedCounterArray", ret)
	}

	// The buffer holds the items followed by their names. A byte slice keeps the
	// garbage collector from treating the name data as Go pointers.
	buf := make([]byte, size)
	ret, _, _ = procPdhGetFormattedCounterArrayW.Call(counter, pdhFmtDouble, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf[0])))
	if ret != 0 {
		return nil, pdhError("PdhGetFormattedCounterArray", ret)
	}
	if uintptr(count)*unsafe.Sizeof(pdhCounterItem{}) > uintptr(len(buf)) {
		return nil, fmt.Errorf("PdhGetFormattedCounterArray returned %d items for %d bytes", count, len(buf))
	}

	base := unsafe.Pointer(&buf[0])
	items := unsafe.Slice((*pdhCounterItem)(base), count)
	values := make(map[string]float64, count)
	for _, item := range items {
		offset := item.name - uintptr(base)
		if item.status > pdhCstatusNewData || offset >= uintptr(len(buf)) {
			continue
		}
		values[windows.UTF16PtrToString((*uint16)(unsafe.Add(base, offset)))] += item.value
	}
	return values, nil
}

func (q *pdhGpuQuery) close() {
	procPdhCloseQuery.Call(q.query)
}

// listDxgiAdapters returns the hardware GPU adapters in DXGI order.
// An adapter that can't be described, such as one being removed, is skipped.
func listDxgiAdapters() ([]pdhAdapter, error) {
	if err := findProcs(procCreateDXGIFactory1); err != nil {
		return nil, err
	}
	var factory unsafe.Pointer
	if hr, _, _ := procCreateDXGIFactory1.Call(uintptr(unsafe.Pointer(&iidIDXGIFactory1)), uintptr(unsafe.Pointer(&factory))); failedHresult(hr) {
		return nil, fmt.Errorf("CreateDXGIFactory1 failed: 0x%08x", uint32(hr))
	}
	defer syscall.SyscallN(comMethod(factory, comRelease), uintptr(factory))

	var adapters []pdhAdapter
	for i := uintptr(0); ; i++ {
		var adapter unsafe.Pointer
		hr, _, _ := syscall.SyscallN(comMethod(factory, dxgiEnumAdapters1), uintptr(factory), i, uintptr(unsafe.Pointer(&adapter)))
		if uint32(hr) == dxgiErrorNotFound {
			assignPdhAdapterIDs(adapters)
			return adapters, nil
		}
		if failedHresult(hr) {
			return nil, fmt.Errorf("IDXGIFactory1::EnumAdapters1 failed: 0x%08x", uint32(hr))
		}

		var desc dxgiAdapterDesc1
		hr, _, _ = syscall.SyscallN(comMethod(adapter, dxgiAdapterGetDesc1), uintptr(adapter), uintptr(unsafe.Pointer(&desc)))
		syscall.SyscallN(comMethod(adapter, comRelease), uintptr(adapter))
		if failedHresult(hr) {
			slog.Debug("Skipping GPU adapter", "index", i, "err", fmt.Sprintf("IDXGIAdapter1::GetDesc1 failed: 0x%08x", uint32(hr)))
			continue
		}
		if desc.Flags&dxgiAdapterFlagSoftware != 0 || desc.VendorID == microsoftVendorID {
			continue
		}
		adapters = append(adapters, pdhAdapter{
			vendorID:    desc.VendorID,
			deviceID:    desc.DeviceID,
			luid:        fmt.Sprintf("0x%08x_0x%08x", uint32(desc.AdapterLuid.HighPart), desc.AdapterLuid.LowPart),
			name:        windows.UTF16ToString(desc.Description[:]),
			memoryTotal: uint64(desc.DedicatedVideoMemory),
		})
	}
}

// comMethod returns the address of a COM object's method from its vtable.
func comMethod(obj unsafe.Pointer, index uintptr) uintptr {
	vtable := *(*unsafe.Pointer)(obj)
	return *(*uintptr)(unsafe.Add(vtable, index*unsafe.Sizeof(uintptr(0))))
}

func failedHresult(hr uintptr) bool {
	return int32(uint32(hr)) < 0
}

func findProcs(procs ...*windows.LazyProc) error {
	for _, proc := range procs {
		if err := proc.Find(); err != nil {
			return err
		}
	}
	return nil
}

func pdhError(call string, status uintptr) error {
	return fmt.Errorf("%s failed: 0x%08x", call, uint32(status))
}
