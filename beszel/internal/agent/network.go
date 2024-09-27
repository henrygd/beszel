package agent

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	psutilNet "github.com/shirou/gopsutil/v4/net"
)

func (a *Agent) initializeNetIoStats() {
	// reset valid network interfaces
	a.netInterfaces = make(map[string]struct{}, 0)

	// map of network interface names passed in via NICS env var
	var nicsMap map[string]struct{}
	nics, nicsEnvExists := os.LookupEnv("NICS")
	if nicsEnvExists {
		nicsMap = make(map[string]struct{}, 0)
		for _, nic := range strings.Split(nics, ",") {
			nicsMap[nic] = struct{}{}
		}
	}

	// reset network I/O stats
	a.netIoStats.BytesSent = 0
	a.netIoStats.BytesRecv = 0

	// get intial network I/O stats
	if netIO, err := psutilNet.IOCounters(true); err == nil {
		a.netIoStats.Time = time.Now()
		for _, v := range netIO {
			switch {
			// skip if nics exists and the interface is not in the list
			case nicsEnvExists:
				if _, nameInNics := nicsMap[v.Name]; !nameInNics {
					continue
				}
			// otherwise run the interface name through the skipNetworkInterface function
			default:
				if a.skipNetworkInterface(v) {
					continue
				}
			}
			slog.Info("Detected network interface", "name", v.Name, "sent", v.BytesSent, "recv", v.BytesRecv)
			a.netIoStats.BytesSent += v.BytesSent
			a.netIoStats.BytesRecv += v.BytesRecv
			// store as a valid network interface
			a.netInterfaces[v.Name] = struct{}{}
		}
	}
}

func (a *Agent) skipNetworkInterface(v psutilNet.IOCountersStat) bool {
	switch {
	case strings.HasPrefix(v.Name, "lo"),
		strings.HasPrefix(v.Name, "docker"),
		strings.HasPrefix(v.Name, "br-"),
		strings.HasPrefix(v.Name, "veth"),
		v.BytesRecv == 0,
		v.BytesSent == 0:
		return true
	default:
		return false
	}
}

func newDockerClient() *http.Client {
	dockerHost := "unix:///var/run/docker.sock"
	if dockerHostEnv, exists := os.LookupEnv("DOCKER_HOST"); exists {
		slog.Info("DOCKER_HOST", "host", dockerHostEnv)
		dockerHost = dockerHostEnv
	}

	parsedURL, err := url.Parse(dockerHost)
	if err != nil {
		slog.Error("Error parsing DOCKER_HOST", "err", err)
		os.Exit(1)
	}

	transport := &http.Transport{
		ForceAttemptHTTP2:   false,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
		MaxConnsPerHost:     20,
		MaxIdleConnsPerHost: 20,
		DisableKeepAlives:   false,
	}

	switch parsedURL.Scheme {
	case "unix":
		transport.DialContext = func(ctx context.Context, proto, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", parsedURL.Path)
		}
	case "tcp", "http", "https":
		transport.DialContext = func(ctx context.Context, proto, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", parsedURL.Host)
		}
	default:
		slog.Error("Invalid DOCKER_HOST", "scheme", parsedURL.Scheme)
		os.Exit(1)
	}

	return &http.Client{
		Timeout:   time.Second,
		Transport: transport,
	}
}

// closes idle connections on timeouts to prevent reuse of stale connections
func (a *Agent) closeIdleConnections(err error) (isTimeout bool) {
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		slog.Warn("Closing idle connections", "err", err)
		a.dockerClient.Transport.(*http.Transport).CloseIdleConnections()
		return true
	}
	return false
}
