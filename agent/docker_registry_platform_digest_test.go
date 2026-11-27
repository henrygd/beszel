//go:build testing

package agent

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/require"
)

func TestCheckImageUpdateSinglePlatformDigest(t *testing.T) {
	child := registryDigest('a')
	other := registryDigest('b')
	descriptor := func(d, arch, variant, mediaType string) string {
		return fmt.Sprintf(`{"mediaType":%q,"digest":%q,"size":123,"platform":{"os":"linux","architecture":%q,"variant":%q}}`, mediaType, d, arch, variant)
	}
	arm := descriptor(child, "arm64", "v8", ociImageManifest)
	amd := descriptor(other, "amd64", "", ociImageManifest)
	for _, tc := range []struct {
		name, arch, variant, local, entries, mediaType          string
		rootMatch, corrupt, failGET, wrongHeader, want, wantErr bool
		get                                                     int
	}{
		{name: "arm64 child", arch: "arm64", local: child, entries: arm + "," + amd, mediaType: ociImageIndex, get: 1},
		{name: "amd64 child", arch: "amd64", local: other, entries: arm + "," + amd, mediaType: ociImageIndex, get: 1},
		{name: "docker manifest list", arch: "arm64", local: child, entries: descriptor(child, "arm64", "", dockerImageManifest), mediaType: dockerManifestList, get: 1},
		{name: "root match fast path", arch: "arm64", local: child, entries: arm, mediaType: ociImageIndex, rootMatch: true},
		{name: "changed arm64 child despite matching amd64", arch: "arm64", local: other, entries: arm + "," + amd, mediaType: ociImageIndex, want: true, get: 1},
		{name: "changed amd64 child", arch: "amd64", local: child, entries: arm + "," + amd, mediaType: ociImageIndex, want: true, get: 1},
		{name: "explicit variant mismatch", arch: "arm64", variant: "v9", local: child, entries: arm + "," + descriptor(other, "arm64", "v9", ociImageManifest), mediaType: ociImageIndex, want: true, get: 1},
		{name: "default arm64 variant", arch: "arm64", variant: "v8", local: child, entries: descriptor(child, "arm64", "", ociImageManifest), mediaType: ociImageIndex, get: 1},
		{name: "default arm variant", arch: "arm", variant: "v7", local: child, entries: descriptor(child, "arm", "", ociImageManifest), mediaType: ociImageIndex, get: 1},
		{name: "arm variant alias", arch: "arm", variant: "6", local: child, entries: descriptor(child, "arm", "v6", ociImageManifest), mediaType: ociImageIndex, get: 1},
		{name: "arm64 baseline alias", arch: "arm64", variant: "v8.0", local: child, entries: arm, mediaType: ociImageIndex, get: 1},
		{name: "amd64 nonbaseline distinct", arch: "amd64", variant: "v3", local: other, entries: amd + "," + descriptor(child, "amd64", "v3", ociImageManifest), mediaType: ociImageIndex, want: true, get: 1},
		{name: "unknown variant ambiguous", arch: "arm64", local: child, entries: arm + "," + descriptor(other, "arm64", "v9", ociImageManifest), mediaType: ociImageIndex, wantErr: true, get: 1},
		{name: "missing local platform", local: child, entries: arm, mediaType: ociImageIndex, wantErr: true},
		{name: "only other platform", arch: "amd64", local: child, entries: arm, mediaType: ociImageIndex, wantErr: true, get: 1},
		{name: "invalid child digest", arch: "arm64", local: child, entries: descriptor("invalid", "arm64", "v8", ociImageManifest), mediaType: ociImageIndex, wantErr: true, get: 1},
		{name: "attestation ignored", arch: "arm64", local: other, entries: arm + "," + descriptor(other, "unknown", "", ociImageManifest), mediaType: ociImageIndex, want: true, get: 1},
		{name: "nested ambiguous mismatch", arch: "arm64", local: other, entries: arm + "," + descriptor(other, "arm64", "", ociImageIndex), mediaType: ociImageIndex, wantErr: true, get: 1},
		{name: "corrupt body", arch: "arm64", local: child, entries: arm, mediaType: ociImageIndex, corrupt: true, wantErr: true, get: 1},
		{name: "wrong GET digest header", arch: "arm64", local: child, entries: arm, mediaType: ociImageIndex, wrongHeader: true, wantErr: true, get: 1},
		{name: "GET error", arch: "arm64", local: child, entries: arm, mediaType: ociImageIndex, failGET: true, wantErr: true, get: 1},
		{name: "single manifest mismatch", arch: "arm64", local: child, mediaType: ociImageManifest, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"schemaVersion":2,"mediaType":%q,"manifests":[%s]}`, tc.mediaType, tc.entries)
			root := digest.FromString(body).String()
			local := tc.local
			if tc.rootMatch {
				local = root
			}
			inspect := fmt.Sprintf(`{"Os":"linux","Architecture":%q,"Variant":%q,"RepoDigests":["docker.io/library/alpine@%s"]}`, tc.arch, tc.variant, local)
			gets, heads, auth := 0, 0, 0
			dm := newRegistryChecker(t, inspect, registryTransportFunc(func(req *http.Request) (*http.Response, error) {
				switch {
				case req.Method == http.MethodGet && req.URL.Host == "auth.docker.io":
					auth++
					return registryResponse(http.StatusOK, `{"token":"test-token"}`), nil
				case req.Method == http.MethodHead && req.URL.Host == "registry-1.docker.io" && req.URL.Path == "/v2/library/alpine/manifests/latest":
					heads++
					resp := registryResponse(http.StatusOK, "")
					resp.Header.Set("Content-Type", tc.mediaType)
					resp.Header.Set("Docker-Content-Digest", root)
					return resp, nil
				case req.Method == http.MethodGet && req.URL.Host == "registry-1.docker.io" && req.URL.Path == "/v2/library/alpine/manifests/"+root:
					gets++
					require.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
					require.Equal(t, imageManifestAccept, req.Header.Get("Accept"))
					if tc.failGET {
						return registryResponse(http.StatusServiceUnavailable, ""), nil
					}
					payload := body
					if tc.corrupt {
						payload += " "
					}
					resp := registryResponse(http.StatusOK, payload)
					header := root
					if tc.wrongHeader {
						header = other
					}
					resp.Header.Set("Docker-Content-Digest", header)
					resp.Header.Set("Content-Type", tc.mediaType)
					return resp, nil
				default:
					t.Errorf("unexpected request %s %s", req.Method, req.URL)
					return registryResponse(http.StatusNotFound, ""), nil
				}
			}))
			available, err := dm.checkImageUpdate("alpine")
			if tc.wantErr {
				require.Error(t, err)
				require.False(t, available)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, available)
			}
			require.Equal(t, 1, auth)
			require.Equal(t, 1, heads)
			require.Equal(t, tc.get, gets)
		})
	}
}

// A valid content digest does not imply that the index is valid JSON or that
// its size is safe to read. These fixtures make both error paths explicit.
func TestCheckImageUpdateRejectsInvalidIndexPayload(t *testing.T) {
	child := registryDigest('a')
	for _, tc := range []struct{ name, body, want string }{
		{"malformed JSON", `{"schemaVersion":`, "decode image index"},
		{"unsupported schema", `{"schemaVersion":1,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[]}`, "invalid image index"},
		{"oversized index", string(make([]byte, maxImageIndexSize+1)), "image index exceeds size limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := digest.FromString(tc.body).String()
			inspect := fmt.Sprintf(`{"Os":"linux","Architecture":"arm64","RepoDigests":["alpine@%s"]}`, child)
			gets := 0
			dm := newRegistryChecker(t, inspect, registryTransportFunc(func(req *http.Request) (*http.Response, error) {
				switch {
				case req.Method == http.MethodGet && req.URL.Host == "auth.docker.io":
					return registryResponse(http.StatusOK, `{"token":"test-token"}`), nil
				case req.Method == http.MethodHead && req.URL.Host == "registry-1.docker.io" && req.URL.Path == "/v2/library/alpine/manifests/latest":
					resp := registryResponse(http.StatusOK, "")
					resp.Header.Set("Docker-Content-Digest", root)
					resp.Header.Set("Content-Type", ociImageIndex)
					return resp, nil
				case req.Method == http.MethodGet && req.URL.Host == "registry-1.docker.io" && req.URL.Path == "/v2/library/alpine/manifests/"+root:
					gets++
					require.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
					resp := registryResponse(http.StatusOK, tc.body)
					resp.Header.Set("Docker-Content-Digest", root)
					resp.Header.Set("Content-Type", ociImageIndex)
					return resp, nil
				default:
					t.Errorf("unexpected request %s %s", req.Method, req.URL)
					return registryResponse(http.StatusNotFound, ""), nil
				}
			}))
			available, err := dm.checkImageUpdate("alpine")
			require.ErrorContains(t, err, tc.want)
			require.False(t, available)
			require.Equal(t, 1, gets)
		})
	}
}

func TestCompatibleImageVariant(t *testing.T) {
	for _, tc := range []struct {
		arch, local, remote string
		want                bool
	}{
		{"amd64", "v1", "", true},
		{"amd64", "v2", "v2", true},
		{"amd64", "v2", "", false},
		{"amd64", "v3", "v2", false},
		{"amd64", "v4", "v4", true},
		{"arm", "v5", "5", true},
		{"arm", "v6", "6", true},
		{"arm", "v7", "", true},
		{"arm", "v8", "8", true},
		{"arm", "v6", "v7", false},
		{"arm64", "v8", "", true},
		{"arm64", "v8.0", "8", true},
		{"arm64", "v8.1", "v8", false},
		{"arm64", "v9.0", "v9", true},
		{"arm64", "v9", "v8", false},
		{"ppc64le", "power8", "power8", true},
		{"ppc64le", "power9", "power8", false},
		{"ppc64le", "power8", "", false},
		{"riscv64", "rva20u64", "rva20u64", true},
		{"riscv64", "rva23u64", "rva22u64", false},
		{"riscv64", "rva20u64", "", false},
		{"s390x", "", "", true},
		{"amd64", "", "v3", true},
	} {
		t.Run(tc.arch+"/"+tc.local+"/"+tc.remote, func(t *testing.T) {
			require.Equal(t, tc.want, compatibleImageVariant(tc.arch, tc.local, tc.remote))
		})
	}
}
