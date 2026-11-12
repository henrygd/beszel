package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/types"
	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/entities/systemd"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// maxSamples limits historical samples returned by get_system_stats and the
// list sizes of the other tools, keeping responses small for LLM contexts.
const (
	maxSamples     = 400
	maxListRecords = 200
)

// registerTools adds the hub's read-only monitoring tools to the MCP server.
func registerTools(app core.App, server *mcpsdk.Server) {
	h := &mcphub{app: app}
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "list_systems",
		Description: "List monitored systems visible to the authenticated user with current CPU, memory, disk, bandwidth and status.",
	}, h.listSystems)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "get_system",
		Description: "Get hardware details and current usage for one system, looked up by name or id.",
	}, h.getSystem)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name: "get_system_stats",
		Description: "Get historical metrics for one system over a time range. " +
			"Records are stored at 1m, 10m, 20m, 120m and 480m resolutions, kept for 1h, 12h, 24h, 7d and 30d respectively; " +
			"by default the finest resolution covering the requested hours is used.",
	}, h.getSystemStats)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "list_containers",
		Description: "List docker/podman containers of one system with the latest recorded CPU, memory and network usage.",
	}, h.listContainers)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "list_alerts",
		Description: "List the authenticated user's alert configurations (CPU, memory, disk, status, etc.) with state.",
	}, h.listAlerts)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name: "list_alert_events",
		Description: "List triggered/resolved alert history events for the authenticated user's alerts over a time range. " +
			"Use this to answer questions like 'what alerts fired recently and are they resolved'.",
	}, h.listAlertEvents)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name: "list_services",
		Description: "List systemd services of one system with state, CPU and memory usage, looked up by name or id. " +
			"Use state=failed to spot broken services.",
	}, h.listServices)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "get_disk_health",
		Description: "Get SMART health of physical disks (state, temperature, power-on hours) and ZFS pool status for one system.",
	}, h.getDiskHealth)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "list_network_monitors",
		Description: "List network endpoint monitors (ping/tcp/http/dns) of one system with latest latency and loss.",
	}, h.listNetworkMonitors)
}

// ---------------------------------------------------------------------------
// output types

type systemSummary struct {
	Id           string       `json:"id"`
	Name         string       `json:"name"`
	Host         string       `json:"host,omitempty"`
	Status       string       `json:"status"`
	Cpu          float64      `json:"cpu_pct,omitzero"`
	Mem          float64      `json:"memory_pct,omitzero"`
	Disk         float64      `json:"disk_pct,omitzero"`
	Bandwidth    *uint64      `json:"bandwidth_bps,omitzero"` // aggregate bytes/sec
	LoadAvg      *[3]float64  `json:"load_avg,omitzero"`
	Updates      *updatesInfo `json:"updates,omitempty"` // pending package updates
	AgentVersion string       `json:"agent_version,omitzero"`
	Uptime       uint64       `json:"uptime_seconds,omitzero"`
	LastSeen     string       `json:"last_seen,omitzero"` // RFC3339
}

// updatesInfo describes pending package updates reported by the agent
// (security is omitted when the agent cannot determine it).
type updatesInfo struct {
	Total    int  `json:"total"`
	Security *int `json:"security,omitempty"`
}

type systemDetail struct {
	systemSummary
	Port    string         `json:"port,omitempty"`
	Gpu     float64        `json:"gpu_pct,omitzero"`
	Current *systemCurrent `json:"current,omitempty"` // latest recorded sensor/usage detail
	Created string         `json:"created,omitzero"`  // RFC3339
	Server  *serverInfo    `json:"server,omitempty"`
}

// systemCurrent carries detailed current usage that is not needed in every
// listSystems entry (sensors, GPUs, extra filesystems), taken from the latest
// 1m system_stats record.
type systemCurrent struct {
	Temperatures map[string]float64        `json:"temperatures,omitempty"` // sensor -> °C
	Gpus         map[string]system.GPUData `json:"gpus,omitempty"`         // GPU id -> usage data
	Filesystems  map[string]fsSummary      `json:"filesystems,omitempty"`  // mount point -> usage
	Fans         map[string]uint16         `json:"fans,omitempty"`         // fan -> RPM
	Battery      *batteryInfo              `json:"battery,omitempty"`
	Swap         *[2]uint64                `json:"swap_bytes,omitempty"` // [total, used] bytes
}

type fsSummary struct {
	Total   float64 `json:"total_gb"`
	Used    float64 `json:"used_gb"`
	ReadPs  uint64  `json:"read_bps,omitzero"`
	WritePs uint64  `json:"write_bps,omitzero"`
}

type batteryInfo struct {
	Percent uint8 `json:"percent"`
}

type serverInfo struct {
	Hostname string  `json:"hostname,omitempty"`
	Kernel   string  `json:"kernel,omitempty"`
	CpuModel string  `json:"cpu_model,omitempty"`
	Os       string  `json:"os,omitempty"`
	Arch     string  `json:"arch,omitempty"`
	Cores    int     `json:"cores,omitzero"`
	Threads  int     `json:"threads,omitzero"`
	Sockets  int     `json:"sockets,omitzero"`
	Memory   *uint64 `json:"memory_bytes,omitzero"` // total
	Podman   bool    `json:"podman,omitzero"`
}

