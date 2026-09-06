package alerts

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/nicholas-fedor/shoutrrr/pkg/router"
	"github.com/nicholas-fedor/shoutrrr/pkg/types"
)

var (
	errInternalDestination   = errors.New("Only admins can send to internal destinations")
	errUnrestrictedService   = errors.New("Only admins can use notification services without HTTP client support")
	publicNotificationClient = newPublicNotificationClient()
)

func newPublicNotificationClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		// Control receives the resolved IP, immediately before connect. Every
		// address attempted (including DNS retries and redirects) is checked.
		Control: func(_, address string, _ syscall.RawConn) error {
			return checkNotificationAddress(address)
		},
	}
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			// Do not use proxies: they can resolve the target themselves and
			// bypass the destination check on our socket.
			DialContext:         dialer.DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}

func checkNotificationAddress(address string) error {
	addr, err := netip.ParseAddrPort(address)
	if err != nil || addr.Addr().Zone() != "" {
		return errInternalDestination
	}
	ip := net.IP(addr.Addr().AsSlice())
	if !ip.IsGlobalUnicast() || isInternalIP(ip) {
		return errInternalDestination
	}
	return nil
}

func sendPublicNotification(rawURL, message string) error {
	client := &notificationClient{Client: publicNotificationClient}
	service, err := newPublicNotificationService(rawURL, client)
	if err == nil {
		err = service.Send(message, &types.Params{})
	}
	// Some services format errors without preserving their error chain.
	if client.blocked.Load() {
		return errInternalDestination
	}
	return err
}

type notificationClient struct {
	*http.Client
	blocked atomic.Bool
}

func (c *notificationClient) Do(req *http.Request) (*http.Response, error) {
	response, err := c.Client.Do(req)
	if errors.Is(err, errInternalDestination) {
		c.blocked.Store(true)
	}
	return response, err
}

func newPublicNotificationService(rawURL string, client types.HTTPClient) (types.Service, error) {
	r := &router.ServiceRouter{}
	scheme, serviceURL, err := r.ExtractServiceName(rawURL)
	if err != nil {
		return nil, err
	}
	service, err := r.NewService(scheme)
	if err != nil {
		return nil, err
	}
	setter, ok := service.(types.HTTPClientSetter)
	if !ok {
		return nil, errUnrestrictedService
	}
	if serviceURL.Scheme != scheme {
		custom, ok := service.(types.CustomURLService)
		if !ok {
			return nil, fmt.Errorf("%w: %s", router.ErrCustomURLsNotSupported, scheme)
		}
		serviceURL, err = custom.GetServiceURLFromCustom(serviceURL)
		if err != nil {
			return nil, err
		}
	}
	// Shoutrrr v0.19.0 CreateSenderWithOptions injects only AFTER Initialize.
	// Matrix can log in during Initialize, so inject before it as well.
	setter.SetHTTPClient(client)
	if err := service.Initialize(serviceURL, nil); err != nil {
		return nil, err
	}
	// Some initializers replace their HTTP client with a default client.
	setter.SetHTTPClient(client)
	return service, nil
}

var cgnatNetwork = &net.IPNet{
	IP:   net.IPv4(100, 64, 0, 0),
	Mask: net.CIDRMask(10, 32),
}

func isInternalIP(ip net.IP) bool {
	return ip.IsPrivate() ||
		ip.IsLoopback() ||
		ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsMulticast() ||
		cgnatNetwork.Contains(ip)
}
