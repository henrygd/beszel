package agent

import (
	_ "crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
)

const imageRegistryTimeout = 10 * time.Second

const imageManifestAccept = "application/vnd.docker.distribution.manifest.list.v2+json, " +
	"application/vnd.docker.distribution.manifest.v2+json, " +
	"application/vnd.oci.image.manifest.v1+json, " +
	"application/vnd.oci.image.index.v1+json"

// checkImageUpdate compares the digest recorded by Docker for image with the
// digest currently advertised by its registry. A digest-pinned reference is
// immutable and therefore never has an update available.
func (dm *dockerManager) checkImageUpdate(image string) (bool, error) {
	named, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return false, fmt.Errorf("parse image reference %q: %w", image, err)
	}
	if _, pinned := named.(reference.Digested); pinned {
		return false, nil
	}
	named = reference.TagNameOnly(named)

	registry := reference.Domain(named)
	repository := reference.Path(named)
	tag := named.(reference.Tagged).Tag()

	localDigest, err := dm.inspectImageDigest(image, registry, repository)
	if err != nil {
		return false, err
	}

	remoteDigest, err := dm.registryImageDigest(registry, repository, tag)
	if err != nil {
		return false, err
	}

	return remoteDigest != localDigest, nil
}

// inspectImageDigest reads Docker's image metadata without using dm.decode.
// The checker runs in the image-discovery goroutine, so it must not hold any
// of the container statistics locks while waiting on the Docker API.
func (dm *dockerManager) inspectImageDigest(image, registry, repository string) (string, error) {
	if dm.client == nil {
		return "", fmt.Errorf("inspect image %q: Docker client is unavailable", image)
	}

	endpoint := "http://localhost/images/" + url.PathEscape(image) + "/json"
	resp, err := dm.client.Get(endpoint)
	if err != nil {
		return "", fmt.Errorf("inspect image %q: %w", image, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("inspect image %q failed: %s", image, responseStatus(resp))
	}

	var inspect struct {
		RepoDigests []string `json:"RepoDigests"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&inspect); err != nil {
		return "", fmt.Errorf("decode image inspect %q: %w", image, err)
	}
	if len(inspect.RepoDigests) == 0 {
		return "", fmt.Errorf("inspect image %q returned no repository digests", image)
	}

	localDigest, ok := matchingRepositoryDigest(inspect.RepoDigests, registry, repository)
	if !ok {
		return "", fmt.Errorf("inspect image %q returned no valid digest for %s/%s", image, registry, repository)
	}
	return localDigest, nil
}

// matchingRepositoryDigest returns a valid digest belonging to the requested
// repository. Docker can return multiple RepoDigests for one local image; an
// unrelated first entry must never be used for the comparison.
func matchingRepositoryDigest(repoDigests []string, registry, repository string) (string, bool) {
	for _, repoDigest := range repoDigests {
		repoDigest = strings.TrimSpace(repoDigest)
		at := strings.LastIndexByte(repoDigest, '@')
		if at <= 0 || at == len(repoDigest)-1 || strings.Contains(repoDigest[:at], "@") {
			continue
		}

		repoRef, err := reference.ParseNormalizedNamed(repoDigest[:at])
		if err != nil || reference.Path(repoRef) != repository || !sameRegistry(reference.Domain(repoRef), registry) {
			continue
		}
		if _, hasTag := repoRef.(reference.Tagged); hasTag {
			continue
		}

		d, err := digest.Parse(repoDigest[at+1:])
		if err != nil {
			continue
		}
		return d.String(), true
	}
	return "", false
}

func sameRegistry(left, right string) bool {
	left = canonicalRegistry(left)
	right = canonicalRegistry(right)
	return left == right ||
		(left == "ghcr.io" && right == "lscr.io") ||
		(left == "lscr.io" && right == "ghcr.io")
}

func canonicalRegistry(registry string) string {
	if registry == "index.docker.io" {
		return "docker.io"
	}
	return registry
}

func (dm *dockerManager) registryImageDigest(registry, repository, tag string) (string, error) {
	client := dm.registryClient
	if client == nil {
		client = &http.Client{Timeout: imageRegistryTimeout}
	}

	token, err := dm.registryToken(client, registry, repository)
	if err != nil {
		return "", err
	}

	host := registry
	if registry == "docker.io" {
		host = "registry-1.docker.io"
	}
	manifestURL := "https://" + host + "/v2/" + repository + "/manifests/" + url.PathEscape(tag)
	req, err := http.NewRequest(http.MethodHead, manifestURL, nil)
	if err != nil {
		return "", fmt.Errorf("create manifest request: %w", err)
	}
	req.Header.Set("Accept", imageManifestAccept)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch manifest %s:%s: %w", registry, repository, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("manifest request for %s:%s failed: %s", repository, tag, responseStatus(resp))
	}

	remote := strings.TrimSpace(resp.Header.Get("Docker-Content-Digest"))
	d, err := digest.Parse(remote)
	if err != nil {
		return "", fmt.Errorf("manifest request for %s:%s returned invalid digest: %w", repository, tag, err)
	}
	return d.String(), nil
}

func (dm *dockerManager) registryToken(client *http.Client, registry, repository string) (string, error) {
	var authURL string
	switch registry {
	case "docker.io":
		authURL = "https://auth.docker.io/token?service=registry.docker.io&scope=" + url.QueryEscape("repository:"+repository+":pull")
	case "ghcr.io", "lscr.io":
		// lscr.io is the LinuxServer alias for its GHCR-backed images.
		authURL = "https://ghcr.io/token?service=ghcr.io&scope=" + url.QueryEscape("repository:"+repository+":pull")
	default:
		// Anonymous registries remain supported, as they were before the
		// authenticated Docker Hub and GHCR paths were added.
		return "", nil
	}

	req, err := http.NewRequest(http.MethodGet, authURL, nil)
	if err != nil {
		return "", fmt.Errorf("create registry auth request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch registry auth token for %s: %w", repository, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry auth request for %s failed: %s", repository, responseStatus(resp))
	}

	var tokenResponse struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return "", fmt.Errorf("decode registry auth response for %s: %w", repository, err)
	}
	token := strings.TrimSpace(tokenResponse.Token)
	if token == "" {
		token = strings.TrimSpace(tokenResponse.AccessToken)
	}
	if token == "" {
		return "", fmt.Errorf("registry auth response for %s contained no token", repository)
	}
	return token, nil
}

func responseStatus(resp *http.Response) string {
	if resp.Status != "" {
		return resp.Status
	}
	return http.StatusText(resp.StatusCode)
}