type statsSummary struct {
	Count        int         `json:"count"`
	AvgCpu       float64     `json:"avg_cpu_pct"`
	MaxCpu       float64     `json:"max_cpu_pct"`
	AvgMem       float64     `json:"avg_memory_pct"`
	MaxMem       float64     `json:"max_memory_pct"`
	AvgDisk      float64     `json:"avg_disk_pct"`
	MaxDisk      float64     `json:"max_disk_pct"`
	AvgBandwidth *[2]float64 `json:"avg_bandwidth_bps,omitzero"` // [sent, recv]
	MaxBandwidth *[2]float64 `json:"max_bandwidth_bps,omitzero"` // [sent, recv]
	AvgDiskIO    *[2]float64 `json:"avg_disk_io_bps,omitzero"`   // [read, write]
	MaxDiskIO    *[2]float64 `json:"max_disk_io_bps,omitzero"`   // [read, write]
}

type statsSample struct {
	Time      string  `json:"time"` // RFC3339
	Cpu       float64 `json:"cpu_pct"`
	Mem       float64 `json:"memory_pct"`
	Disk      float64 `json:"disk_pct"`
	NetSent   uint64  `json:"net_sent_bps,omitzero"`
	NetRecv   uint64  `json:"net_recv_bps,omitzero"`
	DiskRead  uint64  `json:"disk_read_bps,omitzero"`
	DiskWrite uint64  `json:"disk_write_bps,omitzero"`
}

type alertSummary struct {
	Id        string   `json:"id"`
	System    string   `json:"system"` // system id; see systems map in the list result
	Name      string   `json:"name"`
	Value     *float64 `json:"value,omitempty"`
	Min       *float64 `json:"min,omitempty"`
	Triggered bool     `json:"triggered,omitzero"`
	State     string   `json:"state,omitempty"`
	Created   string   `json:"created,omitempty"`
	Updated   string   `json:"updated,omitempty"`
}

// ---------------------------------------------------------------------------
// helpers

// listVisibleSystems returns up to maxListRecords systems visible to the auth
// record, sorted by name, plus whether more exist beyond the cap.
func (h *mcphub) listVisibleSystems(auth *core.Record) ([]*core.Record, bool, error) {
	params := dbx.Params{}
	filter := ""
	if !auth.IsSuperuser() && !shareAllSystems() {
		filter = "users.id ?= {:uid}"
		params["uid"] = auth.Id
	}
	records, err := h.app.FindRecordsByFilter("systems", filter, "name", maxListRecords, 0, params)
	if err != nil {
		return nil, false, err
	}
	hasMore := false
	if len(records) == maxListRecords {
		hasMore = h.hasMoreRecords("systems", filter, "name", params)
	}
	return records, hasMore, nil
}

// hasMoreRecords reports whether at least one record exists beyond the
// maxListRecords cap for the given filter, fetch costing a single record.
func (h *mcphub) hasMoreRecords(collection string, filter string, sort string, params dbx.Params) bool {
	extra, err := h.app.FindRecordsByFilter(collection, filter, sort, 1, maxListRecords, params)
	return err == nil && len(extra) > 0
}

// newSystemSummary builds the common fields from a system record.
func newSystemSummary(record *core.Record) systemSummary {
	s := systemSummary{
		Id:     record.Id,
		Name:   record.GetString("name"),
		Host:   record.GetString("host"),
		Status: record.GetString("status"),
	}
	var info system.Info
	if err := record.UnmarshalJSONField("info", &info); err == nil {
		s.Cpu = info.Cpu
		s.Mem = info.MemPct
		s.Disk = info.DiskPct
		s.AgentVersion = info.AgentVersion
		s.Uptime = info.Uptime
		if info.BandwidthBytes > 0 {
			bandwidth := info.BandwidthBytes
			s.Bandwidth = &bandwidth
		} else if info.Bandwidth > 0 {
			// deprecated MB/s field sent by older agents
			bandwidth := uint64(info.Bandwidth * 1024 * 1024)
			s.Bandwidth = &bandwidth
		}
		if loadAvg := info.LoadAvg; loadAvg != [3]float64{} {
			s.LoadAvg = &loadAvg
		}
		if len(info.PackageUpdates) > 0 {
			updates := &updatesInfo{Total: int(info.PackageUpdates[0])}
			if len(info.PackageUpdates) > 1 {
				security := int(info.PackageUpdates[1])
				updates.Security = &security
			}
			s.Updates = updates
		}
	}
	if updated := record.GetDateTime("updated"); !updated.IsZero() {
		s.LastSeen = updated.Time().UTC().Format(time.RFC3339)
	}
	return s
}

// ---------------------------------------------------------------------------
// tools

type noParams struct{}

// listSystems returns all visible systems with current usage.
func (h *mcphub) listSystems(ctx context.Context, req *mcpsdk.CallToolRequest, _ noParams) (*mcpsdk.CallToolResult, systemList, error) {
	auth, err := h.authRecord(ctx)
	if err != nil {
		return nil, systemList{}, err
	}
	records, hasMore, err := h.listVisibleSystems(auth)
	if err != nil {
		return nil, systemList{}, err
	}
	systems := make([]systemSummary, 0, len(records))
	for _, record := range records {
		systems = append(systems, newSystemSummary(record))
	}
	return nil, systemList{Count: len(systems), Truncated: hasMore, Systems: systems}, nil
}

type systemList struct {
	Count     int             `json:"count"`
	Truncated bool            `json:"truncated,omitzero"` // more systems exist beyond the returned cap
	Systems   []systemSummary `json:"systems"`
}

