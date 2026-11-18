package agent

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	libvirt "github.com/digitalocean/go-libvirt"
	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/vm"
)

const libvirtMemoryMaxAge = 5 * time.Minute

type libvirtDomainMetadata struct {
	vcpus     uint16
	autostart bool
	updated   time.Time
}

type libvirtCounters struct {
	at        time.Time
	cpu       uint64
	cpuValid  bool
	netSent   uint64
	netRecv   uint64
	netValid  bool
	diskRead  uint64
	diskWrite uint64
	diskValid bool
}

type libvirtManager struct {
	client      *libvirt.Libvirt
	uri         *url.URL
	connect     func(*url.URL) (*libvirt.Libvirt, error)
	hostThreads int
	metadata    map[string]libvirtDomainMetadata
	previous    map[uint16]map[string]libvirtCounters
	now         func() time.Time
}

// newLibvirtManager enables VM collection only when LIBVIRT_URI or
// LIBVIRT_SOCKET is configured. The socket form is convenient for containers
// and should point at libvirt-sock-ro.
func newLibvirtManager(agent *Agent) (*libvirtManager, error) {
	rawURI, uriSet := utils.GetEnv("LIBVIRT_URI")
	socket, socketSet := utils.GetEnv("LIBVIRT_SOCKET")
	if !uriSet && !socketSet {
		return nil, nil
	}
	if !uriSet {
		u := &url.URL{Scheme: "qemu", Path: "/system"}
		query := u.Query()
		query.Set("socket", socket)
		u.RawQuery = query.Encode()
		rawURI = u.String()
	}
	u, err := url.Parse(rawURI)
	if err != nil || u.Scheme == "" {
		return nil, fmt.Errorf("invalid LIBVIRT_URI %q", rawURI)
	}
	if socketSet && uriSet {
		query := u.Query()
		query.Set("socket", socket)
		u.RawQuery = query.Encode()
	}
	manager := &libvirtManager{
		uri:         u,
		connect:     libvirt.ConnectToURI,
		hostThreads: agent.systemDetails.Threads,
		metadata:    make(map[string]libvirtDomainMetadata),
		previous:    make(map[uint16]map[string]libvirtCounters),
		now:         time.Now,
	}
	if manager.hostThreads < 1 {
		manager.hostThreads = 1
	}
	if err := manager.ensureConnected(); err != nil {
		// Return the manager as well as the error so later collection attempts
		// retry automatically when libvirt becomes available.
		return manager, err
	}
	return manager, nil
}

func sanitizedLibvirtURI(u *url.URL) string {
	copy := *u
	if copy.User != nil {
		copy.User = url.User(copy.User.Username())
	}
	return copy.String()
}

func (m *libvirtManager) close() {
	if m != nil && m.client != nil {
		_ = m.client.Disconnect()
	}
}

func (m *libvirtManager) ensureConnected() error {
	if m.client != nil && m.client.IsConnected() {
		return nil
	}
	client, err := m.connect(m.uri)
	if err != nil {
		return err
	}
	m.client = client
	slog.Info("Libvirt monitoring connected", "uri", sanitizedLibvirtURI(m.uri))
	return nil
}

func (m *libvirtManager) getStats(cacheTimeMs uint16) ([]*vm.Stats, error) {
	if err := m.ensureConnected(); err != nil {
		return nil, err
	}
	statsMask := uint32(libvirt.DomainStatsState | libvirt.DomainStatsCPUTotal |
		libvirt.DomainStatsBalloon | libvirt.DomainStatsInterface | libvirt.DomainStatsBlock)
	flags := uint32(libvirt.ConnectGetAllDomainsStatsActive | libvirt.ConnectGetAllDomainsStatsInactive |
		libvirt.ConnectGetAllDomainsStatsNowait)
	records, err := m.client.ConnectGetAllDomainStats(nil, statsMask, flags)
	if err != nil {
		return nil, err
	}
	now := m.now()
	previous := m.previous[cacheTimeMs]
	if previous == nil {
		previous = make(map[string]libvirtCounters)
		m.previous[cacheTimeMs] = previous
	}
	result := make([]*vm.Stats, 0, len(records))
	seen := make(map[string]struct{}, len(records))
	for i := range records {
		record := &records[i]
		uuid := formatLibvirtUUID(record.Dom.UUID)
		seen[uuid] = struct{}{}
		params := makeLibvirtParams(record.Params)
		metadata := m.domainMetadata(record.Dom, uuid, now)
		current := libvirtCounters{
			at:        now,
			cpu:       params.uint64("cpu.time"),
			cpuValid:  params.has("cpu.time"),
			netSent:   params.indexedSum("net", ".tx.bytes"),
			netRecv:   params.indexedSum("net", ".rx.bytes"),
			netValid:  params.has("net.count"),
			diskRead:  params.indexedSum("block", ".rd.bytes"),
			diskWrite: params.indexedSum("block", ".wr.bytes"),
			diskValid: params.has("block.count"),
		}
		stat := &vm.Stats{
			Name:           record.Dom.Name,
			UUID:           uuid,
			State:          vm.State(params.uint64("state.state")),
			VCPUs:          metadata.vcpus,
			Autostart:      metadata.autostart,
			DiskCapacity:   params.indexedSum("block", ".capacity"),
			DiskAllocation: params.indexedSum("block", ".allocation"),
		}
		if last, ok := previous[uuid]; ok {
			elapsed := now.Sub(last.at)
			if elapsed > 0 {
				if cpuRate, valid := validCounterRate(current.cpu, current.cpuValid, last.cpu, last.cpuValid, elapsed); valid {
					stat.CPU = utils.TwoDecimals(cpuRate / float64(m.hostThreads) / 1e9 * 100)
					if stat.VCPUs > 0 {
						stat.GuestCPU = utils.TwoDecimals(cpuRate / float64(stat.VCPUs) / 1e9 * 100)
					}
				}
				if current.netValid && last.netValid {
					stat.Bandwidth = [2]uint64{
						uint64(counterRate(current.netSent, last.netSent, elapsed)),
						uint64(counterRate(current.netRecv, last.netRecv, elapsed)),
					}
				}
				if current.diskValid && last.diskValid {
					stat.DiskIO = [2]uint64{
						uint64(counterRate(current.diskRead, last.diskRead, elapsed)),
						uint64(counterRate(current.diskWrite, last.diskWrite, elapsed)),
					}
				}
			}
		}
		previous[uuid] = current
		setLibvirtMemory(stat, params, now)
		result = append(result, stat)
	}
	for uuid := range previous {
		if _, ok := seen[uuid]; !ok {
			delete(previous, uuid)
			delete(m.metadata, uuid)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left := strings.ToLower(result[i].Name)
		right := strings.ToLower(result[j].Name)
		if left != right {
			return left < right
		}
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].UUID < result[j].UUID
	})
	return result, nil
}

