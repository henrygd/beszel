//go:build !development

package hub

import (
	"beszel"
	"beszel/site"
	"io/fs"
	"net/http"
	"net/url"
	"strings"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// startServer sets up the production server for Beszel
func (h *Hub) startServer(se *core.ServeEvent) error {
	// parse app url
	parsedURL, err := url.Parse(h.appURL)
	if err != nil {
		return err
	}
	// fix base paths in html if using subpath
	basePath := strings.TrimSuffix(parsedURL.Path, "/") + "/"
	indexFile, _ := fs.ReadFile(site.DistDirFS, "index.html")
	indexContent := strings.ReplaceAll(string(indexFile), "./", basePath)
	indexContent = strings.Replace(indexContent, "{{V}}", beszel.Version, 1)
	indexContent = strings.Replace(indexContent, "{{HUB_URL}}", h.appURL, 1)
	// set up static asset serving
	staticPaths := [2]string{"/static/", "/assets/"}
	serveStatic := apis.Static(site.DistDirFS, false)
	// get CSP configuration
	csp, cspExists := GetEnv("CSP")
	// add route
	se.Router.GET("/{path...}", func(e *core.RequestEvent) error {
		// serve static assets if path is in staticPaths
		for i := range staticPaths {
			if strings.Contains(e.Request.URL.Path, staticPaths[i]) {
				e.Response.Header().Set("Cache-Control", "public, max-age=2592000")
				return serveStatic(e)
			}
		}
		if cspExists {
			e.Response.Header().Del("X-Frame-Options")
			e.Response.Header().Set("Content-Security-Policy", csp)
		}
		return e.HTML(http.StatusOK, indexContent)
	})
	return nil
}