type getSystemParams struct {
	System string `json:"system" jsonschema:"system name or id"`
}

// getSystem returns details for one system.
func (h *mcphub) getSystem(ctx context.Context, req *mcpsdk.CallToolRequest, params getSystemParams) (*mcpsdk.CallToolResult, systemDetail, error) {
	auth, err := h.authRecord(ctx)
	if err != nil {
		return nil, systemDetail{}, err
	}
	record, err := h.resolveSystem(auth, params.System)
	if err != nil {
		return nil, systemDetail{}, err
	}
	detail := systemDetail{
		systemSummary: newSystemSummary(record),
		Port:          record.GetString("port"),
		Created:       record.GetDateTime("created").Time().UTC().Format(time.RFC3339),
	}
	var info system.Info
	if err := record.UnmarshalJSONField("info", &info); err == nil {
		detail.Gpu = info.GpuPct
	}
	// detailed current metrics from the latest 1m system_stats record
	if latest, err := h.app.FindRecordsByFilter("system_stats", "system = {:system} && type = {:type}", "-created", 1, 0,
		dbx.Params{"system": record.Id, "type": "1m"}); err == nil && len(latest) > 0 {
		detail.Current = systemCurrentFromStats(latest[0])
	}
	// hardware details reported by the agent
	if detailsRecord, err := h.app.FindRecordById("system_details", record.Id); err == nil {
		si := &serverInfo{
			Hostname: detailsRecord.GetString("hostname"),
			Kernel:   detailsRecord.GetString("kernel"),
			CpuModel: detailsRecord.GetString("cpu"),
			Arch:     detailsRecord.GetString("arch"),
			Cores:    detailsRecord.GetInt("cores"),
			Threads:  detailsRecord.GetInt("threads"),
			Sockets:  detailsRecord.GetInt("sockets"),
			Podman:   detailsRecord.GetBool("podman"),
		}
		si.Os = detailsRecord.GetString("os_name")
		if mem := detailsRecord.GetFloat("memory"); mem > 0 {
			si.Memory = new(uint64)
			*si.Memory = uint64(mem)
		}
		detail.Server = si
	}
	return nil, detail, nil
}

// systemCurrentFromStats extracts detailed current metrics from a system_stats
// record, returning nil when it holds nothing useful.
func systemCurrentFromStats(record *core.Record) *systemCurrent {
	var s system.Stats
	if err := record.UnmarshalJSONField("stats", &s); err != nil {
		return nil
	}
	current := &systemCurrent{}
	if len(s.Temperatures) > 0 {
		current.Temperatures = s.Temperatures
	}
	if len(s.GPUData) > 0 {
		current.Gpus = s.GPUData
	}
	if len(s.Fans) > 0 {
		current.Fans = s.Fans
	}
	if s.SwapBytes != [2]uint64{} {
		current.Swap = &s.SwapBytes
	}
	if s.Battery != [2]uint8{} {
		current.Battery = &batteryInfo{Percent: s.Battery[0]}
	}
	if len(s.ExtraFs) > 0 {
		filesystems := make(map[string]fsSummary, len(s.ExtraFs))
		for name, fs := range s.ExtraFs {
			if fs == nil {
				continue
			}
			summary := fsSummary{Total: fs.DiskTotal, Used: fs.DiskUsed}
			summary.ReadPs, summary.WritePs = fs.DiskReadBytes, fs.DiskWriteBytes
			// deprecated MB/s fields from agents < 0.13.0
			if summary.ReadPs == 0 && (fs.DiskReadPs > 0 || fs.DiskWritePs > 0) {
				summary.ReadPs = uint64(fs.DiskReadPs * 1024 * 1024)
				summary.WritePs = uint64(fs.DiskWritePs * 1024 * 1024)
			}
			filesystems[name] = summary
		}
		current.Filesystems = filesystems
	}
	if current.Temperatures == nil && current.Gpus == nil && current.Filesystems == nil &&
		current.Fans == nil && current.Battery == nil && current.Swap == nil {
		return nil
	}
	return current
}

type getSystemStatsParams struct {
	System string  `json:"system" jsonschema:"system name or id"`
	Period string  `json:"period,omitzero" jsonschema:"record resolution: one of 1m, 10m, 20m, 120m or 480m (default: the finest resolution that covers the requested hours)"`
	Hours  float64 `json:"hours,omitzero" jsonschema:"how many hours to look back (1-720, default 24)"`
}

// statsPeriods lists the stored system_stats resolutions with their retention
// (see records.deleteOldSystemStats), from finest to coarsest.
var statsPeriods = []struct {
	period    string
	retention time.Duration
	label     string // human readable retention
}{
	{"1m", time.Hour, "1 hour"},
	{"10m", 12 * time.Hour, "12 hours"},
	{"20m", 24 * time.Hour, "24 hours"},
	{"120m", 7 * 24 * time.Hour, "7 days"},
	{"480m", 30 * 24 * time.Hour, "30 days"},
}

// maxStatsHours is the longest lookback, matching the 480m retention.
const maxStatsHours = 720

type systemStats struct {
	System  string        `json:"system"`
	Period  string        `json:"period"`
	From    string        `json:"from"` // RFC3339
	To      string        `json:"to"`   // RFC3339
	Note    string        `json:"note,omitempty"`
	Summary statsSummary  `json:"summary"`
	Samples []statsSample `json:"samples"`
}

