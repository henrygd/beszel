package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/henrygd/beszel"
	"github.com/spf13/cobra"
)

const (
	appUpdateImage      = "ghcr.io/dvorinka/beszel:latest"
	appUpdateRepository = "dvorinka/beszel"
	dockerSocketPath    = "/var/run/docker.sock"
)

var updateContainerIDPattern = regexp.MustCompile(`[0-9a-f]{64}`)
var appUpdateHTTPClient = &http.Client{Timeout: 20 * time.Second}

type updateCacheState struct {
	mu      sync.Mutex
	checked time.Time
	info    UpdateInfo
}

var appUpdateCache updateCacheState

type updateApplyState struct {
	mu      sync.Mutex
	running bool
}

var appUpdateApply updateApplyState

// UpdateInfo holds information about the latest GHCR image check.
type UpdateInfo struct {
	Version         string `json:"v,omitempty"`
	Url             string `json:"url,omitempty"`
	CurrentVersion  string `json:"currentVersion"`
	Image           string `json:"image"`
	CurrentImageID  string `json:"currentImageId,omitempty"`
	CurrentDigest   string `json:"currentDigest,omitempty"`
	LatestDigest    string `json:"latestDigest,omitempty"`
	UpdateAvailable bool   `json:"updateAvailable"`
	CanApply        bool   `json:"canApply"`
	Status          string `json:"status"`
	Message         string `json:"message"`
	LastCheck       string `json:"lastCheck"`
}

type applyUpdateResponse struct {
	Started bool   `json:"started"`
	Message string `json:"message"`
}

type dockerAPI struct {
	client *http.Client
}

type dockerContainerInspect struct {
	ID              string                `json:"Id"`
	Name            string                `json:"Name"`
	Image           string                `json:"Image"`
	Config          map[string]any        `json:"Config"`
	HostConfig      map[string]any        `json:"HostConfig"`
	NetworkSettings dockerNetworkSettings `json:"NetworkSettings"`
}

type dockerNetworkSettings struct {
	Networks map[string]map[string]any `json:"Networks"`
}

type dockerImageInspect struct {
	ID          string   `json:"Id"`
	RepoDigests []string `json:"RepoDigests"`
}

type dockerCreateResponse struct {
	ID       string   `json:"Id"`
	Warnings []string `json:"Warnings"`
}

func (h *Hub) getUpdate(e *core.RequestEvent) error {
	info := getCachedUpdateInfo(false)
	return e.JSON(http.StatusOK, info)
}

func (h *Hub) applyUpdate(e *core.RequestEvent) error {
	if !beginAppUpdate() {
		return e.BadRequestError("An app update is already running.", nil)
	}
	helperStarted := false
	defer func() {
		if !helperStarted {
			finishAppUpdate()
		}
	}()

	info := getCachedUpdateInfo(true)
	if !info.CanApply {
		return e.BadRequestError(info.Message, nil)
	}
	if !info.UpdateAvailable {
		return e.BadRequestError("Beszel is already using the latest image digest.", nil)
	}

	docker, err := newDockerAPI()
	if err != nil {
		return e.BadRequestError(err.Error(), nil)
	}
	container, err := docker.inspectContainer(currentContainerID(docker))
	if err != nil {
		return e.BadRequestError("Current Beszel container was not found through Docker.", err)
	}
	if err := docker.pullImage(appUpdateImage); err != nil {
		return e.InternalServerError("Failed to pull latest Beszel image.", err)
	}
	if err := docker.startUpdateHelper(container.ID, appUpdateImage); err != nil {
		return e.InternalServerError("Failed to start update helper.", err)
	}
	helperStarted = true
	time.AfterFunc(5*time.Minute, finishAppUpdate)

	appUpdateCache.mu.Lock()
	appUpdateCache.info.Version = "latest"
	appUpdateCache.info.Status = "updating"
	appUpdateCache.info.Message = "Update helper started. Beszel will restart after the new container is ready."
	appUpdateCache.mu.Unlock()

	return e.JSON(http.StatusOK, applyUpdateResponse{
		Started: true,
		Message: "Update helper started. Beszel will restart after the new container is ready.",
	})
}

