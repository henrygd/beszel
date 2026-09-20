package systems

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/henrygd/beszel/internal/hub/expirymap"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// RecoveryProber manages fast, low-downtime watchdogs for systems mapped to watchdog modules.
type RecoveryProber struct {
	mu        sync.RWMutex
	app       core.App
	watchdogs map[string]*systemWatchdog
	running   bool
	ctx       context.Context
	cancel    context.CancelFunc

	// locks coordinates hub-side recovery actions (automatic WOL vs. manual
	// UI actions) so at most one is ever in flight per channel at a time.
	// It intentionally does not - and cannot - coordinate with the ESP32's
	// own autonomous relay actions in real time; see ChannelLockInfo, which
	// is surfaced to the ESP via /recovery/ping as a best-effort hint.
	locks     *expirymap.ExpiryMap[lockInfo]
	nextLease uint64

	// icmpOnce/icmpNetwork cache which ICMP transport (if any) actually
	// works in this environment, detected on first use. Probes run every
	// few seconds per watchdog, so re-attempting a broken transport on every
	// call would add needless latency - see pingHost.
	icmpOnce    sync.Once
	icmpNetwork string
}

// lockInfo describes the current holder of a channel's recovery lock.
type lockInfo struct {
	Owner     string
	LeaseID   string
	ExpiresAt time.Time
}

// systemWatchdog stores in-memory state and the cancellation hook for a single system watchdog loop.
type systemWatchdog struct {
	systemID    string
	channelID   string
	hostIP      string
	threshold   int
	graceSecs   int
	maintenance bool
	wolEnabled  bool
	autoWol     bool
	macAddress  string
	bcastIP     string
	wolPort     int
	moduleID    string
	channelNum  int
	cancel      context.CancelFunc
}

// NewRecoveryProber instantiates a new watchdog manager.
func NewRecoveryProber(app core.App) *RecoveryProber {
	ctx, cancel := context.WithCancel(context.Background())
	return &RecoveryProber{
		app:       app,
		watchdogs: make(map[string]*systemWatchdog),
		ctx:       ctx,
		cancel:    cancel,
		locks:     expirymap.New[lockInfo](30 * time.Second),
	}
}

// recoverPanic stops a panic inside a background recovery goroutine from
// killing the hub. An unrecovered panic in any goroutine terminates the whole
// Go process, so a bug in best-effort recovery probing must never be able to
// take monitoring - and the web UI - down with it. Deferred argument values are
// captured at defer time, but the Sprintf only runs when a panic is in flight.
func recoverPanic(what string, args ...any) {
	if r := recover(); r != nil {
		log.Printf("beszel: %s panicked: %v\n%s", fmt.Sprintf(what, args...), r, debug.Stack())
	}
}

// acquireLock grants exclusive ownership of a channel's recovery lock to owner
// for ttl. Any existing unexpired lease - regardless of owner - blocks
// acquisition, so two hub-side actions (automatic or manual) can never run
// concurrently against the same channel, including a duplicate click of the
// same action. Returns the minted lease ID and true on success.
func (rp *RecoveryProber) acquireLock(channelID, owner string, ttl time.Duration) (string, bool) {
	if _, held := rp.locks.GetOk(channelID); held {
		return "", false
	}
	leaseID := strconv.FormatUint(atomic.AddUint64(&rp.nextLease, 1), 36)
	rp.locks.Set(channelID, lockInfo{Owner: owner, LeaseID: leaseID, ExpiresAt: time.Now().Add(ttl)}, ttl)
	return leaseID, true
}

// releaseLock clears channelID's lock only if it still matches leaseID. A
// stale release from a superseded lease (e.g. an automatic-WOL attempt that
// was cancelled after its config changed) is a safe no-op instead of
// deleting a different, newer lease acquired by another actor in the interim.
func (rp *RecoveryProber) releaseLock(channelID, leaseID string) {
	if cur, held := rp.locks.GetOk(channelID); held && cur.LeaseID == leaseID {
		rp.locks.Remove(channelID)
	}
}

// lockStatus is a read-only lookup of the current lock holder, if any.
func (rp *RecoveryProber) lockStatus(channelID string) (owner string, secondsRemaining int, held bool) {
	cur, ok := rp.locks.GetOk(channelID)
	if !ok {
		return "", 0, false
	}
	remaining := int(time.Until(cur.ExpiresAt).Seconds())
	if remaining < 0 {
		remaining = 0
	}
	return cur.Owner, remaining, true
}