// resolvePeriod validates the period param against the stored resolutions and
// returns it with its retention and retention label. Without a period, the
// finest resolution whose retention covers the lookback is used (like the web
// UI chart ranges).
func resolvePeriod(period string, hours float64) (string, time.Duration, string, error) {
	lookback := time.Duration(hours * float64(time.Hour))
	for _, p := range statsPeriods {
		if period == p.period || (period == "" && lookback <= p.retention) {
			return p.period, p.retention, p.label, nil
		}
	}
	if period == "" {
		last := statsPeriods[len(statsPeriods)-1]
		return last.period, last.retention, last.label, nil
	}
	return "", 0, "", fmt.Errorf("invalid period %q: must be one of 1m, 10m, 20m, 120m, 480m", period)
}

// getSystemStats returns historical samples and a summary for one system.
func (h *mcphub) getSystemStats(ctx context.Context, req *mcpsdk.CallToolRequest, params getSystemStatsParams) (*mcpsdk.CallToolResult, systemStats, error) {
	auth, err := h.authRecord(ctx)
	if err != nil {
		return nil, systemStats{}, err
	}
	hours := params.Hours
	if hours <= 0 {
		hours = 24
	}
	hours = min(hours, maxStatsHours)
	period, retention, retentionLabel, err := resolvePeriod(params.Period, hours)
	if err != nil {
		return nil, systemStats{}, err
	}
	record, err := h.resolveSystem(auth, params.System)
	if err != nil {
		return nil, systemStats{}, err
	}
	// an explicit period can't reach further back than its retention, so
	// report the range actually covered instead of the requested one
	lookback := time.Duration(hours * float64(time.Hour))
	var note string
	if lookback > retention {
		lookback = retention
		note = fmt.Sprintf("%s records are only kept for %s, so the range was shortened; omit period to pick a resolution covering %g hours",
			period, retentionLabel, hours)
	}
	now := time.Now().UTC()
	from := now.Add(-lookback)
	result := systemStats{
		System: record.GetString("name"),
		Period: period,
		From:   from.Format(time.RFC3339),
		To:     now.Format(time.RFC3339),
		Note:   note,
	}
	systemStatsParams := dbx.Params{
		"system": record.Id,
		"period": period,
		"from":   from.Format(types.DefaultDateLayout),
	}
	records, err := h.app.FindRecordsByFilter("system_stats", "system = {:system} && type = {:period} && created >= {:from}",
		"created", 0, 0, systemStatsParams)
	if err != nil {
		return nil, systemStats{}, err
	}
	if len(records) == 0 {
		empty := fmt.Sprintf("no %s records in this range (the system may be new, down or paused)", period)
		if note != "" {
			empty = note + "; " + empty
		}
		result.Note = empty
		return nil, result, nil
	}
	// decode stats records once; records that fail to decode are skipped
	decoded := make([]decodedStat, 0, len(records))
	for _, statsRecord := range records {
		var s system.Stats
		if err := statsRecord.UnmarshalJSONField("stats", &s); err != nil {
			continue
		}
		decoded = append(decoded, decodedStat{
			time:  statsRecord.GetDateTime("created").Time().UTC().Format(time.RFC3339),
			stats: s,
		})
	}
	// build samples (capped by even stride if needed)
	stride := 1
	if len(decoded) > maxSamples {
		stride = (len(decoded) + maxSamples - 1) / maxSamples
	}
	samples := make([]statsSample, 0, len(decoded)/stride)
	for i, item := range decoded {
		if i%stride != 0 {
			continue
		}
		samples = append(samples, newStatsSample(item.time, item.stats))
	}
	result.Samples = samples
	result.Summary = summarizeStats(decoded)
	return nil, result, nil
}

// decodedStat pairs a system_stats record's creation time with its stats,
// decoded once for both samples and summary.
type decodedStat struct {
	time  string
	stats system.Stats
}

// newStatsSample converts a stats struct into the tool's sample output,
// migrating deprecated fields (MB/s) from older agents to bytes/s.
func newStatsSample(recordTime string, s system.Stats) statsSample {
	sample := statsSample{
		Time: recordTime,
		Cpu:  s.Cpu, Mem: s.MemPct, Disk: s.DiskPct,
	}
	switch {
	case s.Bandwidth[0] > 0 || s.Bandwidth[1] > 0:
		sample.NetSent, sample.NetRecv = s.Bandwidth[0], s.Bandwidth[1]
	case s.NetworkSent > 0 || s.NetworkRecv > 0:
		// deprecated MB/s fields
		sample.NetSent = uint64(s.NetworkSent * 1024 * 1024)
		sample.NetRecv = uint64(s.NetworkRecv * 1024 * 1024)
	}
	switch {
	case s.DiskIO[0] > 0 || s.DiskIO[1] > 0:
		sample.DiskRead, sample.DiskWrite = s.DiskIO[0], s.DiskIO[1]
	case s.DiskReadPs > 0 || s.DiskWritePs > 0:
		// deprecated MB/s fields
		sample.DiskRead = uint64(s.DiskReadPs * 1024 * 1024)
		sample.DiskWrite = uint64(s.DiskWritePs * 1024 * 1024)
	}
	return sample
}