func getCachedUpdateInfo(force bool) UpdateInfo {
	appUpdateCache.mu.Lock()
	if !force && time.Since(appUpdateCache.checked) < 30*time.Minute && appUpdateCache.info.CurrentVersion != "" {
		info := appUpdateCache.info
		appUpdateCache.mu.Unlock()
		return info
	}
	appUpdateCache.mu.Unlock()

	info := checkUpdateInfo()

	appUpdateCache.mu.Lock()
	appUpdateCache.checked = time.Now()
	appUpdateCache.info = info
	appUpdateCache.mu.Unlock()

	return info
}

func checkUpdateInfo() UpdateInfo {
	now := time.Now().UTC().Format(time.RFC3339)
	info := UpdateInfo{
		Url:            "https://github.com/dvorinka/Beszel/pkgs/container/beszel",
		CurrentVersion: beszel.Version,
		Image:          appUpdateImage,
		Status:         "checking",
		LastCheck:      now,
	}

	latestDigest, err := fetchGHCRDigest(context.Background(), appUpdateHTTPClient, appUpdateRepository, "latest")
	if err != nil {
		info.Status = "check-failed"
		info.Message = "Could not read latest image digest from GHCR: " + err.Error()
		return info
	}
	info.LatestDigest = latestDigest

	docker, err := newDockerAPI()
	if err != nil {
		info.Status = "docker-unavailable"
		info.Message = "Automatic updates need the Docker socket mounted at /var/run/docker.sock."
		return info
	}
	containerID := currentContainerID(docker)
	container, err := docker.inspectContainer(containerID)
	if err != nil {
		info.Status = "container-unavailable"
		info.Message = "Docker is available, but the running Beszel container could not be inspected."
		return info
	}
	info.CurrentImageID = container.Image
	info.CanApply = true

	image, err := docker.inspectImage(container.Image)
	if err == nil {
		info.CurrentDigest = findRepoDigest(image.RepoDigests, appUpdateRepository)
	}

	switch {
	case info.CurrentDigest == "":
		info.Version = "latest"
		info.Status = "unknown"
		info.UpdateAvailable = true
		info.Message = "Running image digest is unknown. Update can pull and recreate from latest."
	case digestValue(info.CurrentDigest) != digestValue(info.LatestDigest):
		info.Version = "latest"
		info.Status = "update-available"
		info.UpdateAvailable = true
		info.Message = "New Beszel image is available."
	default:
		info.Status = "up-to-date"
		info.Message = "Beszel is already using the latest image digest."
	}
	return info
}

func beginAppUpdate() bool {
	appUpdateApply.mu.Lock()
	defer appUpdateApply.mu.Unlock()
	if appUpdateApply.running {
		return false
	}
	appUpdateApply.running = true
	return true
}

func finishAppUpdate() {
	appUpdateApply.mu.Lock()
	appUpdateApply.running = false
	appUpdateApply.mu.Unlock()
}

func fetchGHCRDigest(ctx context.Context, client *http.Client, repository, tag string) (string, error) {
	manifestURL := fmt.Sprintf("https://ghcr.io/v2/%s/manifests/%s", repository, tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ", "))

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		token, err := fetchRegistryToken(ctx, client, resp.Header.Get("WWW-Authenticate"))
		if err != nil {
			return "", err
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Accept", "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err = client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
	}

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("registry returned %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", errors.New("registry response did not include Docker-Content-Digest")
	}
	return digest, nil
}

