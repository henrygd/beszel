package agent

import (
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

	"log/slog"
)

var pingTimeRegex = regexp.MustCompile(`time[=<]([\d.]+)\s*ms`)

// icmpMethod tracks which ICMP approach to use. Once a method succeeds or
// all native methods fail, the choice is cached so subsequent probes skip
// the trial-and-error overhead.
type icmpMethod int

const (
	icmpUntried      icmpMethod = iota // haven't tried yet
	icmpRaw                            // privileged raw socket (ip4:icmp)
	icmpDatagram                       // unprivileged datagram socket (udp4)
	icmpExecFallback                   // shell out to system ping command
)

var (
	icmpMode   icmpMethod
	icmpModeMu sync.Mutex
)

// probeICMP sends an ICMP echo request and measures round-trip latency.
// It tries native raw socket first, then unprivileged datagram socket,
// and falls back to the system ping command if both fail.
// Returns latency in milliseconds, or -1 on failure.
func probeICMP(target string) float64 {
	icmpModeMu.Lock()
	mode := icmpMode
	icmpModeMu.Unlock()

	switch mode {
	case icmpRaw:
		return probeICMPNative("ip4:icmp", "0.0.0.0", target)
	case icmpDatagram:
		return probeICMPNative("udp4", "0.0.0.0", target)
	case icmpExecFallback:
		return probeICMPExec(target)
	default:
		// First call — probe which method works
		return probeICMPDetect(target)
	}
}

// probeICMPDetect tries each ICMP method in order and caches the first
// one that succeeds.
func probeICMPDetect(target string) float64 {
	// 1. Try privileged raw socket
	if ms := probeICMPNative("ip4:icmp", "0.0.0.0", target); ms >= 0 {
		icmpModeMu.Lock()
		icmpMode = icmpRaw
		icmpModeMu.Unlock()
		slog.Info("ICMP probe using raw socket")
		return ms
	}

	// 2. Try unprivileged datagram socket (Linux/macOS)
	if ms := probeICMPNative("udp4", "0.0.0.0", target); ms >= 0 {
		icmpModeMu.Lock()
		icmpMode = icmpDatagram
		icmpModeMu.Unlock()
		slog.Info("ICMP probe using unprivileged datagram socket")
		return ms
	}

	// 3. Fall back to system ping command
	slog.Info("ICMP probe falling back to system ping command")
	icmpModeMu.Lock()
	icmpMode = icmpExecFallback
	icmpModeMu.Unlock()
	return probeICMPExec(target)
}

// probeICMPNative sends an ICMP echo request using Go's x/net/icmp package.
// network is "ip4:icmp" for raw sockets or "udp4" for unprivileged datagram sockets.
func probeICMPNative(network, listenAddr, target string) float64 {
	// Resolve the target to an IP address
	dst, err := net.ResolveIPAddr("ip4", target)
	if err != nil {
		return -1
	}

	conn, err := icmp.ListenPacket(network, listenAddr)
	if err != nil {
		return -1
	}
	defer conn.Close()

	// Build ICMP echo request
	msg := &icmp.Message{
		Type: ipv4.ICMPTypeEcho,
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

		// Parse the ICMP protocol number based on network type
		proto := 1 // ICMPv4
		reply, err := icmp.ParseMessage(proto, buf[:n])
		if err != nil {
			return -1
		}

		if reply.Type == ipv4.ICMPTypeEchoReply {
			return float64(time.Since(start).Microseconds()) / 1000.0
		}
		// Ignore non-echo-reply messages (e.g. destination unreachable) and keep reading
	}
}

// probeICMPExec falls back to the system ping command. Returns -1 on failure.
func probeICMPExec(target string) float64 {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("ping", "-n", "1", "-w", "3000", target)
	default: // linux, darwin, freebsd
		cmd = exec.Command("ping", "-c", "1", "-W", "3", target)
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
			return ms
		}
	}

	// Fallback: use wall clock time if ping succeeded but parsing failed
	if err == nil {
		return float64(time.Since(start).Microseconds()) / 1000.0
	}
	return -1
}