// summarizeStats computes aggregate metrics over the decoded records.
func summarizeStats(decoded []decodedStat) statsSummary {
	summary := statsSummary{Count: len(decoded)}
	if len(decoded) == 0 {
		return summary
	}
	var avgBandwidth, maxBandwidth, avgDiskIO, maxDiskIO [2]float64
	count := float64(len(decoded))
	for _, item := range decoded {
		s := item.stats
		summary.AvgCpu += s.Cpu
		summary.MaxCpu = max(summary.MaxCpu, s.Cpu)
		summary.AvgMem += s.MemPct
		summary.MaxMem = max(summary.MaxMem, s.MemPct)
		summary.AvgDisk += s.DiskPct
		summary.MaxDisk = max(summary.MaxDisk, s.DiskPct)
		// sample conversion applies the deprecated-field fallbacks
		sample := newStatsSample("", s)
		avgBandwidth[0] += float64(sample.NetSent)
		avgBandwidth[1] += float64(sample.NetRecv)
		maxBandwidth[0] = max(maxBandwidth[0], float64(sample.NetSent))
		maxBandwidth[1] = max(maxBandwidth[1], float64(sample.NetRecv))
		avgDiskIO[0] += float64(sample.DiskRead)
		avgDiskIO[1] += float64(sample.DiskWrite)
		maxDiskIO[0] = max(maxDiskIO[0], float64(sample.DiskRead))
		maxDiskIO[1] = max(maxDiskIO[1], float64(sample.DiskWrite))
	}
	f := func(v [2]float64) *[2]float64 { return &v }
	summary.AvgCpu /= count
	summary.AvgMem /= count
	summary.AvgDisk /= count
	summary.AvgBandwidth = f([2]float64{avgBandwidth[0] / count, avgBandwidth[1] / count})
	summary.MaxBandwidth = f(maxBandwidth)
	summary.AvgDiskIO = f([2]float64{avgDiskIO[0] / count, avgDiskIO[1] / count})
	summary.MaxDiskIO = f(maxDiskIO)
	return summary
}

type containerSummary struct {
	Name            string     `json:"name"`
	Cpu             float64    `json:"cpu_pct,omitzero"`
	Mem             float64    `json:"mem_mb,omitzero"`
	Bandwidth       *[2]uint64 `json:"bandwidth_bps,omitzero"` // [sent, recv]
	UpdateAvailable bool       `json:"update_available,omitzero"`
}

type containerList struct {
	System     string             `json:"system"`
	Containers []containerSummary `json:"containers"`
}

// listContainers returns the latest recorded docker stats for one system.
func (h *mcphub) listContainers(ctx context.Context, req *mcpsdk.CallToolRequest, params getSystemParams) (*mcpsdk.CallToolResult, containerList, error) {
	auth, err := h.authRecord(ctx)
	if err != nil {
		return nil, containerList{}, err
	}
	record, err := h.resolveSystem(auth, params.System)
	if err != nil {
		return nil, containerList{}, err
	}
	result := containerList{System: record.GetString("name")}
	statsRecords, err := h.app.FindRecordsByFilter("container_stats", "system = {:system}", "-created", 1, 0,
		dbx.Params{"system": record.Id})
	if err != nil {
		return nil, containerList{}, err
	}
	if len(statsRecords) == 0 {
		return nil, result, nil
	}
	var containers []*container.Stats
	if err := statsRecords[0].UnmarshalJSONField("stats", &containers); err != nil {
		return nil, containerList{}, err
	}
	result.Containers = make([]containerSummary, 0, len(containers))
	for _, c := range containers {
		if c == nil {
			continue
		}
		summary := containerSummary{Name: c.Name, Cpu: c.Cpu, Mem: c.Mem, UpdateAvailable: c.UpdateAvailable}
		sent, recv := c.Bandwidth[0], c.Bandwidth[1]
		if sent == 0 && recv == 0 && (c.NetworkSent > 0 || c.NetworkRecv > 0) {
			sent = uint64(c.NetworkSent * 1024 * 1024)
			recv = uint64(c.NetworkRecv * 1024 * 1024)
		}
		if sent > 0 || recv > 0 {
			summary.Bandwidth = &[2]uint64{sent, recv}
		}
		result.Containers = append(result.Containers, summary)
	}
	return nil, result, nil
}

// listAlerts returns the authenticated user's alerts (all alerts for superusers).
func (h *mcphub) listAlerts(ctx context.Context, req *mcpsdk.CallToolRequest, _ noParams) (*mcpsdk.CallToolResult, alertList, error) {
	auth, err := h.authRecord(ctx)
	if err != nil {
		return nil, alertList{}, err
	}
	params := dbx.Params{}
	filter := ""
	if !auth.IsSuperuser() {
		filter = "user = {:uid}"
		params["uid"] = auth.Id
	}
	records, err := h.app.FindRecordsByFilter("alerts", filter, "-updated", maxListRecords, 0, params)
	if err != nil {
		return nil, alertList{}, err
	}
	hasMore := false
	if len(records) == maxListRecords {
		hasMore = h.hasMoreRecords("alerts", filter, "-updated", params)
	}
	result := alertList{Count: len(records), Truncated: hasMore, Alerts: make([]alertSummary, 0, len(records))}
	if len(records) == 0 {
		return nil, result, nil
	}
	// resolve system names referenced by the returned alerts
	result.Systems = h.resolveSystemNames(records)
	for _, record := range records {
		alert := alertSummary{
			Id:        record.Id,
			System:    record.GetString("system"),
			Name:      record.GetString("name"),
			Triggered: record.GetBool("triggered"),
			Created:   record.GetDateTime("created").Time().UTC().Format(time.RFC3339),
			Updated:   record.GetDateTime("updated").Time().UTC().Format(time.RFC3339),
		}
		// alerts.state is a JSON field (a state object for network monitor
		// alerts); GetString renders stored JSON null as the literal string
		// "null", so only pass through meaningful values
		if state, ok := record.Get("state").(types.JSONRaw); ok && string(state) != "null" {
			alert.State = string(state)
		}
		if value := record.GetFloat("value"); value != 0 {
			alert.Value = &value
		}
		if min := record.GetFloat("min"); min != 0 {
			alert.Min = &min
		}
		result.Alerts = append(result.Alerts, alert)
	}
	return nil, result, nil
}