// ChannelLockInfo exposes the current lock holder for a channel so it can be
// surfaced to the ESP32 module via /recovery/ping as a best-effort hint.
func (rp *RecoveryProber) ChannelLockInfo(channelID string) (owner string, secondsRemaining int, held bool) {
	return rp.lockStatus(channelID)
}

// AcquireLock is the exported entry point used by manual (admin-triggered)
// recovery actions in the API layer.
func (rp *RecoveryProber) AcquireLock(channelID, owner string, ttl time.Duration) (string, bool) {
	return rp.acquireLock(channelID, owner, ttl)
}

// ReleaseLock is the exported entry point used by manual (admin-triggered)
// recovery actions in the API layer.
func (rp *RecoveryProber) ReleaseLock(channelID, leaseID string) {
	rp.releaseLock(channelID, leaseID)
}

// Start loads current config and binds pocketbase hooks to maintain the watchdog registry.
func (rp *RecoveryProber) Start() error {
	rp.mu.Lock()
	if rp.running {
		rp.mu.Unlock()
		return nil
	}
	rp.running = true
	rp.mu.Unlock()

	// Start offline modules detection scanner
	go rp.runOfflineScanner(rp.ctx)

	// Load existing mappings
	records, err := rp.app.FindRecordsByFilter("recovery_channels", "", "", -1, 0)
	if err == nil {
		for _, rec := range records {
			rp.registerWatchdog(rec)
		}
	}

	// Register collection lifecycle hooks
	rp.app.OnRecordAfterCreateSuccess("recovery_channels").BindFunc(func(e *core.RecordEvent) error {
		rp.registerWatchdog(e.Record)
		return e.Next()
	})

	rp.app.OnRecordAfterUpdateSuccess("recovery_channels").BindFunc(func(e *core.RecordEvent) error {
		rp.registerWatchdog(e.Record)
		return e.Next()
	})

	rp.app.OnRecordAfterDeleteSuccess("recovery_channels").BindFunc(func(e *core.RecordEvent) error {
		rp.deregisterWatchdog(e.Record.Id)
		return e.Next()
	})

	return nil
}

// registerWatchdog starts or updates a background watcher goroutine for a configuration mapping.
func (rp *RecoveryProber) registerWatchdog(rec *core.Record) {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	channelID := rec.Id
	systemID := rec.GetString("system")
	hostIP := rec.GetString("host_ip")
	threshold := rec.GetInt("failure_threshold")
	graceSecs := rec.GetInt("boot_grace_seconds")
	maintenance := rec.GetBool("maintenance")
	wolEnabled := rec.GetBool("wol_enabled")
	autoWol := rec.GetBool("auto_wol")
	macAddress := rec.GetString("mac_address")
	bcastIP := rec.GetString("broadcast_address")
	wolPort := rec.GetInt("wol_port")
	moduleID := rec.GetString("module")
	channelNum := rec.GetInt("channel_number")

	if threshold <= 0 {
		threshold = 3
	}
	if graceSecs <= 0 {
		graceSecs = 60
	}
	if wolPort <= 0 {
		wolPort = 9
	}
	if bcastIP == "" {
		bcastIP = "255.255.255.255"
	}

	if old, exists := rp.watchdogs[channelID]; exists {
		old.cancel()
		delete(rp.watchdogs, channelID)
	}

	ctx, cancel := context.WithCancel(context.Background())
	w := &systemWatchdog{
		systemID:    systemID,
		channelID:   channelID,
		hostIP:      hostIP,
		threshold:   threshold,
		graceSecs:   graceSecs,
		maintenance: maintenance,
		wolEnabled:  wolEnabled,
		autoWol:     autoWol,
		macAddress:  macAddress,
		bcastIP:     bcastIP,
		wolPort:     wolPort,
		moduleID:    moduleID,
		channelNum:  channelNum,
		cancel:      cancel,
	}

	rp.watchdogs[channelID] = w

	go rp.runWatchdog(ctx, w)
}

// deregisterWatchdog cancels the context of a watchdog loop, stopping the checking loop.
func (rp *RecoveryProber) deregisterWatchdog(channelID string) {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	if w, exists := rp.watchdogs[channelID]; exists {
		w.cancel()
		delete(rp.watchdogs, channelID)
	}
}

