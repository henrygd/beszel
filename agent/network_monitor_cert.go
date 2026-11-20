package agent

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/henrygd/beszel/internal/entities/monitor"
)

// certCheckInterval is how often a certificate is re-read, so renewals show up
// within the hour.
const certCheckInterval = time.Hour

// certChecker fetches the leaf certificate for an HTTPS target over the given IP version.
type certChecker func(ctx context.Context, target string, ipVersion uint8) (monitor.CertInfo, error)

// certCheckEnabled reports whether a monitor's certificate is checked, which is
// the case for every HTTP monitor with an https target.
func certCheckEnabled(config monitor.Config) bool {
	return config.Protocol == "http" && len(config.Target) > 8 && strings.EqualFold(config.Target[:8], "https://")
}

// checkCert reads the leaf certificate presented by an HTTPS target. The chain is
// not verified, so expired or self-signed certificates are still reported. It
// connects over the monitor's IP version, since each family may reach a
// different server.
func checkCert(ctx context.Context, target string, ipVersion uint8) (monitor.CertInfo, error) {
	address, host, err := certAddress(target)
	if err != nil {
		return monitor.CertInfo{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, monitor.MaxProbeTimeout)
	defer cancel()
	dialer := tls.Dialer{Config: &tls.Config{ServerName: host, InsecureSkipVerify: true}}
	conn, err := dialer.DialContext(ctx, monitor.Network("tcp", ipVersion), address)
	if err != nil {
		return monitor.CertInfo{}, lookupError(err, ipVersion, host)
	}
	defer conn.Close()
	certs := conn.(*tls.Conn).ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return monitor.CertInfo{}, errors.New("no peer certificates")
	}
	leaf := certs[0]
	return monitor.CertInfo{
		Expires: leaf.NotAfter.UnixMilli(),
		Issuer:  leaf.Issuer.CommonName,
	}, nil
}

// certAddress returns the dial address and server name for an HTTPS URL.
func certAddress(target string) (address, host string, err error) {
	u, err := url.Parse(target)
	if err != nil {
		return "", "", err
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return "", "", fmt.Errorf("certificate check requires an https target: %s", target)
	}
	host = u.Hostname()
	if host == "" {
		return "", "", fmt.Errorf("missing host in target: %s", target)
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	return net.JoinHostPort(host, port), host, nil
}