type alertList struct {
	Count     int               `json:"count"`
	Truncated bool              `json:"truncated,omitzero"` // more alerts exist beyond the returned cap
	Systems   map[string]string `json:"systems,omitempty"`  // system id -> name
	Alerts    []alertSummary    `json:"alerts"`
}

// ---------------------------------------------------------------------------
// system-scoped helpers

// resolveSystemNames maps the system ids referenced by records to their names
// (returns nil when nothing could be resolved).
func (h *mcphub) resolveSystemNames(records []*core.Record) map[string]string {
	if len(records) == 0 {
		return nil
	}
	params := dbx.Params{}
	conditions := make([]string, 0, len(records))
	for i, record := range records {
		key := fmt.Sprintf("s%d", i)
		conditions = append(conditions, "id = {:"+key+"}")
		params[key] = record.GetString("system")
	}
	names := map[string]string{}
	if systemRecords, err := h.app.FindRecordsByFilter("systems", strings.Join(conditions, " || "), "", len(records), 0, params); err == nil {
		for _, systemRecord := range systemRecords {
			names[systemRecord.Id] = systemRecord.GetString("name")
		}
		return names
	}
	return nil
}

// ---------------------------------------------------------------------------
// list_alert_events

type alertEventParams struct {
	Hours float64 `json:"hours,omitzero" jsonschema:"how many hours to look back (1-720, default 168 = 7 days)"`
}

// listAlertEvents returns triggered/resolved alert history events. History is
// scoped to the authenticated user's alerts like the alerts collection rules;
// superusers see all events.
func (h *mcphub) listAlertEvents(ctx context.Context, req *mcpsdk.CallToolRequest, params alertEventParams) (*mcpsdk.CallToolResult, alertEventList, error) {
	auth, err := h.authRecord(ctx)
	if err != nil {
		return nil, alertEventList{}, err
	}
	hours := params.Hours
	if hours <= 0 {
		hours = 168
	}
	if hours > 720 {
		hours = 720
	}
	now := time.Now().UTC()
	queryParams := dbx.Params{
		"from": now.Add(-time.Duration(hours * float64(time.Hour))).Format(types.DefaultDateLayout),
	}
	filter := "created >= {:from}"
	if !auth.IsSuperuser() {
		filter = "user = {:uid} && created >= {:from}"
		queryParams["uid"] = auth.Id
	}
	records, err := h.app.FindRecordsByFilter("alerts_history", filter, "-created", maxListRecords, 0, queryParams)
	if err != nil {
		return nil, alertEventList{}, err
	}
	hasMore := false
	if len(records) == maxListRecords {
		hasMore = h.hasMoreRecords("alerts_history", filter, "-created", queryParams)
	}
	result := alertEventList{Count: len(records), Truncated: hasMore, Events: make([]alertEvent, 0, len(records))}
	if len(records) == 0 {
		return nil, result, nil
	}
	result.Systems = h.resolveSystemNames(records)
	for _, record := range records {
		event := alertEvent{
			Id:          record.Id,
			System:      record.GetString("system"),
			Name:        record.GetString("name"),
			MonitorName: record.GetString("monitor_name"),
			TriggeredAt: record.GetDateTime("created").Time().UTC().Format(time.RFC3339),
		}
		if value := record.GetFloat("value"); value != 0 {
			event.Value = &value
		}
		// resolved stays unset while the event is ongoing
		if resolved := record.GetDateTime("resolved"); !resolved.IsZero() {
			event.ResolvedAt = resolved.Time().UTC().Format(time.RFC3339)
		}
		result.Events = append(result.Events, event)
	}
	return nil, result, nil
}

type alertEvent struct {
	Id          string   `json:"id"`
	System      string   `json:"system"` // system id; see systems map
	Name        string   `json:"name"`   // alert name (CPU, Memory, StatusFailedServices, ...)
	MonitorName string   `json:"monitor_name,omitempty"`
	Value       *float64 `json:"value,omitempty"` // observed value that triggered the alert
	TriggeredAt string   `json:"triggered_at"`    // RFC3339
	ResolvedAt  string   `json:"resolved_at,omitempty"`
}

type alertEventList struct {
	Count     int               `json:"count"`
	Truncated bool              `json:"truncated,omitzero"`
	Systems   map[string]string `json:"systems,omitempty"` // system id -> name
	Events    []alertEvent      `json:"events"`
}

// ---------------------------------------------------------------------------
// list_services

// serviceStateNames maps the systemd.ServiceState enum values (stored
// numerically in systemd_services records) to their systemd names.
var serviceStateNames = map[uint]string{
	uint(systemd.StatusActive):       "active",
	uint(systemd.StatusInactive):     "inactive",
	uint(systemd.StatusFailed):       "failed",
	uint(systemd.StatusActivating):   "activating",
	uint(systemd.StatusDeactivating): "deactivating",
	uint(systemd.StatusReloading):    "reloading",
}

