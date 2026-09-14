package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"

	"log/slog"
)

// Match the numeric RTT independently of the localized label used by Windows.
var pingTimeRegex = regexp.MustCompile(`(?i)[=<]\s*([0-9]+(?:[.,][0-9]+)?)\s*ms\b`)

var icmpSequence atomic.Uint32

type icmpPacketConn interface {
	Close() error
}

// icmpMethod tracks which ICMP approach to use. Once a method succeeds or
// all native methods fail, the choice is cached so subsequent monitors skip
// the trial-and-error overhead.
type icmpMethod uint8

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

// monitorICMP sends an ICMP echo request and measures round-trip response.
// Supports both IPv4 and IPv6 targets. The ICMP method (raw socket,
// unprivileged datagram, or exec fallback) is detected once per address
// family and cached for subsequent monitors.
// Returns response in microseconds, or -1 and an error on failure.
func monitorICMP(ctx context.Context, target string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	family, ip, err := resolveICMPTarget(ctx, target)
	if err != nil {
		return -1, err
	}

	icmpModeMu.Lock()
	if family.mode == icmpUntried {
		family.mode = detectICMPMode(family, icmpListen)
	}
	mode := family.mode
	icmpModeMu.Unlock()

	switch mode {
	case icmpRaw:
		return monitorICMPNative(ctx, family.rawNetwork, family, &net.IPAddr{IP: ip})
	case icmpDatagram:
		return monitorICMPNative(ctx, family.dgramNetwork, family, &net.UDPAddr{IP: ip})
	case icmpExecFallback:
		return monitorICMPExec(ctx, ip.String(), family.isIPv6)
	default:
		return -1, errors.New("unsupported ICMP mode")
	}
}

// resolveICMPTarget resolves a target hostname or IP to determine the address
// family and concrete IP address. Prefers IPv4 for dual-stack hostnames.
func resolveICMPTarget(ctx context.Context, target string) (*icmpFamily, net.IP, error) {
	if ip := net.ParseIP(target); ip != nil {
		if ip.To4() != nil {
			return &icmpV4, ip.To4(), nil
		}
		return &icmpV6, ip, nil
	}

	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", target)
	if err != nil || len(ips) == 0 {
		return nil, nil, err
	}
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			return &icmpV4, v4, nil
		}
	}
	return &icmpV6, ips[0], nil
}

func detectICMPMode(family *icmpFamily, listen func(network, listenAddr string) (icmpPacketConn, error)) icmpMethod {
	label := "IPv4"
	if family.isIPv6 {
		label = "IPv6"
	}

	conn, err := listen(family.rawNetwork, family.listenAddr)
	slog.Debug("ICMP raw socket test", "family", label, "err", err)
	if err == nil {
		conn.Close()
		return icmpRaw
	}

	conn, err = listen(family.dgramNetwork, family.listenAddr)
	slog.Debug("ICMP datagram socket test", "family", label, "err", err)
	if err == nil {
		conn.Close()
		return icmpDatagram
	}

	return icmpExecFallback
}

// monitorICMPNative sends an ICMP echo request using Go's x/net/icmp package.
func monitorICMPNative(ctx context.Context, network string, family *icmpFamily, dst net.Addr) (int64, error) {
	conn, err := icmp.ListenPacket(network, family.listenAddr)
	if err != nil {
		return -1, err
	}
	defer conn.Close()

	return monitorICMPPacket(ctx, conn, family, dst)
}

