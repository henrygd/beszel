package alerts

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	errUnrestrictedService   = errors.New("Only admins can use this notification service") // Restrict services w/o custom connection support
	publicNotificationDialer = &net.Dialer{
		Timeout: 10 * time.Second,
		// Control checks each resolved address immediately before connecting.
		Control: func(_, address string, _ syscall.RawConn) error { return checkNotificationAddress(address) },
	}
	publicNotificationClient = newPublicNotificationClient()
)

func newPublicNotificationClient() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			// Do not use proxies: they can resolve the target themselves and
			// bypass the destination check on our socket.
			DialContext:         publicNotificationDialer.DialContext,
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
	service, err := newPublicNotificationService(rawURL, types.SenderOptions{HTTPClient: client, DialContext: client.dialContext})
	if err == nil {
		if closer, ok := service.(io.Closer); ok {
			defer closer.Close()
		}
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

func (c *notificationClient) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := publicNotificationDialer.DialContext(ctx, network, address)
	if errors.Is(err, errInternalDestination) {
		c.blocked.Store(true)
	}
	return conn, err
}

func newPublicNotificationService(rawURL string, opts types.SenderOptions) (types.Service, error) {
	r := &router.ServiceRouter{}
	scheme, serviceURL, err := r.ExtractServiceName(rawURL)
	if err != nil {
		return nil, err
	}
	service, err := r.NewService(scheme)
	if err != nil {
		return nil, err
	}
	httpSetter, httpOK := service.(types.HTTPClientSetter)
	dialSetter, dialOK := service.(types.DialContextSetter)
	if (!httpOK || opts.HTTPClient == nil) && (!dialOK || opts.DialContext == nil) {
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
	// Shoutrrr v0.20.0 CreateSenderWithOptions injects only AFTER Initialize.
	// Matrix can log in during Initialize, so inject before it as well.
	if httpOK {
		httpSetter.SetHTTPClient(opts.HTTPClient)
	}
	if dialOK {
		dialSetter.SetDialContext(opts.DialContext)
	}
	if err := service.Initialize(serviceURL, nil); err != nil {
		return nil, err
	}
	// Some initializers replace their HTTP client with a default client.
	if httpOK {
		httpSetter.SetHTTPClient(opts.HTTPClient)
	}
	if dialOK {
		dialSetter.SetDialContext(opts.DialContext)
	}
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