// serviceSubStateNames maps the systemd.ServiceSubState enum values.
var serviceSubStateNames = map[uint]string{
	uint(systemd.SubStateDead):    "dead",
	uint(systemd.SubStateRunning): "running",
	uint(systemd.SubStateExited):  "exited",
	uint(systemd.SubStateFailed):  "failed",
	uint(systemd.SubStateUnknown): "unknown",
}

// listServices returns the systemd services reported by one system's agent.
func (h *mcphub) listServices(ctx context.Context, req *mcpsdk.CallToolRequest, params getSystemParams) (*mcpsdk.CallToolResult, serviceList, error) {
	auth, err := h.authRecord(ctx)
	if err != nil {
		return nil, serviceList{}, err
	}
	record, err := h.resolveSystem(auth, params.System)
	if err != nil {
		return nil, serviceList{}, err
	}
	filter := "system = {:system}"
	queryParams := dbx.Params{"system": record.Id}
	records, err := h.app.FindRecordsByFilter("systemd_services", filter, "name", maxListRecords, 0, queryParams)
	if err != nil {
		return nil, serviceList{}, err
	}
	hasMore := false
	if len(records) == maxListRecords {
		hasMore = h.hasMoreRecords("systemd_services", filter, "name", queryParams)
	}
	result := serviceList{System: record.GetString("name"), Count: len(records), Truncated: hasMore,
		Services: make([]serviceSummary, 0, len(records))}
	for _, serviceRecord := range records {
		service := serviceSummary{
			Name: serviceRecord.GetString("name"),
			Cpu:  serviceRecord.GetFloat("cpu"),
		}
		if state, ok := serviceStateNames[uint(serviceRecord.GetInt("state"))]; ok {
			service.State = state
		}
		if sub, ok := serviceSubStateNames[uint(serviceRecord.GetInt("sub"))]; ok {
			service.Sub = sub
		}
		if peak := serviceRecord.GetFloat("cpuPeak"); peak > 0 {
			service.CpuPeak = peak
		}
		if mem := serviceRecord.GetFloat("memory"); mem > 0 {
			service.Mem = new(uint64)
			*service.Mem = uint64(mem)
		}
		if memPeak := serviceRecord.GetFloat("memPeak"); memPeak > 0 {
			service.MemPeak = new(uint64)
			*service.MemPeak = uint64(memPeak)
		}
		// updated is stored as unix milliseconds by the hub
		if updatedMs := serviceRecord.GetInt64("updated"); updatedMs > 0 {
			service.Updated = time.UnixMilli(updatedMs).UTC().Format(time.RFC3339)
		}
		result.Services = append(result.Services, service)
	}
	return nil, result, nil
}

type serviceSummary struct {
	Name    string  `json:"name"`
	State   string  `json:"state"` // active, inactive, failed, activating, deactivating, reloading
	Sub     string  `json:"sub,omitempty"`
	Cpu     float64 `json:"cpu_pct,omitzero"`
	CpuPeak float64 `json:"cpu_peak_pct,omitzero"`
	Mem     *uint64 `json:"mem_bytes,omitzero"`
	MemPeak *uint64 `json:"mem_peak_bytes,omitzero"`
	Updated string  `json:"updated,omitempty"` // RFC3339
}

type serviceList struct {
	System    string           `json:"system"`
	Count     int              `json:"count"`
	Truncated bool             `json:"truncated,omitzero"`
	Services  []serviceSummary `json:"services"`
}

// ---------------------------------------------------------------------------
// get_disk_health

// getDiskHealth returns SMART device and ZFS pool health for one system.
func (h *mcphub) getDiskHealth(ctx context.Context, req *mcpsdk.CallToolRequest, params getSystemParams) (*mcpsdk.CallToolResult, diskHealth, error) {
	auth, err := h.authRecord(ctx)
	if err != nil {
		return nil, diskHealth{}, err
	}
	record, err := h.resolveSystem(auth, params.System)
	if err != nil {
		return nil, diskHealth{}, err
	}
	result := diskHealth{System: record.GetString("name")}
	// SMART devices refreshed periodically from the agent
	if records, err := h.app.FindRecordsByFilter("smart_devices", "system = {:system}", "name", maxListRecords, 0,
		dbx.Params{"system": record.Id}); err == nil {
		result.Smart = make([]smartDevice, 0, len(records))
		for _, smartRecord := range records {
			device := smartDevice{
				Name:     smartRecord.GetString("name"),
				Model:    smartRecord.GetString("model"),
				State:    smartRecord.GetString("state"), // SMART status, e.g. PASSED
				Type:     smartRecord.GetString("type"),
				Serial:   smartRecord.GetString("serial"),
				Firmware: smartRecord.GetString("firmware"),
			}
			if capacity := smartRecord.GetFloat("capacity"); capacity > 0 {
				device.Capacity = new(uint64)
				*device.Capacity = uint64(capacity)
			}
			if temp := smartRecord.GetFloat("temp"); temp > 0 {
				device.TempC = new(float64)
				*device.TempC = temp
			}
			if hours := smartRecord.GetFloat("hours"); hours > 0 {
				device.PowerOnHours = new(uint64)
				*device.PowerOnHours = uint64(hours)
			}
			if cycles := smartRecord.GetFloat("cycles"); cycles > 0 {
				device.PowerCycles = new(uint64)
				*device.PowerCycles = uint64(cycles)
			}
			result.Smart = append(result.Smart, device)
		}
	}
	// ZFS pool details
	if records, err := h.app.FindRecordsByFilter("zfs_pools", "system = {:system}", "name", maxListRecords, 0,
		dbx.Params{"system": record.Id}); err == nil {
		result.ZfsPools = make([]zfsPool, 0, len(records))
		for _, poolRecord := range records {
			pool := zfsPool{
				Name:        poolRecord.GetString("name"),
				DisplayName: poolRecord.GetString("display_name"),
				Health:      poolRecord.GetString("health"), // ONLINE, DEGRADED, FAULTED, ...
				Updated:     poolRecord.GetDateTime("updated").Time().UTC().Format(time.RFC3339),
			}
			for field, target := range map[string]**uint64{
				"size": &pool.Size, "alloc": &pool.Alloc, "free": &pool.Free,
			} {
				if value := poolRecord.GetFloat(field); value > 0 {
					*target = new(uint64)
					**target = uint64(value)
				}
			}
			result.ZfsPools = append(result.ZfsPools, pool)
		}
	}
	return nil, result, nil
}

