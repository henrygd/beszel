//go:build development

package hub

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/osutils"
)

// Wraps http.RoundTripper to modify dev proxy HTML responses
type responseModifier struct {
	transport http.RoundTripper
	hub       *Hub
}

func (rm *responseModifier) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := rm.transport.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	// Only modify HTML responses
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		return resp, nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, err
	}
	resp.Body.Close()
	// Create a new response with the modified body
	modifiedBody := modifyIndexHTML(rm.hub, body)
	resp.Body = io.NopCloser(strings.NewReader(modifiedBody))
	resp.ContentLength = int64(len(modifiedBody))
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(modifiedBody)))

	return resp, nil
}

// startServer sets up the development server for Beszel
func (h *Hub) startServer(se *core.ServeEvent) error {
	proxy := httputil.NewSingleHostReverseProxy(&url.URL{
		Scheme: "http",
		Host:   "localhost:5173",
	})

	proxy.Transport = &responseModifier{
		transport: http.DefaultTransport,
		hub:       h,
	}

	se.Router.GET("/{path...}", func(e *core.RequestEvent) error {
		proxy.ServeHTTP(e.Response, e.Request)
		return nil
	})
	_ = osutils.LaunchURL(h.appURL)
	return nil
}