// Stop cancels all background work owned by the RecoveryProber: the
// offline-module scanner, the recovery-lock expiry map's cleaner goroutine,
// and every per-channel watchdog goroutine. Intended for test cleanup
// (mirrors AlertManager.Stop()) - production shutdown just exits the
// process, so this isn't wired into normal hub startup/shutdown. Safe to
// call multiple times.
func (rp *RecoveryProber) Stop() {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	for channelID, w := range rp.watchdogs {
		w.cancel()
		delete(rp.watchdogs, channelID)
	}
	rp.locks.StopCleaner()
	rp.cancel()
}

// runWatchdog runs the checking state machine loop.
func (rp *RecoveryProber) runWatchdog(ctx context.Context, w *systemWatchdog) {
	defer recoverPanic("recovery watchdog (system=%s channel=%s)", w.systemID, w.channelID)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if w.maintenance {
				continue
			}

			// Perform normal 5s check
			online := rp.pingHost(w.hostIP, 2*time.Second)
			if !online {
				// Transition to FAST_VERIFY state
				rp.logEvent(w.systemID, w.channelID, "FAST_VERIFY_STARTED", map[string]any{
					"reason": "first normal probe failed",
				})

				success := false
				// Perform verification attempts (2 seconds apart)
				for i := 0; i < w.threshold; i++ {
					select {
					case <-ctx.Done():
						return
					case <-time.After(2 * time.Second):
						if rp.pingHost(w.hostIP, 2*time.Second) {
							success = true
							break
						}
					}
					if success {
						break
					}
				}

				if success {
					rp.logEvent(w.systemID, w.channelID, "FAST_VERIFY_RECOVERED", map[string]any{
						"status": "online",
					})
					continue
				}

				// All verify checks failed. Verify network gateway status.
				gatewayIP := rp.getGatewayIP(w.systemID)
				if gatewayIP != "" {
					gatewayOK := rp.probeGateway(gatewayIP)
					rp.updateGatewayOnlineStatus(w.systemID, gatewayOK)
					if !gatewayOK {
						// Gateway itself is down. Classify as network issue.
						rp.logEvent(w.systemID, w.channelID, "NETWORK_FAILURE", map[string]any{
							"gateway": gatewayIP,
							"reason":  "gateway port probe failed",
						})
						continue
					}
				}

				// Gateway is online. Server down is officially classified.
				rp.logEvent(w.systemID, w.channelID, "FAILURE_CONFIRMED", map[string]any{
					"reason": fmt.Sprintf("failed %d verify checks", w.threshold),
				})

				// If WOL is enabled and automatic, attempt it. attemptAutomaticWOL
				// is lock-protected so it can never run concurrently with a
				// manual WOL/relay action (or a duplicate automatic attempt)
				// on the same channel.
				recoverySuccessful := false
				if w.wolEnabled && w.autoWol && w.macAddress != "" {
					recoverySuccessful = rp.attemptAutomaticWOL(ctx, w)
				}

				// Beszel must not directly control a relay (see
				// BESZEL_ESP32_HARDWARE_RECOVERY_BRIEF.md §9 and the final
				// architecture note: "only the watchdog can verify and
				// authorize an automatic physical recovery"). If WOL did not
				// succeed (or was skipped), physical recovery is the ESP32
				// module's own job - it runs the same verify/escalate ladder
				// locally and independently, without needing (or waiting for)
				// an HTTP nudge from the hub. Log this so the recovery
				// timeline doesn't go silent at this point.
				if !recoverySuccessful && w.moduleID != "" {
					rp.logEvent(w.systemID, w.channelID, "ESP_AUTONOMOUS_EXPECTED", map[string]any{
						"module":  w.moduleID,
						"channel": w.channelNum,
						"reason":  "WOL did not succeed; physical recovery is owned by the ESP32 module's independent watchdog, not triggered automatically by the hub",
					})
				}
			}
			if ctx.Err() != nil {
				return
			}
		}
	}
}

