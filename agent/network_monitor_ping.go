package agent

import (
	"bytes"
	"crypto/rand"
	"errors"
	"math"
	"net"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"

	"log/slog"
)

var pingTimeRegex = regexp.MustCompile(`time[=<]([\d.]+)\s*ms`)

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
func monitorICMP(target string) (int64, error) {
	family, ip, err := resolveICMPTarget(target)
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
		return monitorICMPNative(family.rawNetwork, family, &net.IPAddr{IP: ip})
	case icmpDatagram:
		return monitorICMPNative(family.dgramNetwork, family, &net.UDPAddr{IP: ip})
	case icmpExecFallback:
		return monitorICMPExec(target, family.isIPv6)
	default:
		return -1, errors.New("unsupported ICMP mode")
	}
}

// resolveICMPTarget resolves a target hostname or IP to determine the address
// family and concrete IP address. Prefers IPv4 for dual-stack hostnames.
func resolveICMPTarget(target string) (*icmpFamily, net.IP, error) {
	if ip := net.ParseIP(target); ip != nil {
		if ip.To4() != nil {
			return &icmpV4, ip.To4(), nil
		}
		return &icmpV6, ip, nil
	}

	ips, err := net.LookupIP(target)
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
func monitorICMPNative(network string, family *icmpFamily, dst net.Addr) (int64, error) {
	conn, err := icmp.ListenPacket(network, family.listenAddr)
	if err != nil {
		return -1, err
	}
	defer conn.Close()

	return monitorICMPPacket(conn, family, dst)
}

func monitorICMPPacket(conn net.PacketConn, family *icmpFamily, dst net.Addr) (int64, error) {
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

// monitorICMPExec falls back to the system ping command. Returns -1 and an error on failure.
func monitorICMPExec(target string, isIPv6 bool) (int64, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		if isIPv6 {
			cmd = exec.Command("ping", "-6", "-n", "1", "-w", "3000", target)
		} else {
			cmd = exec.Command("ping", "-n", "1", "-w", "3000", target)
		}
	default:
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
			return -1, err
		}
	}

	matches := pingTimeRegex.FindSubmatch(output)
	if len(matches) >= 2 {
		if ms, err := strconv.ParseFloat(string(matches[1]), 64); err == nil {
			return int64(math.Round(ms * 1000)), nil
		}
	}

	// Fallback: use wall clock time if ping succeeded but parsing failed
	if err == nil {
		return time.Since(start).Microseconds(), nil
	}
	return -1, err
}
