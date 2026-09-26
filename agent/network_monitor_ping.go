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

const (
	// icmpPingCount is the number of pings sent per ICMP check.
	icmpPingCount = 5
	// icmpPingInterval spaces native pings, matching the shortest interval
	// unprivileged `ping -i` allows. The ping command keeps its default interval.
	icmpPingInterval = 200 * time.Millisecond
	// icmpReplyTimeout is how long to wait for the reply to each ping.
	icmpReplyTimeout = 3 * time.Second
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

// monitorICMP sends count ICMP echo requests to target, like `ping -c count`,
// and measures the round-trip time of each. Supports both IPv4 and IPv6 targets.
// The ICMP method (raw socket, unprivileged datagram, or exec fallback) is
// detected once per address family and cached for subsequent monitors.
// Returns one response per ping in microseconds, -1 for a lost ping, and an
// error only when no ping got a reply.
func monitorICMP(ctx context.Context, target string, count int) ([]int64, error) {
	// The target is resolved once for all pings.
	resolveCtx, cancel := context.WithTimeout(ctx, icmpReplyTimeout)
	family, ip, err := resolveICMPTarget(resolveCtx, target)
	cancel()
	if err != nil {
		return nil, err
	}
	if ip == nil {
		return nil, fmt.Errorf("no addresses resolved for %s", target)
	}

	icmpModeMu.Lock()
	if family.mode == icmpUntried {
		family.mode = detectICMPMode(family, icmpListen)
	}
	mode := family.mode
	icmpModeMu.Unlock()

	switch mode {
	case icmpRaw:
		return monitorICMPNative(ctx, family.rawNetwork, family, &net.IPAddr{IP: ip}, count)
	case icmpDatagram:
		return monitorICMPNative(ctx, family.dgramNetwork, family, &net.UDPAddr{IP: ip}, count)
	case icmpExecFallback:
		return monitorICMPExec(ctx, ip.String(), family.isIPv6, count)
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

// monitorICMPNative sends count ICMP echo requests over one socket using Go's x/net/icmp package.
func monitorICMPNative(ctx context.Context, network string, family *icmpFamily, dst net.Addr, count int) ([]int64, error) {
	conn, err := icmp.ListenPacket(network, family.listenAddr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	return monitorICMPPacket(ctx, conn, family, dst, count)
}

// monitorICMPPacket sends count echo requests spaced by icmpPingInterval and
// collects their replies, waiting up to icmpReplyTimeout after the last send.
func monitorICMPPacket(ctx context.Context, conn net.PacketConn, family *icmpFamily, dst net.Addr, count int) ([]int64, error) {
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
	// Reserve one sequence number per ping, so a reply maps back to its ping by offset.
	firstSeq := icmpSequence.Add(uint32(count)) - uint32(count) + 1
	targetIP := icmpAddrIP(dst)

	responses := make([]int64, count)
	for i := range responses {
		responses[i] = -1
	}
	sentAt := make([]time.Time, count)
	var sent, received int
	var lastSent, readDeadline time.Time
	var lastErr error
	buf := make([]byte, 1500)
	start := time.Now()
	for received < count {
		if sent < count && !time.Now().Before(start.Add(time.Duration(sent)*icmpPingInterval)) {
			msg := &icmp.Message{
				Type: family.echoType,
				Code: 0,
				Body: &icmp.Echo{ID: id, Seq: int((firstSeq + uint32(sent)) & 0xffff), Data: token},
			}
			msgBytes, err := msg.Marshal(nil)
			if err != nil {
				return nil, err
			}
			now := time.Now()
			if _, err := conn.WriteTo(msgBytes, dst); err != nil {
				if ctx.Err() != nil {
					return nil, err
				}
				// A failed send counts as a lost ping.
				lastErr = err
			} else {
				sentAt[sent], lastSent = now, now
			}
			sent++
			continue
		}

		// Wake up for the next send, or wait for outstanding replies after the last one.
		deadline := lastSent.Add(icmpReplyTimeout)
		if sent < count {
			deadline = start.Add(time.Duration(sent) * icmpPingInterval)
		} else if lastSent.IsZero() {
			break // every send failed
		}
		if !deadline.Equal(readDeadline) {
			if err := conn.SetReadDeadline(deadline); err != nil {
				return nil, err
			}
			readDeadline = deadline
		}

		n, peer, err := conn.ReadFrom(buf)
		now := time.Now()
		if err != nil {
			if sent < count && errors.Is(err, os.ErrDeadlineExceeded) {
				continue // time to send the next ping
			}
			lastErr = err
			break
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
		i := int((uint32(body.Seq) - firstSeq) & 0xffff)
		if i >= sent || sentAt[i].IsZero() || responses[i] >= 0 {
			continue
		}
		responses[i] = now.Sub(sentAt[i]).Microseconds()
		received++
	}

	if err := ctx.Err(); err != nil {
		return nil, err
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
// Linux, BSD IPv4 ping, and macOS ping6.
func pingCommand(goos, target string, isIPv6 bool, count int) (string, []string, error) {
	family := "-4"
	if isIPv6 {
		family = "-6"
	}
	n := strconv.Itoa(count)
	switch goos {
	case "windows":
		return "ping", []string{family, "-n", n, "-w", "3000", target}, nil
	case "linux":
		return "ping", []string{family, "-n", "-c", n, target}, nil
	case "darwin", "freebsd", "openbsd":
		command := "ping"
		if isIPv6 {
			command = "ping6"
		}
		return command, []string{"-n", "-c", n, target}, nil
	default:
		return "", nil, fmt.Errorf("ping fallback is unsupported on %s", goos)
	}
}

// monitorICMPExec falls back to the system ping command, sending count pings in
// one run. Returns one response per ping in microseconds, -1 for a lost ping,
// and an error only when no ping got a reply.
func monitorICMPExec(ctx context.Context, target string, isIPv6 bool, count int) ([]int64, error) {
	// ping sends one echo request per second, then waits for the last reply.
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(count-1)*time.Second+icmpReplyTimeout)
	defer cancel()
	name, args, err := pingCommand(runtime.GOOS, target, isIPv6, count)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(execCtx, name, args...)
	// Keep Unix output and decimal formatting stable. Windows ignores LC_ALL.
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	// Partial loss may end in a non-zero exit or the timeout, so use every reply printed until then.
	output, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	responses := make([]int64, count)
	for i := range responses {
		responses[i] = -1
	}
	if copy(responses, parsePingResponses(output)) > 0 {
		return responses, nil
	}
	switch {
	case execCtx.Err() != nil:
		return responses, execCtx.Err()
	case err != nil:
		return responses, fmt.Errorf("%s failed: %w", name, err)
	default:
		return responses, errors.New("ping output contains no round-trip time")
	}
}

// parsePingResponses returns the reported RTT of each reply in ping output, never
// subprocess execution time. For a bounded value such as Windows' time<1ms, the
// reported upper bound is retained. Lines with several times, such as the Windows
// "Minimum = 1ms, Maximum = 3ms, Average = 2ms" summary, and duplicate replies are skipped.
func parsePingResponses(output []byte) []int64 {
	var responses []int64
	for line := range bytes.Lines(output) {
		matches := pingTimeRegex.FindAllSubmatch(line, 2)
		if len(matches) != 1 || bytes.Contains(line, []byte("DUP!")) {
			continue
		}
		ms, err := strconv.ParseFloat(strings.ReplaceAll(string(matches[0][1]), ",", "."), 64)
		if err != nil || math.IsInf(ms, 0) || ms >= float64(math.MaxInt64)/1000 {
			continue
		}
		responses = append(responses, int64(math.Round(ms*1000)))
	}
	return responses
}
