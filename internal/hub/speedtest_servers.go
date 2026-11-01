package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/internal/hub/expirymap"
	"github.com/pocketbase/pocketbase/core"
)

// speedtestServersURL is Ookla's public server list API. It returns the servers
// closest to the caller, or servers matching the search parameter.
var speedtestServersURL = "https://www.speedtest.net/api/js/servers"

const (
	speedtestServersLimit    = 20
	speedtestServersCacheTTL = 10 * time.Minute
	speedtestServersTimeout  = 5 * time.Second
	speedtestServersMaxQuery = 100
)

// speedtestServer is an Ookla server as shown in the speedtest server picker.
// Name and Location match the server name and location the Ookla CLI reports in results.
type speedtestServer struct {
	ID       uint32 `json:"id"`
	Name     string `json:"name"`
	Location string `json:"location"`
}

// speedtestServerCache holds JSON-encoded server lists keyed by lowercased search term.
var speedtestServerCache = sync.OnceValue(func() *expirymap.ExpiryMap[string] {
	return expirymap.New[string](time.Hour)
})

// getSpeedtestServers handles GET /api/beszel/speedtest/servers requests.
// The browser can't call Ookla's API directly because it doesn't allow cross-origin requests.
func (h *Hub) getSpeedtestServers(e *core.RequestEvent) error {
	search := strings.TrimSpace(e.Request.URL.Query().Get("search"))
	if len(search) > speedtestServersMaxQuery {
		search = search[:speedtestServersMaxQuery]
	}
	cacheKey := strings.ToLower(search)
	cache := speedtestServerCache()
	body, ok := cache.GetOk(cacheKey)
	if !ok {
		servers, err := fetchSpeedtestServers(e.Request.Context(), search)
		if err != nil {
			h.Logger().Warn("failed to fetch speedtest servers", "search", search, "err", err)
			return e.JSON(http.StatusBadGateway, map[string]string{"message": "Failed to fetch speedtest servers."})
		}
		encoded, err := json.Marshal(servers)
		if err != nil {
			return e.InternalServerError("", err)
		}
		body = string(encoded)
		cache.Set(cacheKey, body, speedtestServersCacheTTL)
	}
	return e.Blob(http.StatusOK, "application/json", []byte(body))
}

// fetchSpeedtestServers queries Ookla's server list API.
func fetchSpeedtestServers(ctx context.Context, search string) ([]speedtestServer, error) {
	ctx, cancel := context.WithTimeout(ctx, speedtestServersTimeout)
	defer cancel()

	query := url.Values{"engine": {"js"}, "limit": {strconv.Itoa(speedtestServersLimit)}}
	if search != "" {
		query.Set("search", search)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, speedtestServersURL+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Beszel/"+beszel.Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var items []struct {
		ID      string `json:"id"`
		Sponsor string `json:"sponsor"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}
	servers := make([]speedtestServer, 0, len(items))
	for _, item := range items {
		id, err := strconv.ParseUint(item.ID, 10, 32)
		if err != nil || id == 0 {
			continue
		}
		servers = append(servers, speedtestServer{ID: uint32(id), Name: item.Sponsor, Location: item.Name})
	}
	return servers, nil
}
