//go:build darwin

package agent

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ebitengine/purego"
)

// Apple silicon system memory bandwidth from IOReport, loaded with purego so
// the agent stays CGO-free. The byte-exact "AMC Stats" counters need root, so
// this uses the PMP "DCS BW" histograms, which any user can subscribe to. Each
// histogram counts how often the power manager saw DRAM bandwidth in a bucket
// ("4GB/s", "8GB/s", ...); the count-weighted bucket value is the average rate
// over the interval. "AMCC RD" and "AMCC WR" cover all memory clients.
// On an M6 this tracked memory copy loads within about 5% (buckets are 4 GB/s).

const (
	ioReportLib       = "/usr/lib/libIOReport.dylib"
	coreFoundationLib = "/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation"
	cfStringUTF8      = 0x08000100
)

var memBwIORChannels = [2]string{memBwRead: "AMCC RD", memBwWrite: "AMCC WR"}

var (
	cfStringCreateWithCString     func(alloc uintptr, s string, enc uint32) uintptr
	cfStringGetCString            func(s uintptr, buf *byte, size int64, enc uint32) bool
	cfDictionaryGetValue          func(d, key uintptr) uintptr
	cfArrayGetCount               func(a uintptr) int64
	cfArrayGetValueAtIndex        func(a uintptr, i int64) uintptr
	cfRelease                     func(x uintptr)
	ioReportCopyChannelsInGroup   func(group, subgroup uintptr, a, b, c uint64) uintptr
	ioReportCreateSubscription    func(a, channels uintptr, subscribed *uintptr, b uint64, c uintptr) uintptr
	ioReportCreateSamples         func(sub, subscribed, a uintptr) uintptr
	ioReportCreateSamplesDelta    func(prev, cur, a uintptr) uintptr
	ioReportChannelGetChannelName func(ch uintptr) uintptr
	ioReportStateGetCount         func(ch uintptr) int32
	ioReportStateGetNameForIndex  func(ch uintptr, i int32) uintptr
	ioReportStateGetResidency     func(ch uintptr, i int32) int64
)

func registerDarwinFuncs(lib uintptr, funcs map[string]any) error {
	for name, fptr := range funcs {
		sym, err := purego.Dlsym(lib, name)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		purego.RegisterFunc(fptr, sym)
	}
	return nil
}

// Keep successful library handles for the process lifetime, and register the
// shared function pointers only once.
var loadIOReport = sync.OnceValue(func() error {
	cf, err := purego.Dlopen(coreFoundationLib, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return err
	}
	loaded := false
	defer func() {
		if !loaded {
			_ = purego.Dlclose(cf)
		}
	}()
	ior, err := purego.Dlopen(ioReportLib, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return err
	}
	defer func() {
		if !loaded {
			_ = purego.Dlclose(ior)
		}
	}()
	if err := registerDarwinFuncs(cf, map[string]any{
		"CFStringCreateWithCString": &cfStringCreateWithCString,
		"CFStringGetCString":        &cfStringGetCString,
		"CFDictionaryGetValue":      &cfDictionaryGetValue,
		"CFArrayGetCount":           &cfArrayGetCount,
		"CFArrayGetValueAtIndex":    &cfArrayGetValueAtIndex,
		"CFRelease":                 &cfRelease,
	}); err != nil {
		return err
	}
	err = registerDarwinFuncs(ior, map[string]any{
		"IOReportCopyChannelsInGroup":   &ioReportCopyChannelsInGroup,
		"IOReportCreateSubscription":    &ioReportCreateSubscription,
		"IOReportCreateSamples":         &ioReportCreateSamples,
		"IOReportCreateSamplesDelta":    &ioReportCreateSamplesDelta,
		"IOReportChannelGetChannelName": &ioReportChannelGetChannelName,
		"IOReportStateGetCount":         &ioReportStateGetCount,
		"IOReportStateGetNameForIndex":  &ioReportStateGetNameForIndex,
		"IOReportStateGetResidency":     &ioReportStateGetResidency,
	})
	loaded = err == nil
	return err
})

func cfString(s uintptr) string {
	if s == 0 {
		return ""
	}
	var buf [128]byte
	if !cfStringGetCString(s, &buf[0], int64(len(buf)), cfStringUTF8) {
		return ""
	}
	n := 0
	for n < len(buf) && buf[n] != 0 {
		n++
	}
	return string(buf[:n])
}

// ioReportBandwidth holds a PMP "DCS BW" subscription and integrates the
// histogram averages into cumulative byte counters.
type ioReportBandwidth struct {
	mu          sync.Mutex
	sub         uintptr
	subscribed  uintptr
	channelsKey uintptr
	prev        uintptr
	prevAt      time.Time
	bytes       [2]uint64
	ns          uint64
}

