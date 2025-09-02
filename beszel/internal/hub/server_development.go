//go:build development

package hub

import (
	"beszel"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// responseModifier wraps an http.RoundTripper to modify HTML responses
type responseModifier struct {
	transport http.RoundTripper
	hub       *Hub
}

// RoundTrip implements http.RoundTripper interface with response modification
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

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, err
	}
	resp.Body.Close()

	// Modify the HTML content here
	modifiedBody := rm.modifyHTML(string(body), req)

	// Create a new response with the modified body
	resp.Body = io.NopCloser(strings.NewReader(modifiedBody))
	resp.ContentLength = int64(len(modifiedBody))
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(modifiedBody)))

	return resp, nil
}

// modifyHTML applies modifications to HTML content
func (rm *responseModifier) modifyHTML(html string, req *http.Request) string {
	parsedURL, err := url.Parse(rm.hub.appURL)
	if err != nil {
		return html
	}
	// fix base paths in html if using subpath
	basePath := strings.TrimSuffix(parsedURL.Path, "/") + "/"
	html = strings.ReplaceAll(html, "./", basePath)
	html = strings.Replace(html, "{{V}}", beszel.Version, 1)
	slog.Info("modifying HTML", "appURL", rm.hub.appURL)
	html = strings.Replace(html, "{{HUB_URL}}", rm.hub.appURL, 1)

	systems, err := rm.hub.getUserSystemsFromRequest(req)
	if err != nil {
		return html
	}
	systemsJson, err := json.Marshal(systems)
	if err != nil {
		return html
	}
	html = strings.Replace(html, "'{SYSTEMS}'", string(systemsJson), 1)

	return html
}

// startServer sets up the development server for Beszel
func (h *Hub) startServer(se *core.ServeEvent) error {
	slog.Info("starting server", "appURL", h.appURL)
	proxy := httputil.NewSingleHostReverseProxy(&url.URL{
		Scheme: "http",
		Host:   "localhost:5173",
	})

	// Set up custom transport with response modification
	proxy.Transport = &responseModifier{
		transport: http.DefaultTransport,
		hub:       h,
	}

	se.Router.GET("/{path...}", func(e *core.RequestEvent) error {
		proxy.ServeHTTP(e.Response, e.Request)
		return nil
	})
	return nil
}