// attemptAutomaticWOL sends a Wake-on-LAN magic packet for w and waits up to
// w.graceSecs for the host to respond. It acquires the channel's recovery
// lock first so it can never race a manual WOL/relay action (or a duplicate
// automatic attempt) on the same channel; if the lock is already held it
// logs WOL_BLOCKED_LOCK and returns false immediately instead of proceeding.
//
// The lease is released via defer, scoping its lifetime to this single
// attempt rather than the long-lived watchdog goroutine. If ctx is
// cancelled mid-wait (e.g. because the channel's config just changed and
// registerWatchdog cancelled this goroutine to start a fresh one), this
// function returns promptly - within the wait loop's ~1s tick granularity -
// and the deferred, lease-ID-checked release frees the lock immediately.
// That is what makes toggling wol_enabled/auto_wol off take effect right
// away, without needing a separate, racier force-release call elsewhere.
func (rp *RecoveryProber) attemptAutomaticWOL(ctx context.Context, w *systemWatchdog) bool {
	leaseID, acquired := rp.acquireLock(w.channelID, "BESZEL_WOL", time.Duration(w.graceSecs+10)*time.Second)
	if !acquired {
		rp.logEvent(w.systemID, w.channelID, "WOL_BLOCKED_LOCK", map[string]any{
			"reason": "another recovery action is already in progress for this channel",
		})
		return false
	}
	defer rp.releaseLock(w.channelID, leaseID)

	rp.logEvent(w.systemID, w.channelID, "WOL_SENT", map[string]any{
		"mac":       w.macAddress,
		"broadcast": w.bcastIP,
		"port":      w.wolPort,
	})

	if err := SendMagicPacket(w.macAddress, w.bcastIP, w.wolPort); err != nil {
		rp.logEvent(w.systemID, w.channelID, "WOL_ERROR", map[string]any{
			"error": err.Error(),
		})
	}

	// Wait for boot grace period
	graceTicker := time.NewTicker(1 * time.Second)
	defer graceTicker.Stop()
	graceTimeout := time.After(time.Duration(w.graceSecs) * time.Second)

	for {
		select {
		case <-ctx.Done():
			return false
		case <-graceTimeout:
			rp.logEvent(w.systemID, w.channelID, "WOL_FAILED", map[string]any{
				"reason": "boot grace period timed out",
			})
			return false
		case <-graceTicker.C:
			if rp.pingHost(w.hostIP, 2*time.Second) {
				rp.logEvent(w.systemID, w.channelID, "WOL_SUCCESS", map[string]any{
					"status": "online",
				})
				return true
			}
		}
	}
}

// probePorts dials TCP ports to check if host is responsive. Retained only
// as pingHost's last-resort fallback for environments where neither a
// privileged nor unprivileged ICMP socket can be opened (e.g. a container
// runtime without NET_RAW and a locked-down ping_group_range).
func (rp *RecoveryProber) probePorts(host string, ports []int) bool {
	for _, port := range ports {
		address := net.JoinHostPort(host, fmt.Sprintf("%d", port))
		conn, err := net.DialTimeout("tcp", address, 2*time.Second)
		if err == nil {
			conn.Close()
			return true
		}
	}
	return false
}

// pingHost checks host reachability with an ICMP echo request rather than
// probing any particular port. If no ICMP transport is available in this
// environment, falls back to a TCP dial on port 22 so recovery still
// functions, just less precisely.
func (rp *RecoveryProber) pingHost(host string, timeout time.Duration) bool {
	if ok, isTransportErr := rp.icmpPing(host, timeout); !isTransportErr {
		return ok
	}
	return rp.probePorts(host, []int{22})
}

// icmpPing sends an ICMP echo request to host using this process's detected
// transport. The working transport - a privileged raw socket ("ip4:icmp",
// needs NET_RAW/root) or an unprivileged datagram socket ("udp4", needs a
// permissive ping_group_range) - is detected once and cached, since
// retrying a broken mode on every 5s probe would add needless latency.
// Returns (reachable, transportUnavailable): callers use the second value to
// decide whether to fall back to a different check.
func (rp *RecoveryProber) icmpPing(host string, timeout time.Duration) (ok bool, transportUnavailable bool) {
	rp.icmpOnce.Do(func() {
		rp.icmpNetwork = detectICMPNetwork()
	})
	if rp.icmpNetwork == "" {
		return false, true
	}
	reached, err := icmpEcho(rp.icmpNetwork, host, timeout)
	if err != nil {
		// Transport broke after previously working (e.g. a capability
		// change mid-run) - let the caller fall back instead of treating
		// this host as unreachable outright.
		return false, true
	}
	return reached, false
}

// detectICMPNetwork returns the first ICMP transport this process can open
// a socket for, or "" if neither is available.
func detectICMPNetwork() string {
	for _, network := range []string{"ip4:icmp", "udp4"} {
		conn, err := icmp.ListenPacket(network, "0.0.0.0")
		if err == nil {
			conn.Close()
			return network
		}
	}
	return ""
}