func monitorICMPPacket(ctx context.Context, conn net.PacketConn, family *icmpFamily, dst net.Addr) (int64, error) {
	if err := ctx.Err(); err != nil {
		return -1, err
	}
	// Closing the socket interrupts both reads and writes on cancellation.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	// Prepare correlation data before starting the round-trip timer. The token
	// also distinguishes delayed replies after the 16-bit sequence wraps.
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return -1, err
	}
	echo := &icmp.Echo{
		ID:   os.Getpid() & 0xffff,
		Seq:  int(icmpSequence.Add(1) & 0xffff),
		Data: token,
	}
	// Linux ping sockets replace the Echo ID with their bound port. Darwin
	// datagram sockets and raw sockets preserve the supplied ID.
	if local, ok := conn.LocalAddr().(*net.UDPAddr); ok && runtime.GOOS == "linux" {
		echo.ID = local.Port
	}
	targetIP := icmpAddrIP(dst)
	msg := &icmp.Message{
		Type: family.echoType,
		Code: 0,
		Body: echo,
	}
	msgBytes, err := msg.Marshal(nil)
	if err != nil {
		return -1, err
	}

	// Set deadline before sending
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return -1, err
	}

	buf := make([]byte, 1500)
	start := time.Now()
	if _, err := conn.WriteTo(msgBytes, dst); err != nil {
		return -1, err
	}

	// Read reply
	for {
		n, peer, err := conn.ReadFrom(buf)
		received := time.Now()
		if err != nil {
			return -1, err
		}
		if !targetIP.Equal(icmpAddrIP(peer)) {
			continue
		}

		reply, err := icmp.ParseMessage(family.proto, buf[:n])
		if err != nil || reply.Type != family.replyType || reply.Code != 0 {
			continue
		}

		body, ok := reply.Body.(*icmp.Echo)
		if ok && body.ID == echo.ID && body.Seq == echo.Seq && bytes.Equal(body.Data, echo.Data) {
			return received.Sub(start).Microseconds(), nil
		}
		// Keep waiting for our reply without extending the original deadline.
	}
}

func icmpAddrIP(addr net.Addr) net.IP {
	switch addr := addr.(type) {
	case *net.IPAddr:
		return addr.IP
	case *net.UDPAddr:
		return addr.IP
	default:
		return nil
	}
}

// pingCommand selects the executable and arguments for the supported agent platforms.
// The context deadline enforces the timeout: -W has incompatible meanings across
// Linux, BSD IPv4 ping, and macOS ping6.
func pingCommand(goos, target string, isIPv6 bool) (string, []string, error) {
	family := "-4"
	if isIPv6 {
		family = "-6"
	}
	switch goos {
	case "windows":
		return "ping", []string{family, "-n", "1", "-w", "3000", target}, nil
	case "linux":
		return "ping", []string{family, "-n", "-c", "1", target}, nil
	case "darwin", "freebsd", "openbsd":
		command := "ping"
		if isIPv6 {
			command = "ping6"
		}
		return command, []string{"-n", "-c", "1", target}, nil
	default:
		return "", nil, fmt.Errorf("ping fallback is unsupported on %s", goos)
	}
}

// monitorICMPExec falls back to the system ping command. Returns -1 and an error on failure.
func monitorICMPExec(ctx context.Context, target string, isIPv6 bool) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	name, args, err := pingCommand(runtime.GOOS, target, isIPv6)
	if err != nil {
		return -1, err
	}
	cmd := exec.CommandContext(ctx, name, args...)
	// Keep Unix output and decimal formatting stable. Windows ignores LC_ALL.
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	output, err := cmd.Output()
	if ctx.Err() != nil {
		return -1, ctx.Err()
	}
	if err != nil {
		return -1, fmt.Errorf("%s failed: %w", name, err)
	}
	return parsePingResponse(output)
}

// parsePingResponse returns the reported RTT, never subprocess execution time.
// For a bounded value such as Windows' time<1ms, retain the reported upper bound.
func parsePingResponse(output []byte) (int64, error) {
	matches := pingTimeRegex.FindSubmatch(output)
	if len(matches) < 2 {
		return -1, errors.New("ping output contains no round-trip time")
	}
	ms, err := strconv.ParseFloat(strings.ReplaceAll(string(matches[1]), ",", "."), 64)
	if err != nil || math.IsInf(ms, 0) || ms >= float64(math.MaxInt64)/1000 {
		return -1, errors.New("invalid round-trip time in ping output")
	}
	return int64(math.Round(ms * 1000)), nil
}
