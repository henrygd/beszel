package agent

import (
	_ "crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
)

const imageRegistryTimeout = 10 * time.Second

const (
	ociImageIndex       = "application/vnd.oci.image.index.v1+json"
	dockerManifestList  = "application/vnd.docker.distribution.manifest.list.v2+json"
	ociImageManifest    = "application/vnd.oci.image.manifest.v1+json"
	dockerImageManifest = "application/vnd.docker.distribution.manifest.v2+json"
	maxImageIndexSize   = 4 << 20
)

type registryImagePlatform struct {
	OS           string
	Architecture string
	Variant      string
}

type registryLocalImageMetadata struct {
	RepoDigests []string
	registryImagePlatform
}

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

	local, err := dm.inspectImageMetadata(image, registry, repository)
	if err != nil {
		return false, err
	}

	remoteDigest, err := dm.registryImageDigest(registry, repository, tag, local)
	if err != nil {
		return false, err
	}

	return !slices.Contains(local.RepoDigests, remoteDigest), nil
}

// inspectImageMetadata reads Docker's image metadata without using dm.decode.
// The checker runs in the image-discovery goroutine, so it must not hold any
// of the container statistics locks while waiting on the Docker API.
func (dm *dockerManager) inspectImageMetadata(image, registry, repository string) (*registryLocalImageMetadata, error) {
	if dm.client == nil {
		return nil, fmt.Errorf("inspect image %q: Docker client is unavailable", image)
	}

	endpoint := "http://localhost/images/" + url.PathEscape(image) + "/json"
	resp, err := dm.client.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("inspect image %q: %w", image, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("inspect image %q failed: %s", image, responseStatus(resp))
	}

	var inspect registryLocalImageMetadata
	if err := json.NewDecoder(resp.Body).Decode(&inspect); err != nil {
		return nil, fmt.Errorf("decode image inspect %q: %w", image, err)
	}
	if len(inspect.RepoDigests) == 0 {
		return nil, fmt.Errorf("inspect image %q returned no repository digests", image)
	}

	inspect.RepoDigests = matchingRepositoryDigests(inspect.RepoDigests, registry, repository)
	if len(inspect.RepoDigests) == 0 {
		return nil, fmt.Errorf("inspect image %q returned no valid digest for %s/%s", image, registry, repository)
	}
	return &inspect, nil
}

// matchingRepositoryDigests returns all valid digests belonging to the requested
// repository. Container engines can return both index and platform manifest digests for one
// local image, in either order.
func matchingRepositoryDigests(repoDigests []string, registry, repository string) []string {
	var digests []string
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
		digests = append(digests, d.String())
	}
	return digests
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

func (dm *dockerManager) registryImageDigest(registry, repository, tag string, local *registryLocalImageMetadata) (string, error) {
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
	// A matching index digest needs no further registry request.
	if !slices.Contains(local.RepoDigests, d.String()) {
		mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		if mediaType == ociImageIndex || mediaType == dockerManifestList {
			platformDigest, err := registryPlatformDigest(client, req, d, mediaType, local)
			if err == nil {
				return platformDigest, nil
			}
			// The HEAD digest is valid. An optional GET must not clear an
			// update that the original index comparison would report.
			slog.Debug("Image platform digest comparison failed", "repository", repository, "tag", tag, "err", err)
		}
	}
	return d.String(), nil
}

