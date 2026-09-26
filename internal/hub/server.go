package hub

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/internal/hub/utils"
)

// PublicAppInfo defines the structure of the public app information that will be injected into the HTML
type PublicAppInfo struct {
	BASE_PATH           string
	HUB_VERSION         string
	HUB_URL             string
	OAUTH_DISABLE_POPUP bool `json:"OAUTH_DISABLE_POPUP,omitempty"`
}

// modifyIndexHTML injects the public app information into the index.html content
func modifyIndexHTML(hub *Hub, html []byte) string {
	info := getPublicAppInfo(hub)
	content, err := json.Marshal(info)
	if err != nil {
		return string(html)
	}
	htmlContent := strings.ReplaceAll(string(html), "./", info.BASE_PATH)
	return strings.Replace(htmlContent, "\"{info}\"", string(content), 1)
}

// isAppRoute reports whether urlPath matches a frontend route, so unknown paths
// can be served with a 404 status. The base path prefix is optional because
// reverse proxies may or may not strip it before forwarding.
//
// Keep in sync with routes in internal/site/src/components/router.tsx.
func isAppRoute(urlPath, basePath string) bool {
	urlPath = strings.ToLower(urlPath)
	if base := strings.TrimSuffix(strings.ToLower(basePath), "/"); base != "" {
		if rest, ok := strings.CutPrefix(urlPath, base); ok && (rest == "" || rest[0] == '/') {
			urlPath = rest
		}
	}
	urlPath = strings.TrimSuffix(urlPath, "/")
	switch urlPath {
	case "", "/containers", "/smart", "/monitors", "/settings", "/forgot-password", "/request-otp":
		return true
	}
	// routes with a single required (/system/:id) or optional (/settings/:name?) param
	for _, prefix := range [...]string{"/system/", "/settings/"} {
		if param, ok := strings.CutPrefix(urlPath, prefix); ok {
			return param != "" && !strings.Contains(param, "/")
		}
	}
	return false
}

func getPublicAppInfo(hub *Hub) PublicAppInfo {
	parsedURL, _ := url.Parse(hub.appURL)
	info := PublicAppInfo{
		BASE_PATH:   strings.TrimSuffix(parsedURL.Path, "/") + "/",
		HUB_VERSION: beszel.Version,
		HUB_URL:     hub.appURL,
	}
	if val, _ := utils.GetEnv("OAUTH_DISABLE_POPUP"); val == "true" {
		info.OAUTH_DISABLE_POPUP = true
	}
	return info
}
