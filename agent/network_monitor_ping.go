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

// Each ICMP probe sends a burst of echo requests like `ping -c 5`, so a single
// dropped packet counts as partial loss. Variables so tests can shorten them.
var (
	icmpPingCount    = 5
	icmpPingSpacing  = time.Second
	icmpReplyTimeout = 3 * time.Second
)

// icmpBurstDuration is the time from the first request until the reply
// deadline of the last one.
func icmpBurstDuration() time.Duration {
	return time.Duration(icmpPingCount-1)*icmpPingSpacing + icmpReplyTimeout
}

// icmpProbeTimeout bounds a whole burst, leaving slack beyond the last reply
// deadline so the burst normally ends on its own.
func icmpProbeTimeout() time.Duration {
	return icmpBurstDuration() + time.Second
}

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

// monitorICMP sends a burst of ICMP echo requests and measures each round trip.
// Supports both IPv4 and IPv6 targets. The ICMP method (raw socket,
// unprivileged datagram, or exec fallback) is detected once per address
// family and cached for subsequent monitors.
// Returns one response in microseconds per request, with -1 for each lost
// request, and an error if no reply was received.
func monitorICMP(ctx context.Context, target string) ([]int64, error) {
	ctx, cancel := context.WithTimeout(ctx, icmpProbeTimeout())
	defer cancel()

	family, ip, err := resolveICMPTarget(ctx, target)
	if err != nil {
		return nil, err
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
		return nil, errors.New("unsupported ICMP mode")
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

// monitorICMPNative sends ICMP echo requests using Go's x/net/icmp package.
func monitorICMPNative(ctx context.Context, network string, family *icmpFamily, dst net.Addr) ([]int64, error) {
	conn, err := icmp.ListenPacket(network, family.listenAddr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	return monitorICMPPacket(ctx, conn, family, dst)
}

// monitorICMPPacket sends icmpPingCount echo requests icmpPingSpacing apart and
// waits up to icmpReplyTimeout after the last one. Replies are read between
// sends, so a single goroutine owns the socket.
func monitorICMPPacket(ctx context.Context, conn net.PacketConn, family *icmpFamily, dst net.Addr) ([]int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Closing the socket interrupts both reads and writes on cancellation.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	// Prepare correlation data before starting the round-trip timers. The token
	// also distinguishes delayed replies after the 16-bit sequence wraps.
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return nil, err
	}
	id := os.Getpid() & 0xffff
	// Linux ping sockets replace the Echo ID with their bound port. Darwin
	// datagram sockets and raw sockets preserve the supplied ID.
	if local, ok := conn.LocalAddr().(*net.UDPAddr); ok && runtime.GOOS == "linux" {
		id = local.Port
	}
	targetIP := icmpAddrIP(dst)

	count := icmpPingCount
	responses := make([]int64, count)
	seqs := make([]int, count)
	sentAt := make([]time.Time, count)
	for i := range responses {
		responses[i] = -1
	}
	sent, received := 0, 0
	var lastErr error
	var start, replyDeadline time.Time
	buf := make([]byte, 1500)

	for {
		if sent < count && !time.Now().Before(start.Add(time.Duration(sent)*icmpPingSpacing)) {
			seqs[sent] = int(icmpSequence.Add(1) & 0xffff)
			msg := &icmp.Message{
				Type: family.echoType,
				Code: 0,
				Body: &icmp.Echo{ID: id, Seq: seqs[sent], Data: token},
			}
			msgBytes, err := msg.Marshal(nil)
			if err != nil {
				return nil, err
			}
			now := time.Now()
			if sent == 0 {
				start = now
			}
			if _, err := conn.WriteTo(msgBytes, dst); err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				// A failed send counts as loss; keep the burst going.
				lastErr = err
			} else {
				sentAt[sent] = now
			}
			sent++
			if sent == count {
				replyDeadline = now.Add(icmpReplyTimeout)
			}
			continue
		}
		if sent == count && received == count {
			break
		}

		// Wait for replies until the next send is due or, after the last send,
		// until the reply deadline. Retrying never extends that deadline.
		deadline := replyDeadline
		if sent < count {
			deadline = start.Add(time.Duration(sent) * icmpPingSpacing)
		}
		if err := conn.SetReadDeadline(deadline); err != nil {
			return nil, err
		}
		n, peer, err := conn.ReadFrom(buf)
		receivedAt := time.Now()
		if err != nil {
			if !errors.Is(err, os.ErrDeadlineExceeded) {
				return nil, err
			}
			if sent == count {
				lastErr = err
				break
			}
			continue
		}
		if !targetIP.Equal(icmpAddrIP(peer)) {
			continue
		}

		reply, err := icmp.ParseMessage(family.proto, buf[:n])
		if err != nil || reply.Type != family.replyType || reply.Code != 0 {
			continue
		}
		body, ok := reply.Body.(*icmp.Echo)
		if !ok || body.ID != id || !bytes.Equal(body.Data, token) {
			continue
		}
		for i := range sent {
			if seqs[i] == body.Seq && responses[i] < 0 && !sentAt[i].IsZero() {
				responses[i] = receivedAt.Sub(sentAt[i]).Microseconds()
				received++
				break
			}
		}
	}

	if received == 0 {
		return responses, lastErr
	}
	return responses, nil
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
// Linux, BSD IPv4 ping, and macOS ping6. Linux gets -w so ping exits on its own
// and still prints its replies.
func pingCommand(goos, target string, isIPv6 bool) (string, []string, error) {
	count := strconv.Itoa(icmpPingCount)
	family := "-4"
	if isIPv6 {
		family = "-6"
	}
	switch goos {
	case "windows":
		return "ping", []string{family, "-n", count, "-w", strconv.FormatInt(icmpReplyTimeout.Milliseconds(), 10), target}, nil
	case "linux":
		// -w is an overall deadline for both iputils and busybox ping.
		return "ping", []string{family, "-n", "-c", count, "-w", strconv.Itoa(max(int(math.Ceil(icmpBurstDuration().Seconds())), 1)), target}, nil
	case "darwin", "freebsd", "openbsd":
		command := "ping"
		if isIPv6 {
			command = "ping6"
		}
		return command, []string{"-n", "-c", count, target}, nil
	default:
		return "", nil, fmt.Errorf("ping fallback is unsupported on %s", goos)
	}
}

// monitorICMPExec falls back to the system ping command. Returns one response
// per request, with -1 for each lost request, and an error if no reply was found.
func monitorICMPExec(ctx context.Context, target string, isIPv6 bool) ([]int64, error) {
	ctx, cancel := context.WithTimeout(ctx, icmpProbeTimeout())
	defer cancel()
	name, args, err := pingCommand(runtime.GOOS, target, isIPv6)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, name, args...)
	// Keep Unix output and decimal formatting stable. Windows ignores LC_ALL.
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	// Output keeps what ping printed even when it exits non-zero on partial loss
	// or is killed at the deadline, so replies received so far still count.
	output, err := cmd.Output()
	responses, parseErr := parsePingResponses(output, icmpPingCount)
	if parseErr == nil {
		return responses, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w", name, err)
	}
	return nil, parseErr
}

// parsePingResponses returns the reported RTTs, never subprocess execution time,
// padded with -1 up to count for lost requests. For a bounded value such as
// Windows' time<1ms, retain the reported upper bound. Only lines with a single
// RTT are replies: summary lines report several (Windows' Minimum/Maximum/Average)
// or none that match (Unix min/avg/max). Duplicate replies are ignored.
func parsePingResponses(output []byte, count int) ([]int64, error) {
	responses := make([]int64, 0, count)
	for line := range bytes.Lines(output) {
		if len(responses) == count {
			break
		}
		matches := pingTimeRegex.FindAllSubmatch(line, 2)
		if len(matches) != 1 || bytes.Contains(line, []byte("DUP!")) {
			continue
		}
		ms, err := strconv.ParseFloat(strings.ReplaceAll(string(matches[0][1]), ",", "."), 64)
		if err != nil || math.IsInf(ms, 0) || ms >= float64(math.MaxInt64)/1000 {
			return nil, errors.New("invalid round-trip time in ping output")
		}
		responses = append(responses, int64(math.Round(ms*1000)))
	}
	if len(responses) == 0 {
		return nil, errors.New("ping output contains no round-trip time")
	}
	for len(responses) < count {
		responses = append(responses, -1)
	}
	return responses, nil
}
