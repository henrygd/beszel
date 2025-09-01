//go:build development

package hub

import (
	"net/http/httputil"
	"net/url"

	"github.com/pocketbase/pocketbase/core"
)

// startServer sets up the development server for Beszel
func (h *Hub) startServer(se *core.ServeEvent) error {
	proxy := httputil.NewSingleHostReverseProxy(&url.URL{
		Scheme: "http",
		Host:   "localhost:5173",
	})
	se.Router.GET("/{path...}", func(e *core.RequestEvent) error {
		proxy.ServeHTTP(e.Response, e.Request)
		return nil
	})
	return nil
}