func (m *libvirtManager) domainMetadata(domain libvirt.Domain, uuid string, now time.Time) libvirtDomainMetadata {
	if metadata, ok := m.metadata[uuid]; ok && now.Sub(metadata.updated) < 10*time.Minute {
		return metadata
	}
	metadata := libvirtDomainMetadata{updated: now}
	_, _, _, vcpus, _, err := m.client.DomainGetInfo(domain)
	if err == nil {
		metadata.vcpus = vcpus
	}
	if autostart, err := m.client.DomainGetAutostart(domain); err == nil {
		metadata.autostart = autostart != 0
	}
	m.metadata[uuid] = metadata
	return metadata
}

func setLibvirtMemory(stat *vm.Stats, params libvirtParams, now time.Time) {
	lastUpdate := params.uint64("balloon.last-update")
	available := params.uint64("balloon.available")
	usable := params.uint64("balloon.usable")
	if lastUpdate == 0 || available == 0 || usable > available {
		return
	}
	age := now.Sub(time.Unix(int64(lastUpdate), 0))
	if age < 0 || age > libvirtMemoryMaxAge {
		return
	}
	stat.MemoryFresh = true
	stat.MemoryTotal = utils.TwoDecimals(float64(available) / 1024)
	stat.Memory = utils.TwoDecimals(float64(available-usable) / 1024)
}

func counterRate(current, previous uint64, elapsed time.Duration) float64 {
	if current < previous || elapsed <= 0 {
		return 0
	}
	return float64(current-previous) / elapsed.Seconds()
}

func validCounterRate(current uint64, currentValid bool, previous uint64, previousValid bool, elapsed time.Duration) (float64, bool) {
	if !currentValid || !previousValid {
		return 0, false
	}
	return counterRate(current, previous, elapsed), true
}

func formatLibvirtUUID(uuid libvirt.UUID) string {
	raw := hex.EncodeToString(uuid[:])
	if len(raw) != 32 {
		return raw
	}
	return raw[0:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:32]
}

type libvirtParams map[string]libvirt.TypedParamValue

func (p libvirtParams) has(name string) bool {
	_, ok := p[name]
	return ok
}

func (p libvirtParams) uint64(name string) uint64 {
	value, ok := p[name]
	if !ok {
		return 0
	}
	switch v := value.I.(type) {
	case uint64:
		return v
	case uint32:
		return uint64(v)
	case int64:
		if v >= 0 {
			return uint64(v)
		}
	case int32:
		if v >= 0 {
			return uint64(v)
		}
	case string:
		parsed, _ := strconv.ParseUint(v, 10, 64)
		return parsed
	}
	return 0
}

func (p libvirtParams) indexedSum(prefix, suffix string) uint64 {
	count := p.uint64(prefix + ".count")
	var total uint64
	for i := uint64(0); i < count; i++ {
		value := p.uint64(fmt.Sprintf("%s.%d%s", prefix, i, suffix))
		if ^uint64(0)-total < value {
			return ^uint64(0)
		}
		total += value
	}
	return total
}

func makeLibvirtParams(params []libvirt.TypedParam) libvirtParams {
	result := make(libvirtParams, len(params))
	for i := range params {
		result[params[i].Field] = params[i].Value
	}
	return result
}