func newIOReportBandwidth() (*ioReportBandwidth, error) {
	group := cfStringCreateWithCString(0, "PMP", cfStringUTF8)
	subgroup := cfStringCreateWithCString(0, "DCS BW", cfStringUTF8)
	defer cfRelease(group)
	defer cfRelease(subgroup)
	channels := ioReportCopyChannelsInGroup(group, subgroup, 0, 0, 0)
	if channels == 0 {
		return nil, fmt.Errorf("no PMP DCS BW channels")
	}
	defer cfRelease(channels)
	r := &ioReportBandwidth{channelsKey: cfStringCreateWithCString(0, "IOReportChannels", cfStringUTF8)}
	ready := false
	defer func() {
		if !ready {
			r.close()
		}
	}()
	r.sub = ioReportCreateSubscription(0, channels, &r.subscribed, 0, 0)
	if r.sub == 0 || r.subscribed == 0 {
		return nil, fmt.Errorf("IOReport subscription failed")
	}
	r.prev = ioReportCreateSamples(r.sub, r.subscribed, 0)
	r.prevAt = time.Now()
	if r.prev == 0 {
		return nil, fmt.Errorf("IOReport sample failed")
	}
	found := 0
	r.forEachChannel(r.prev, func(dir int, _ uintptr) { found |= 1 << dir })
	if found != 3 {
		return nil, fmt.Errorf("no %s/%s histograms", memBwIORChannels[memBwRead], memBwIORChannels[memBwWrite])
	}
	ready = true
	return r, nil
}

func (r *ioReportBandwidth) close() {
	for _, ref := range []*uintptr{&r.prev, &r.sub, &r.subscribed, &r.channelsKey} {
		if *ref != 0 {
			cfRelease(*ref)
			*ref = 0
		}
	}
}

func (r *ioReportBandwidth) forEachChannel(samples uintptr, fn func(dir int, ch uintptr)) {
	arr := cfDictionaryGetValue(samples, r.channelsKey)
	if arr == 0 {
		return
	}
	for i := range cfArrayGetCount(arr) {
		ch := cfArrayGetValueAtIndex(arr, i)
		name := cfString(ioReportChannelGetChannelName(ch))
		for dir, want := range memBwIORChannels {
			if name == want {
				fn(dir, ch)
			}
		}
	}
}

// histogramMean returns the count-weighted bucket value in bytes/s.
func histogramMean(ch uintptr) (float64, bool) {
	var total, weighted float64
	for i := range ioReportStateGetCount(ch) {
		count := ioReportStateGetResidency(ch, i)
		if count <= 0 {
			continue
		}
		bps, ok := parseBandwidthLabel(cfString(ioReportStateGetNameForIndex(ch, i)))
		if !ok {
			return 0, false
		}
		total += float64(count)
		weighted += float64(count) * bps
	}
	if total == 0 {
		return 0, false
	}
	return weighted / total, true
}

func (r *ioReportBandwidth) readings() ([]memBwReading, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur := ioReportCreateSamples(r.sub, r.subscribed, 0)
	if cur == 0 {
		return nil, fmt.Errorf("IOReport sample failed")
	}
	now := time.Now()
	delta := ioReportCreateSamplesDelta(r.prev, cur, 0)
	cfRelease(r.prev)
	dt := now.Sub(r.prevAt)
	r.prev, r.prevAt = cur, now
	if delta != 0 {
		var rates [2]float64
		seen := 0
		r.forEachChannel(delta, func(dir int, ch uintptr) {
			if bps, ok := histogramMean(ch); ok {
				rates[dir] = bps
				seen |= 1 << dir
			}
		})
		cfRelease(delta)
		if seen == 3 && dt > 0 {
			for dir := range rates {
				r.bytes[dir] += uint64(rates[dir] * dt.Seconds())
			}
			r.ns += uint64(dt)
		}
	}
	return []memBwReading{
		{value: r.bytes[memBwRead], enabled: r.ns, running: r.ns},
		{value: r.bytes[memBwWrite], enabled: r.ns, running: r.ns},
	}, nil
}

// newMemBandwidthManager subscribes to the IOReport bandwidth histograms. It
// returns nil when IOReport or the expected channels are unavailable.
func newMemBandwidthManager() (m *memBandwidthManager) {
	defer func() {
		if r := recover(); r != nil {
			slog.Debug("Memory bandwidth disabled", "err", r)
			m = nil
		}
	}()
	if err := loadIOReport(); err != nil {
		slog.Debug("Memory bandwidth disabled", "err", err)
		return nil
	}
	r, err := newIOReportBandwidth()
	if err != nil {
		slog.Debug("Memory bandwidth disabled", "err", err)
		return nil
	}
	slog.Debug("Memory bandwidth enabled", "source", "IOReport PMP DCS BW")
	return &memBandwidthManager{
		counters: memBwByteCounters("ioreport_pmp"),
		read:     r.readings,
		now:      time.Now,
		prev:     make(map[uint16]memBwSample),
	}
}
