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
	"github.com/pocketbase/pocketbase/ui"
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

	// Helper to serve PocketBase Admin UI index.html with RegExp.escape polyfill
	serveAdminIndexHTML := func(e *core.RequestEvent) error {
		if adminIndexFile, err := fs.ReadFile(ui.DistDirFS, "index.html"); err == nil && len(adminIndexFile) > 0 {
			adminHtml := string(adminIndexFile)
			if !strings.Contains(adminHtml, "RegExp.escape") {
				polyfill := `<head><script>if(!RegExp.escape){RegExp.escape=function(s){return String(s).replace(/[.*+?^\x24{}()|[\]\\]/g,function(c){return"\\"+c;});};}</script>`
				adminHtml = strings.Replace(adminHtml, "<head>", polyfill, 1)
			}
			return e.HTML(http.StatusOK, adminHtml)
		}
		return nil
	}

	// Configurable & Dynamic Admin Route
	adminStatic := apis.Static(ui.DistDirFS, false)
	customAdminPath, _ := utils.GetEnv("ADMIN_PATH")
	customAdminPath = strings.TrimSpace(customAdminPath)
	if customAdminPath != "" {
		if !strings.HasPrefix(customAdminPath, "/") {
			customAdminPath = "/" + customAdminPath
		}
		if !strings.HasSuffix(customAdminPath, "/") {
			customAdminPath = customAdminPath + "/"
		}
		adminPrefix := strings.TrimSuffix(customAdminPath, "/")
		if adminPrefix != "" && adminPrefix != "/_" {
			se.Router.GET(adminPrefix+"/{path...}", func(e *core.RequestEvent) error {
				path := e.Request.URL.Path
				if path == adminPrefix || path == customAdminPath || path == customAdminPath+"index.html" {
					return serveAdminIndexHTML(e)
				}
				e.Request.URL.Path = strings.TrimPrefix(path, adminPrefix)
				if e.Request.URL.Path == "" {
					e.Request.URL.Path = "/"
				}
				return adminStatic(e)
			})
		}
	}

	// Inject RegExp.escape polyfill for legacy /_/ path as well
	se.Router.BindFunc(func(e *core.RequestEvent) error {
		path := e.Request.URL.Path
		if path == "/_/" || path == "/_/index.html" || path == "/_" {
			return serveAdminIndexHTML(e)
		}
		return e.Next()
	})

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
		return e.HTML(http.StatusOK, html)
	})
	return nil
}