// registryPlatformDigest resolves an index mismatch for the locally inspected
// platform. Reuse the authenticated HEAD request, but fetch the immutable index
// digest rather than the tag, which could have changed since the HEAD request.
func registryPlatformDigest(client *http.Client, manifestRequest *http.Request, indexDigest digest.Digest, mediaType string, local *registryLocalImageMetadata) (string, error) {
	localPlatform := normalizedImagePlatform(local.registryImagePlatform)
	if localPlatform.OS == "" || localPlatform.OS == "unknown" || localPlatform.Architecture == "" || localPlatform.Architecture == "unknown" {
		return indexDigest.String(), nil
	}
	req := manifestRequest.Clone(manifestRequest.Context())
	req.Method = http.MethodGet
	req.URL = req.URL.JoinPath("..", indexDigest.String())
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch image index: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch image index failed: %s", responseStatus(resp))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxImageIndexSize+1))
	if err != nil {
		return "", fmt.Errorf("read image index: %w", err)
	}
	if len(body) > maxImageIndexSize {
		return "", fmt.Errorf("image index exceeds size limit")
	}
	if indexDigest.Algorithm().FromBytes(body) != indexDigest {
		return "", fmt.Errorf("image index digest mismatch")
	}
	if header := strings.TrimSpace(resp.Header.Get("Docker-Content-Digest")); header != "" && header != indexDigest.String() {
		return "", fmt.Errorf("image index response digest mismatch")
	}
	var index struct {
		SchemaVersion int
		MediaType     string
		Manifests     []struct {
			MediaType string
			Digest    string
			Platform  *struct {
				registryImagePlatform
				OSVersion  string   `json:"os.version"`
				OSFeatures []string `json:"os.features"`
				Features   []string
			}
		}
	}
	if err := json.Unmarshal(body, &index); err != nil {
		return "", fmt.Errorf("decode image index: %w", err)
	}
	if index.SchemaVersion != 2 || index.Manifests == nil || (index.MediaType != "" && index.MediaType != mediaType) {
		return "", fmt.Errorf("invalid image index")
	}
	// The image describes what was installed, not the host's full platform
	// capabilities. Only replace the index comparison when every possible
	// selection for the installed OS/architecture is locally recorded.
	// Keep descriptor order for equivalent platform requirements.
	type platformKey struct {
		variant, version, osFeatures, features string
	}
	seen := make(map[platformKey]bool)
	var current digest.Digest
	uncertain := false
	for _, manifest := range index.Manifests {
		nested := manifest.MediaType == ociImageIndex || manifest.MediaType == dockerManifestList
		if !nested && manifest.MediaType != ociImageManifest && manifest.MediaType != dockerImageManifest {
			continue
		}
		p := manifest.Platform
		var platform registryImagePlatform
		if p != nil {
			platform = normalizedImagePlatform(p.registryImagePlatform)
			// Explicit unknown/unknown leaves are not runnable platform
			// candidates (for example, attestations). Partial metadata is
			// inconclusive rather than proof of incompatibility.
			if !nested && platform.OS == "unknown" && platform.Architecture == "unknown" {
				continue
			}
			if (platform.OS != "" && platform.OS != "unknown" && platform.OS != localPlatform.OS) ||
				(platform.Architecture != "" && platform.Architecture != "unknown" && platform.Architecture != localPlatform.Architecture) {
				continue
			}
		}
		d, err := digest.Parse(manifest.Digest)
		if err != nil {
			if nested {
				return "", fmt.Errorf("invalid nested index digest: %w", err)
			}
			return "", fmt.Errorf("invalid platform manifest digest: %w", err)
		}
		samePlatform := p != nil && platform.OS == localPlatform.OS && platform.Architecture == localPlatform.Architecture
		if samePlatform {
			key := platformKey{
				platform.Variant,
				p.OSVersion, platformFeaturesKey(p.OSFeatures), platformFeaturesKey(p.Features),
			}
			if seen[key] {
				continue
			}
			// A later descriptor with the same requirements cannot
			// supersede an earlier direct manifest.
			if !nested {
				seen[key] = true
			}
		}
		// Do not guess the host's variant/Windows compatibility or
		// recursively resolve an index. Missing OCI platform is allowed;
		// incomplete declared metadata is not sufficient for comparison.
		if nested || (p == nil && mediaType != ociImageIndex) ||
			(p != nil && !samePlatform) {
			uncertain = true
			continue
		}
		if slices.Contains(local.RepoDigests, d.String()) {
			current = d
		} else {
			uncertain = true
		}
	}
	if !uncertain && current != "" {
		return current.String(), nil
	}
	// A well-formed but inconclusive index is not a registry error.
	// Preserve the original digest comparison, including possible updates.
	return indexDigest.String(), nil
}

// Feature requirements are sets, not ordered lists.
func platformFeaturesKey(features []string) string {
	features = slices.Clone(features)
	slices.Sort(features)
	return fmt.Sprintf("%q", slices.Compact(features))
}

// Normalize platform spellings before comparing or excluding descriptors.
// Match containerd's documented aliases without filling absent fields with
// the agent's own runtime platform (the inspected image can be remote).
func normalizedImagePlatform(p registryImagePlatform) registryImagePlatform {
	p.OS = strings.ToLower(p.OS)
	if p.OS == "macos" {
		p.OS = "darwin"
	}
	p.Architecture = strings.ToLower(p.Architecture)
	p.Variant = strings.ToLower(p.Variant)
	switch p.Architecture {
	case "aarch64":
		p.Architecture = "arm64"
	case "x86_64", "x86-64":
		p.Architecture = "amd64"
	case "i386":
		p.Architecture, p.Variant = "386", ""
	case "armhf":
		p.Architecture, p.Variant = "arm", "v7"
	case "armel":
		p.Architecture, p.Variant = "arm", "v6"
	}
	p.Variant = normalizedImageVariant(p.Architecture, p.Variant)
	return p
}

// Omitted platform defaults and ARM aliases follow containerd's platform
// normalization. Other explicit variants are compared without assumptions.
func normalizedImageVariant(arch, variant string) string {
	switch arch {
	case "amd64":
		if variant == "v1" {
			return ""
		}
	case "arm64":
		switch variant {
		case "8", "v8", "v8.0":
			return ""
		case "9", "9.0", "v9.0":
			return "v9"
		}
	case "arm":
		switch variant {
		case "", "7":
			return "v7"
		case "5", "6", "8":
			return "v" + variant
		}
	}
	return variant
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