func fetchRegistryToken(ctx context.Context, client *http.Client, challenge string) (string, error) {
	params := parseBearerChallenge(challenge)
	realm := params["realm"]
	if realm == "" {
		return "", errors.New("registry auth challenge missing realm")
	}
	tokenURL, err := url.Parse(realm)
	if err != nil {
		return "", err
	}
	query := tokenURL.Query()
	for _, key := range []string{"service", "scope"} {
		if params[key] != "" {
			query.Set(key, params[key])
		}
	}
	tokenURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("token service returned %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	var data struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", err
	}
	if data.Token == "" {
		return "", errors.New("token service returned empty token")
	}
	return data.Token, nil
}

func parseBearerChallenge(challenge string) map[string]string {
	out := make(map[string]string)
	challenge = strings.TrimSpace(strings.TrimPrefix(challenge, "Bearer"))
	for _, part := range strings.Split(challenge, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		out[key] = strings.Trim(value, `"`)
	}
	return out
}

func newDockerAPI() (*dockerAPI, error) {
	if _, err := os.Stat(dockerSocketPath); err != nil {
		return nil, err
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", dockerSocketPath)
		},
	}
	return &dockerAPI{
		client: &http.Client{
			Timeout:   10 * time.Minute,
			Transport: transport,
		},
	}, nil
}

func (d *dockerAPI) do(method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, "http://docker"+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Docker API %s %s returned %s: %s", method, path, resp.Status, strings.TrimSpace(string(raw)))
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func (d *dockerAPI) inspectContainer(id string) (*dockerContainerInspect, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("empty container id")
	}
	var inspect dockerContainerInspect
	err := d.do(http.MethodGet, "/containers/"+url.PathEscape(id)+"/json", nil, &inspect)
	return &inspect, err
}

func (d *dockerAPI) inspectImage(id string) (*dockerImageInspect, error) {
	var inspect dockerImageInspect
	err := d.do(http.MethodGet, "/images/"+url.PathEscape(id)+"/json", nil, &inspect)
	return &inspect, err
}

func (d *dockerAPI) pullImage(image string) error {
	name, tag, _ := strings.Cut(image, ":")
	if tag == "" {
		tag = "latest"
	}
	path := "/images/create?fromImage=" + url.QueryEscape(name) + "&tag=" + url.QueryEscape(tag)
	return d.do(http.MethodPost, path, nil, nil)
}

func (d *dockerAPI) startUpdateHelper(targetID, image string) error {
	name := "beszel-update-" + shortID(targetID) + "-" + fmt.Sprint(time.Now().Unix())
	createBody := map[string]any{
		"Image": image,
		"Cmd": []string{
			"container-update-helper",
			"--target", targetID,
			"--image", image,
		},
		"HostConfig": map[string]any{
			"AutoRemove": true,
			"Binds":      []string{dockerSocketPath + ":" + dockerSocketPath},
		},
	}
	var created dockerCreateResponse
	if err := d.do(http.MethodPost, "/containers/create?name="+url.QueryEscape(name), createBody, &created); err != nil {
		return err
	}
	return d.do(http.MethodPost, "/containers/"+url.PathEscape(created.ID)+"/start", nil, nil)
}

func currentContainerID(d *dockerAPI) string {
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		if container, err := d.inspectContainer(hostname); err == nil {
			return container.ID
		}
	}
	raw, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return ""
	}
	return updateContainerIDPattern.FindString(string(raw))
}

func findRepoDigest(repoDigests []string, repository string) string {
	for _, digest := range repoDigests {
		if strings.Contains(digest, repository+"@sha256:") {
			return digest
		}
	}
	return ""
}

func digestValue(digest string) string {
	if _, value, ok := strings.Cut(digest, "@"); ok {
		return value
	}
	return digest
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// NewContainerUpdateHelperCmd creates a helper command that runs outside the current container.
func NewContainerUpdateHelperCmd() *cobra.Command {
	var targetID string
	var image string
	cmd := &cobra.Command{
		Use:    "container-update-helper",
		Short:  "Replace the running Beszel container with a newer image",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if targetID == "" || image == "" {
				return errors.New("target and image are required")
			}
			docker, err := newDockerAPI()
			if err != nil {
				return err
			}
			return docker.replaceContainer(targetID, image)
		},
	}
	cmd.Flags().StringVar(&targetID, "target", "", "target container id")
	cmd.Flags().StringVar(&image, "image", appUpdateImage, "replacement image")
	return cmd
}