// icmpEcho sends a single ICMP echo request to host over network ("ip4:icmp"
// or "udp4") and reports whether a matching reply arrived within timeout.
// The returned error indicates the ICMP transport itself failed (socket
// open/write/parse), not a plain timeout - a non-responding host is a normal
// (false, nil) result, not an error.
func icmpEcho(network, host string, timeout time.Duration) (bool, error) {
	conn, err := icmp.ListenPacket(network, "0.0.0.0")
	if err != nil {
		return false, err
	}
	defer conn.Close()

	dst, err := net.ResolveIPAddr("ip4", host)
	if err != nil {
		return false, err
	}

	id := os.Getpid() & 0xffff
	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID:   id,
			Seq:  int(time.Now().UnixNano() & 0xffff),
			Data: []byte("beszel-recovery-ping"),
		},
	}
	wb, err := msg.Marshal(nil)
	if err != nil {
		return false, err
	}

	// The "udp4" ping-socket transport addresses by UDPAddr; the kernel
	// demultiplexes replies to it by the ephemeral port it assigned, so no
	// separate ID check is needed for that mode (see the read loop below).
	var writeAddr net.Addr = dst
	if network == "udp4" {
		writeAddr = &net.UDPAddr{IP: dst.IP}
	}
	if _, err := conn.WriteTo(wb, writeAddr); err != nil {
		return false, err
	}
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return false, err
	}

	rb := make([]byte, 1500)
	for {
		n, peer, err := conn.ReadFrom(rb)
		if err != nil {
			// Read timeout (or a transient read error) - treat as "no
			// reply" rather than a transport failure.
			return false, nil
		}
		rm, err := icmp.ParseMessage(1, rb[:n]) // 1 = ICMPv4 protocol number
		if err != nil {
			continue
		}
		if rm.Type != ipv4.ICMPTypeEchoReply {
			continue
		}
		body, ok := rm.Body.(*icmp.Echo)
		if !ok {
			continue
		}
		if network == "ip4:icmp" && body.ID != id {
			continue
		}
		peerIP, ok := addrIP(peer)
		if ok && !peerIP.Equal(dst.IP) {
			continue
		}
		return true, nil
	}
}

// addrIP extracts the IP from the net.Addr types icmp.PacketConn.ReadFrom
// can return, depending on transport ("ip4:icmp" -> *net.IPAddr, "udp4" ->
// *net.UDPAddr).
func addrIP(addr net.Addr) (net.IP, bool) {
	switch a := addr.(type) {
	case *net.IPAddr:
		return a.IP, true
	case *net.UDPAddr:
		return a.IP, true
	default:
		return nil, false
	}
}

// probeGateway checks gateway reachability with an ICMP ping first, falling
// back to dialing DNS port 53 or HTTP port 80 if ICMP is unavailable.
func (rp *RecoveryProber) probeGateway(gatewayIP string) bool {
	if ok, transportUnavailable := rp.icmpPing(gatewayIP, 1500*time.Millisecond); !transportUnavailable {
		return ok
	}
	address := net.JoinHostPort(gatewayIP, "53")
	conn, err := net.DialTimeout("tcp", address, 1500*time.Millisecond)
	if err == nil {
		conn.Close()
		return true
	}
	address80 := net.JoinHostPort(gatewayIP, "80")
	conn80, err := net.DialTimeout("tcp", address80, 1500*time.Millisecond)
	if err == nil {
		conn80.Close()
		return true
	}
	return false
}

// channelBySystem looks up the recovery channel mapped to systemID.
//
// It resolves the collection explicitly instead of passing the name straight
// to FindFirstRecordByFilter. PocketBase's RecordQuery dereferences the
// collection with no nil check (core/record_query.go:38 in v0.36.8), and
// resolving "recovery_channels" by name through the cached-collection path can
// hand back a nil collection without an accompanying error - which segfaults
// the query builder instead of returning something we can handle. Passing a
// *Collection takes the direct branch and skips that lookup entirely.
func (rp *RecoveryProber) channelBySystem(systemID string) (*core.Record, bool) {
	if rp == nil || rp.app == nil || systemID == "" {
		return nil, false
	}
	collection, err := rp.app.FindCollectionByNameOrId("recovery_channels")
	if err != nil || collection == nil {
		return nil, false
	}
	rec, err := rp.app.FindFirstRecordByFilter(collection, "system = {:system}", dbx.Params{"system": systemID})
	if err != nil || rec == nil {
		return nil, false
	}
	return rec, true
}

// getGatewayIP queries database configuration to retrieve the gateway IP for a system ID.
func (rp *RecoveryProber) getGatewayIP(systemID string) string {
	chanRec, ok := rp.channelBySystem(systemID)
	if !ok {
		return ""
	}
	moduleID := chanRec.GetString("module")
	if moduleID == "" {
		return ""
	}
	moduleRec, err := rp.app.FindRecordById("recovery_modules", moduleID)
	if err != nil {
		return ""
	}
	return moduleRec.GetString("gateway_ip")
}

