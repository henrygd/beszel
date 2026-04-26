package agent

import (
	"math"
	"net"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"

	"log/slog"
)

var pingTimeRegex = regexp.MustCompile(`time[=<]([\d.]+)\s*ms`)

type icmpPacketConn interface {
	Close() error
}

// icmpMethod tracks which ICMP approach to use. Once a method succeeds or
// all native methods fail, the choice is cached so subsequent probes skip
// the trial-and-error overhead.
type icmpMethod int

const (
	icmpUntried      icmpMethod = iota // haven't tried yet
	icmpRaw                            // privileged raw socket
	icmpDatagram                       // unprivileged datagram socket
	icmpExecFallback                   // shell out to system ping command
)

// icmpFamily holds the network parameters and cached detection result for one address family.
type icmpFamily struct {
	rawNetwork   string    // e.g. "ip4:icmp" or "ip6:ipv6-icmp"
	dgramNetwork string    // e.g. "udp4" or "udp6"
	listenAddr   string    // "0.0.0.0" or "::"
	echoType     icmp.Type // outgoing echo request type
	replyType    icmp.Type // expected echo reply type
	proto        int       // IANA protocol number for parsing replies
	isIPv6       bool
	mode         icmpMethod // cached detection result (guarded by icmpModeMu)
}

var (
	icmpV4 = icmpFamily{
		rawNetwork:   "ip4:icmp",
		dgramNetwork: "udp4",
		listenAddr:   "0.0.0.0",
		echoType:     ipv4.ICMPTypeEcho,
		replyType:    ipv4.ICMPTypeEchoReply,
		proto:        1,
	}
	icmpV6 = icmpFamily{
		rawNetwork:   "ip6:ipv6-icmp",
		dgramNetwork: "udp6",
		listenAddr:   "::",
		echoType:     ipv6.ICMPTypeEchoRequest,
		replyType:    ipv6.ICMPTypeEchoReply,
		proto:        58,
		isIPv6:       true,
	}
	icmpModeMu sync.Mutex
	icmpListen = func(network, listenAddr string) (icmpPacketConn, error) {
		return icmp.ListenPacket(network, listenAddr)
	}
)

// probeICMP sends an ICMP echo request and measures round-trip response.
// Supports both IPv4 and IPv6 targets. The ICMP method (raw socket,
// unprivileged datagram, or exec fallback) is detected once per address
// family and cached for subsequent probes.
// Returns response in microseconds, or -1 on failure.
func probeICMP(target string) int64 {
	family, ip := resolveICMPTarget(target)
	if family == nil {
		return -1
	}

	icmpModeMu.Lock()
	if family.mode == icmpUntried {
		family.mode = detectICMPMode(family, icmpListen)
	}
	mode := family.mode
	icmpModeMu.Unlock()

	switch mode {
	case icmpRaw:
		return probeICMPNative(family.rawNetwork, family, &net.IPAddr{IP: ip})
	case icmpDatagram:
		return probeICMPNative(family.dgramNetwork, family, &net.UDPAddr{IP: ip})
	case icmpExecFallback:
		return probeICMPExec(target, family.isIPv6)
	default:
		return -1
	}
}

// resolveICMPTarget resolves a target hostname or IP to determine the address
// family and concrete IP address. Prefers IPv4 for dual-stack hostnames.
func resolveICMPTarget(target string) (*icmpFamily, net.IP) {
	if ip := net.ParseIP(target); ip != nil {
		if ip.To4() != nil {
			return &icmpV4, ip.To4()
		}
		return &icmpV6, ip
	}

	ips, err := net.LookupIP(target)
	if err != nil || len(ips) == 0 {
		return nil, nil
	}
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			return &icmpV4, v4
		}
	}
	return &icmpV6, ips[0]
}

func detectICMPMode(family *icmpFamily, listen func(network, listenAddr string) (icmpPacketConn, error)) icmpMethod {
	label := "IPv4"
	if family.isIPv6 {
		label = "IPv6"
	}

	if conn, err := listen(family.rawNetwork, family.listenAddr); err == nil {
		conn.Close()
		slog.Info("ICMP probe using raw socket", "family", label)
		return icmpRaw
	} else {
		slog.Debug("ICMP raw socket unavailable", "family", label, "err", err)
	}

	if conn, err := listen(family.dgramNetwork, family.listenAddr); err == nil {
		conn.Close()
		slog.Info("ICMP probe using unprivileged datagram socket", "family", label)
		return icmpDatagram
	} else {
		slog.Debug("ICMP datagram socket unavailable", "family", label, "err", err)
	}

	slog.Info("ICMP probe falling back to system ping command", "family", label)
	return icmpExecFallback
}

// probeICMPNative sends an ICMP echo request using Go's x/net/icmp package.
func probeICMPNative(network string, family *icmpFamily, dst net.Addr) int64 {
	conn, err := icmp.ListenPacket(network, family.listenAddr)
	if err != nil {
		return -1
	}
	defer conn.Close()

	// Build ICMP echo request
	msg := &icmp.Message{
		Type: family.echoType,
		Code: 0,
		Body: &icmp.Echo{
			ID:   os.Getpid() & 0xffff,
			Seq:  1,
			Data: []byte("beszel-probe"),
		},
	}
	msgBytes, err := msg.Marshal(nil)
	if err != nil {
		return -1
	}

	// Set deadline before sending
	conn.SetDeadline(time.Now().Add(3 * time.Second))

	start := time.Now()
	if _, err := conn.WriteTo(msgBytes, dst); err != nil {
		return -1
	}

	// Read reply
	buf := make([]byte, 1500)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			return -1
		}

		reply, err := icmp.ParseMessage(family.proto, buf[:n])
		if err != nil {
			return -1
		}

		if reply.Type == family.replyType {
			return time.Since(start).Microseconds()
		}
		// Ignore non-echo-reply messages (e.g. destination unreachable) and keep reading
	}
}

// probeICMPExec falls back to the system ping command. Returns -1 on failure.
func probeICMPExec(target string, isIPv6 bool) int64 {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		if isIPv6 {
			cmd = exec.Command("ping", "-6", "-n", "1", "-w", "3000", target)
		} else {
			cmd = exec.Command("ping", "-n", "1", "-w", "3000", target)
		}
	default: // linux, darwin, freebsd
		if isIPv6 {
			cmd = exec.Command("ping", "-6", "-c", "1", "-W", "3", target)
		} else {
			cmd = exec.Command("ping", "-c", "1", "-W", "3", target)
		}
	}

	start := time.Now()
	output, err := cmd.Output()
	if err != nil {
		// If ping fails but we got output, still try to parse
		if len(output) == 0 {
			return -1
		}
	}

	matches := pingTimeRegex.FindSubmatch(output)
	if len(matches) >= 2 {
		if ms, err := strconv.ParseFloat(string(matches[1]), 64); err == nil {
			return int64(math.Round(ms * 1000))
		}
	}

	// Fallback: use wall clock time if ping succeeded but parsing failed
	if err == nil {
		return time.Since(start).Microseconds()
	}
	return -1
}