func (d *dockerAPI) replaceContainer(targetID, image string) error {
	current, err := d.inspectContainer(targetID)
	if err != nil {
		return err
	}
	originalName := strings.TrimPrefix(current.Name, "/")
	if originalName == "" {
		originalName = "beszel"
	}
	stamp := fmt.Sprint(time.Now().Unix())
	oldName := originalName + "-old-" + stamp
	newName := originalName + "-new-" + stamp

	config := cloneMap(current.Config)
	hostConfig := cloneMap(current.HostConfig)
	config["Image"] = image

	delete(hostConfig, "AutoRemove")
	createBody := cloneMap(config)
	createBody["HostConfig"] = hostConfig
	createBody["NetworkingConfig"] = map[string]any{"EndpointsConfig": cleanEndpointsConfig(current.NetworkSettings.Networks)}

	var created dockerCreateResponse
	if err := d.do(http.MethodPost, "/containers/create?name="+url.QueryEscape(newName), createBody, &created); err != nil {
		return err
	}
	cleanupNew := true
	defer func() {
		if cleanupNew {
			_ = d.do(http.MethodDelete, "/containers/"+url.PathEscape(created.ID)+"?force=true", nil, nil)
		}
	}()

	if err := d.do(http.MethodPost, "/containers/"+url.PathEscape(current.ID)+"/stop?t=10", nil, nil); err != nil {
		return err
	}
	if err := d.do(http.MethodPost, "/containers/"+url.PathEscape(current.ID)+"/rename?name="+url.QueryEscape(oldName), nil, nil); err != nil {
		_ = d.do(http.MethodPost, "/containers/"+url.PathEscape(current.ID)+"/start", nil, nil)
		return err
	}
	if err := d.do(http.MethodPost, "/containers/"+url.PathEscape(created.ID)+"/rename?name="+url.QueryEscape(originalName), nil, nil); err != nil {
		_ = d.do(http.MethodPost, "/containers/"+url.PathEscape(current.ID)+"/rename?name="+url.QueryEscape(originalName), nil, nil)
		_ = d.do(http.MethodPost, "/containers/"+url.PathEscape(current.ID)+"/start", nil, nil)
		return err
	}
	if err := d.do(http.MethodPost, "/containers/"+url.PathEscape(created.ID)+"/start", nil, nil); err != nil {
		_ = d.do(http.MethodPost, "/containers/"+url.PathEscape(created.ID)+"/rename?name="+url.QueryEscape(newName), nil, nil)
		_ = d.do(http.MethodPost, "/containers/"+url.PathEscape(current.ID)+"/rename?name="+url.QueryEscape(originalName), nil, nil)
		_ = d.do(http.MethodPost, "/containers/"+url.PathEscape(current.ID)+"/start", nil, nil)
		return err
	}

	cleanupNew = false
	_ = d.do(http.MethodDelete, "/containers/"+url.PathEscape(current.ID)+"?force=true", nil, nil)
	return nil
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

// cleanEndpointsConfig strips runtime-populated fields from Docker network settings
// so they can be safely reused in a container create request.
func cleanEndpointsConfig(networks map[string]map[string]any) map[string]any {
	if networks == nil {
		return nil
	}
	out := make(map[string]any, len(networks))
	for netName, cfg := range networks {
		cleaned := make(map[string]any, len(cfg))
		for k, v := range cfg {
			switch k {
			case "NetworkID", "EndpointID", "Gateway", "IPAddress", "IPPrefixLen",
				"IPv6Gateway", "GlobalIPv6Address", "GlobalIPv6PrefixLen", "MacAddress":
				continue
			default:
				cleaned[k] = v
			}
		}
		out[netName] = cleaned
	}
	return out
}