// updateGatewayOnlineStatus persists whether the gateway is currently
// reachable on the recovery_modules record mapped to systemID's channel, so
// the health score and frontend can show gateway status without a separate
// always-on polling loop - this only runs when a channel's fast-verify has
// already failed and gateway health needs checking anyway.
func (rp *RecoveryProber) updateGatewayOnlineStatus(systemID string, online bool) {
	chanRec, ok := rp.channelBySystem(systemID)
	if !ok {
		return
	}
	moduleID := chanRec.GetString("module")
	if moduleID == "" {
		return
	}
	moduleRec, err := rp.app.FindRecordById("recovery_modules", moduleID)
	if err != nil {
		return
	}
	if moduleRec.GetBool("gateway_online") == online {
		return
	}
	moduleRec.Set("gateway_online", online)
	_ = rp.app.Save(moduleRec)
}

// logEvent creates an audit trail entry in recovery_events.
func (rp *RecoveryProber) logEvent(systemID, channelID, event string, metadata map[string]any) {
	collection, err := rp.app.FindCollectionByNameOrId("recovery_events")
	if err != nil {
		return
	}

	var moduleID string
	var channelNum int
	if channelID != "" {
		if rec, err := rp.app.FindRecordById("recovery_channels", channelID); err == nil {
			moduleID = rec.GetString("module")
			channelNum = rec.GetInt("channel_number")
		}
	}

	record := core.NewRecord(collection)
	record.Set("system", systemID)
	if moduleID != "" {
		record.Set("module", moduleID)
		record.Set("channel", channelNum)
	}
	record.Set("event", event)
	record.Set("timestamp", time.Now().UTC())

	metaJSON, _ := json.Marshal(metadata)
	record.Set("metadata", string(metaJSON))

	_ = rp.app.Save(record)
}

// moduleIsLive reports whether a recovery_modules record has pinged
// recently enough to be considered online: within max(90s, 3x its
// configured ping interval) of its dedicated last_ping heartbeat. Falls
// back to the `updated` autodate for records saved before that field
// existed. Mirrors isRecoveryModuleOnline in internal/hub/api.go - kept as
// a separate copy here since this package can't import internal/hub
// (internal/hub already imports internal/hub/systems).
func moduleIsLive(rec *core.Record) bool {
	lastPing := rec.GetDateTime("last_ping")
	if lastPing.IsZero() {
		lastPing = rec.GetDateTime("updated")
		if lastPing.IsZero() {
			return false
		}
	}
	staleAfter := 90 * time.Second
	if interval := rec.GetInt("ping_interval_seconds"); interval > 0 {
		if threshold := 3 * time.Duration(interval) * time.Second; threshold > staleAfter {
			staleAfter = threshold
		}
	}
	return time.Since(lastPing.Time()) < staleAfter
}

// runOfflineScanner runs a periodic check to transition recovery modules to offline if pings stop.
func (rp *RecoveryProber) runOfflineScanner(ctx context.Context) {
	defer recoverPanic("recovery offline scanner")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			records, err := rp.app.FindRecordsByFilter("recovery_modules", "status != 'unapproved'", "", -1, 0)
			if err != nil {
				continue
			}
			for _, rec := range records {
				status := rec.GetString("status")
				if !moduleIsLive(rec) {
					if status != "offline" {
						rec.Set("status", "offline")
						_ = rp.app.Save(rec)
						rp.logEvent("", "", "MODULE_OFFLINE", map[string]any{
							"module": rec.Id,
							"name":   rec.GetString("name"),
							"mac":    rec.GetString("mac_address"),
						})
					}
				} else {
					if status == "offline" || status == "" {
						rec.Set("status", "online")
						_ = rp.app.Save(rec)
						rp.logEvent("", "", "MODULE_ONLINE", map[string]any{
							"module": rec.Id,
							"name":   rec.GetString("name"),
							"mac":    rec.GetString("mac_address"),
						})
					}
				}
			}
		}
	}
}

// triggerESP32Relay makes a POST dispatch call to the ESP32 module relay API endpoint.
func (rp *RecoveryProber) triggerESP32Relay(espIP string, channelNum, pulseMs int) error {
	payload := map[string]any{
		"channel":           channelNum,
		"pulse_duration_ms": pulseMs,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("http://%s/api/relay/trigger", espIP)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}
