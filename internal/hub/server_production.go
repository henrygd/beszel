//go:build !development

package hub

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/henrygd/beszel/internal/hub/utils"
	"github.com/henrygd/beszel/internal/site"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// startServer sets up the production server for Beszel
func (h *Hub) startServer(se *core.ServeEvent) error {
	indexFile, _ := fs.ReadFile(site.DistDirFS, "index.html")
	html := modifyIndexHTML(h, indexFile)
	// set up static asset serving
	staticPaths := [2]string{"/static/", "/assets/"}
	serveStatic := apis.Static(site.DistDirFS, false)
	// get CSP configuration
	csp, cspExists := utils.GetEnv("CSP")
	// add route
	se.Router.GET("/{path...}", func(e *core.RequestEvent) error {
		// serve static assets if path is in staticPaths
		for i, staticPath := range staticPaths {
			if strings.Contains(e.Request.URL.Path, staticPath) {
				if i == 1 {
					e.Response.Header().Set("Cache-Control", "public, max-age=2592000, immutable")
				} else {
					e.Response.Header().Set("Cache-Control", "public, max-age=3600")
				}
				return serveStatic(e)
			}
		}
		e.Response.Header().Set("Cache-Control", "no-cache")
		if cspExists {
			e.Response.Header().Del("X-Frame-Options")
			e.Response.Header().Set("Content-Security-Policy", csp)
		}
		return e.HTML(http.StatusOK, html)
	})
	return nil
}