type smartDevice struct {
	Name         string   `json:"name"` // device name, e.g. nvme0n1
	Model        string   `json:"model,omitempty"`
	State        string   `json:"state,omitempty"` // SMART status, e.g. PASSED
	Type         string   `json:"type,omitempty"`  // disk type, e.g. ssd
	Serial       string   `json:"serial,omitempty"`
	Firmware     string   `json:"firmware,omitempty"`
	Capacity     *uint64  `json:"capacity_bytes,omitzero"`
	TempC        *float64 `json:"temp_c,omitzero"`
	PowerOnHours *uint64  `json:"power_on_hours,omitzero"`
	PowerCycles  *uint64  `json:"power_cycles,omitzero"`
}

type zfsPool struct {
	Name        string  `json:"name"`
	DisplayName string  `json:"display_name,omitempty"`
	Health      string  `json:"health,omitempty"` // ONLINE, DEGRADED, FAULTED, ...
	Size        *uint64 `json:"size_bytes,omitzero"`
	Alloc       *uint64 `json:"alloc_bytes,omitzero"`
	Free        *uint64 `json:"free_bytes,omitzero"`
	Updated     string  `json:"updated,omitempty"` // RFC3339
}

type diskHealth struct {
	System   string        `json:"system"`
	Smart    []smartDevice `json:"smart_devices"`
	ZfsPools []zfsPool     `json:"zfs_pools"`
}

// ---------------------------------------------------------------------------
// list_network_monitors

// listNetworkMonitors returns the endpoint monitors configured for one system,
// with the latest and 1h aggregated latency/loss stored by the hub.
func (h *mcphub) listNetworkMonitors(ctx context.Context, req *mcpsdk.CallToolRequest, params getSystemParams) (*mcpsdk.CallToolResult, networkMonitorList, error) {
	auth, err := h.authRecord(ctx)
	if err != nil {
		return nil, networkMonitorList{}, err
	}
	record, err := h.resolveSystem(auth, params.System)
	if err != nil {
		return nil, networkMonitorList{}, err
	}
	records, err := h.app.FindRecordsByFilter("network_monitors", "system = {:system}", "target", maxListRecords, 0,
		dbx.Params{"system": record.Id})
	if err != nil {
		return nil, networkMonitorList{}, err
	}
	result := networkMonitorList{System: record.GetString("name"), Count: len(records),
		Monitors: make([]networkMonitor, 0, len(records))}
	for _, monitorRecord := range records {
		monitor := networkMonitor{
			Target:   monitorRecord.GetString("target"),
			Protocol: monitorRecord.GetString("protocol"), // icmp, tcp, http or dns
			Port:     uint16(monitorRecord.GetInt("port")),
			Enabled:  monitorRecord.GetBool("enabled"),
		}
		if latency := monitorRecord.GetFloat("res"); latency > 0 {
			monitor.Latency = &latency
		}
		if latency := monitorRecord.GetFloat("resAvg1h"); latency > 0 {
			monitor.LatencyAvg1h = &latency
		}
		if latency := monitorRecord.GetFloat("resMax1h"); latency > 0 {
			monitor.LatencyMax1h = &latency
		}
		if loss := monitorRecord.GetFloat("loss1h"); loss > 0 {
			monitor.Loss1h = &loss
		}
		if updated := monitorRecord.GetDateTime("updated"); !updated.IsZero() {
			monitor.Updated = updated.Time().UTC().Format(time.RFC3339)
		}
		result.Monitors = append(result.Monitors, monitor)
	}
	return nil, result, nil
}

type networkMonitor struct {
	Target       string   `json:"target"`
	Protocol     string   `json:"protocol,omitempty"` // icmp, tcp, http or dns
	Port         uint16   `json:"port,omitzero"`
	Enabled      bool     `json:"enabled"`
	Latency      *float64 `json:"latency_ms,omitzero"` // latest average response
	LatencyAvg1h *float64 `json:"latency_avg_1h_ms,omitzero"`
	LatencyMax1h *float64 `json:"latency_max_1h_ms,omitzero"`
	Loss1h       *float64 `json:"loss_pct_1h,omitzero"` // packet loss % over the last hour
	Updated      string   `json:"updated,omitempty"`    // RFC3339
}

type networkMonitorList struct {
	System   string           `json:"system"`
	Count    int              `json:"count"`
	Monitors []networkMonitor `json:"monitors"`
}
