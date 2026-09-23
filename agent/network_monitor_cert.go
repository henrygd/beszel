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

const (
	certCheckTimeout       = 10 * time.Second
	certCheckInterval      = 24 * time.Hour
	certCheckRetryInterval = time.Hour
)

// certChecker fetches the leaf certificate for an HTTPS target.
type certChecker func(context.Context, string) (monitor.CertInfo, error)

// checkCert reads the leaf certificate presented by an HTTPS target. The chain is
// not verified, so expired or self-signed certificates are still reported.
func checkCert(ctx context.Context, target string) (monitor.CertInfo, error) {
	address, host, err := certAddress(target)
	if err != nil {
		return monitor.CertInfo{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, certCheckTimeout)
	defer cancel()
	dialer := tls.Dialer{Config: &tls.Config{ServerName: host, InsecureSkipVerify: true}}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return monitor.CertInfo{}, err
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
		Subject: leaf.Subject.CommonName,
		Checked: time.Now().UnixMilli(),
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
