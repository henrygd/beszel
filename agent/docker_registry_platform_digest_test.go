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
	old := registryDigest('c')
	descriptor := func(d, arch, variant, mediaType string) string {
		return fmt.Sprintf(`{"mediaType":%q,"digest":%q,"size":123,"platform":{"os":"linux","architecture":%q,"variant":%q}}`, mediaType, d, arch, variant)
	}
	arm := descriptor(child, "arm64", "v8", ociImageManifest)
	amd := descriptor(other, "amd64", "", ociImageManifest)
	armV6 := descriptor(child, "arm", "v6", ociImageManifest)
	armV7 := descriptor(other, "arm", "v7", ociImageManifest)
	armV9 := descriptor(other, "arm64", "v9", ociImageManifest)
	amdV3 := descriptor(child, "amd64", "v3", ociImageManifest)
	invalidARM := descriptor("invalid", "arm", "v7", ociImageManifest)
	windows := func(d, version string) string {
		return fmt.Sprintf(`{"mediaType":%q,"digest":%q,"size":123,"platform":{"os":"windows","architecture":"amd64","os.version":%q}}`, dockerImageManifest, d, version)
	}
	winOld := windows(child, "10.0.17763.1")
	winNew := windows(other, "10.0.20348.1")
	missingPlatform := func(d string) string {
		return fmt.Sprintf(`{"mediaType":%q,"digest":%q,"size":123}`, ociImageManifest, d)
	}
	missingChild, missingOther := missingPlatform(child), missingPlatform(other)
	featureDescriptor := func(d, requirements string) string {
		return fmt.Sprintf(`{"mediaType":%q,"digest":%q,"size":123,"platform":{"os":"linux","architecture":"amd64",%s}}`, ociImageManifest, d, requirements)
	}
	nestedMissingPlatform := fmt.Sprintf(`{"mediaType":%q,"digest":%q,"size":123}`, ociImageIndex, child)
	nestedWindows := fmt.Sprintf(`{"mediaType":%q,"digest":%q,"size":123,"platform":{"os":"windows","architecture":"amd64"}}`, ociImageIndex, child)
	for _, tc := range []struct {
		name, arch, variant, local, entries, mediaType, os, osVersion string
		extraLocal, errContains                                       string
		rootMatch, corrupt, failGET, wrongHeader, want, wantErr       bool
		get                                                           int
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
		{name: "unknown variant ambiguous", arch: "arm64", local: child, entries: arm + "," + descriptor(other, "arm64", "v9", ociImageManifest), mediaType: ociImageIndex, want: true, get: 1},
		{name: "review arm unknown variant all changed", arch: "arm", local: old, entries: armV6 + "," + armV7, mediaType: ociImageIndex, want: true, get: 1},
		{name: "review arm unknown variant all changed reversed", arch: "arm", local: old, entries: armV7 + "," + armV6, mediaType: ociImageIndex, want: true, get: 1},
		{name: "review arm unknown variant first matches", arch: "arm", local: child, entries: armV6 + "," + armV7, mediaType: ociImageIndex, want: true, get: 1},
		{name: "review arm unknown variant last matches", arch: "arm", local: child, entries: armV7 + "," + armV6, mediaType: ociImageIndex, want: true, get: 1},
		{name: "review arm matching second repo digest", arch: "arm", local: old, extraLocal: child, entries: armV7 + "," + armV6, mediaType: ociImageIndex, want: true, get: 1},
		{name: "review arm explicit variant changed", arch: "arm", variant: "v7", local: child, entries: armV6 + "," + armV7, mediaType: ociImageIndex, want: true, get: 1},
		{name: "review arm explicit variant unchanged", arch: "arm", variant: "v6", local: child, entries: armV6 + "," + armV7, mediaType: ociImageIndex, get: 1},
		{name: "review arm64 unknown variant all changed", arch: "arm64", local: old, entries: arm + "," + armV9, mediaType: ociImageIndex, want: true, get: 1},
		{name: "review amd64 unknown variant all changed", arch: "amd64", local: old, entries: amd + "," + amdV3, mediaType: ociImageIndex, want: true, get: 1},
		{name: "review duplicate candidate unchanged", arch: "arm", local: child, entries: armV6 + "," + armV6, mediaType: ociImageIndex, get: 1},
		{name: "review duplicate candidate changed", arch: "arm", local: old, entries: armV6 + "," + armV6, mediaType: ociImageIndex, want: true, get: 1},
		{name: "review ambiguous with incompatible nested index", arch: "arm", local: old, entries: armV6 + "," + armV7 + "," + descriptor(child, "amd64", "", ociImageIndex), mediaType: ociImageIndex, want: true, get: 1},
		{name: "review ambiguous with unresolved nested index", arch: "arm", local: old, entries: armV6 + "," + armV7 + "," + nestedMissingPlatform, mediaType: ociImageIndex, wantErr: true, get: 1},
		{name: "review ambiguous with possible nested index first", arch: "arm", local: old, entries: descriptor(child, "arm", "v6", ociImageIndex) + "," + armV6 + "," + armV7, mediaType: ociImageIndex, wantErr: true, get: 1},
		{name: "review ambiguous with invalid digest last", arch: "arm", local: old, entries: armV6 + "," + armV7 + "," + invalidARM, mediaType: ociImageIndex, wantErr: true, errContains: "invalid platform manifest digest", get: 1},
		{name: "review Windows ambiguous with invalid digest last", os: "windows", arch: "amd64", local: old, entries: winOld + "," + winNew + "," + windows("invalid", "10.0.17763.1"), mediaType: dockerManifestList, wantErr: true, errContains: "invalid platform manifest digest", get: 1},
		{name: "review ambiguous root match fast path", arch: "arm", local: old, entries: armV6 + "," + armV7, mediaType: ociImageIndex, rootMatch: true},
		{name: "missing local platform", local: child, entries: arm, mediaType: ociImageIndex, wantErr: true},
		{name: "only other platform", arch: "amd64", local: child, entries: arm, mediaType: ociImageIndex, wantErr: true, get: 1},
		{name: "invalid child digest", arch: "arm64", local: child, entries: descriptor("invalid", "arm64", "v8", ociImageManifest), mediaType: ociImageIndex, wantErr: true, get: 1},
		{name: "attestation ignored", arch: "arm64", local: other, entries: arm + "," + descriptor(other, "unknown", "", ociImageManifest), mediaType: ociImageIndex, want: true, get: 1},
		{name: "nested ambiguous mismatch", arch: "arm64", local: other, entries: arm + "," + descriptor(other, "arm64", "", ociImageIndex), mediaType: ociImageIndex, wantErr: true, get: 1},
		{name: "review nested other architecture", arch: "amd64", local: child, entries: amd + "," + descriptor(child, "arm64", "", ociImageIndex), mediaType: ociImageIndex, want: true, get: 1},
		{name: "review nested other OS", arch: "amd64", local: child, entries: amd + "," + nestedWindows, mediaType: ociImageIndex, want: true, get: 1},
		{name: "review nested other explicit variant", arch: "amd64", variant: "v1", local: child, entries: amd + "," + descriptor(child, "amd64", "v3", ociImageIndex), mediaType: ociImageIndex, want: true, get: 1},
		{name: "review nested unknown platform", arch: "amd64", local: child, entries: amd + "," + nestedMissingPlatform, mediaType: ociImageIndex, wantErr: true, get: 1},
		{name: "review nested possible matching variant", arch: "amd64", variant: "v3", local: child, entries: descriptor(other, "amd64", "v3", ociImageManifest) + "," + descriptor(child, "amd64", "", ociImageIndex), mediaType: ociImageIndex, wantErr: true, get: 1},
		{name: "review nested matching child", arch: "amd64", local: other, entries: amd + "," + nestedMissingPlatform, mediaType: ociImageIndex, get: 1},
		{name: "review nested precedes matching child", arch: "amd64", local: other, entries: descriptor(child, "amd64", "", ociImageIndex) + "," + amd, mediaType: ociImageIndex, wantErr: true, errContains: "image index contains unresolved nested manifests", get: 1},
		{name: "review nested unknown precedes matching child", arch: "amd64", local: other, entries: nestedMissingPlatform + "," + amd, mediaType: ociImageIndex, wantErr: true, errContains: "image index contains unresolved nested manifests", get: 1},
		{name: "review incompatible nested precedes matching child", arch: "amd64", local: other, entries: descriptor(child, "arm64", "", ociImageIndex) + "," + amd, mediaType: ociImageIndex, get: 1},
		{name: "review incompatible direct then nested then matching child", arch: "amd64", local: other, entries: arm + "," + nestedMissingPlatform + "," + amd, mediaType: ociImageIndex, wantErr: true, errContains: "image index contains unresolved nested manifests", get: 1},
		{name: "review nested follows matching child", arch: "amd64", local: other, entries: amd + "," + descriptor(child, "amd64", "", ociImageIndex), mediaType: ociImageIndex, get: 1},
		{name: "review nested precedes duplicate matching children", arch: "amd64", local: other, entries: nestedMissingPlatform + "," + amd + "," + amd, mediaType: ociImageIndex, wantErr: true, errContains: "image index contains unresolved nested manifests", get: 1},
		{name: "review nested between duplicate matching children", arch: "amd64", local: other, entries: amd + "," + nestedMissingPlatform + "," + amd, mediaType: ociImageIndex, get: 1},
		{name: "review nested preceding root match fast path", arch: "amd64", local: other, entries: nestedMissingPlatform + "," + amd, mediaType: ociImageIndex, rootMatch: true},
		{name: "review ARM nested precedes matching child", arch: "arm", variant: "v6", local: child, entries: descriptor(old, "arm", "v6", ociImageIndex) + "," + armV6 + "," + armV7, mediaType: ociImageIndex, wantErr: true, errContains: "image index contains unresolved nested manifests", get: 1},
		{name: "review ARM incompatible variant nested first", arch: "arm", variant: "v6", local: child, entries: descriptor(old, "arm", "v7", ociImageIndex) + "," + armV6, mediaType: ociImageIndex, get: 1},
		{name: "review omitted platform changed", arch: "arm64", local: old, entries: missingChild, mediaType: ociImageIndex, want: true, get: 1},
		{name: "review null platform treated as omitted", arch: "arm64", local: old, entries: fmt.Sprintf(`{"mediaType":%q,"digest":%q,"size":123,"platform":null}`, ociImageManifest, child), mediaType: ociImageIndex, want: true, get: 1},
		{name: "review omitted platform unchanged", arch: "arm64", local: child, entries: missingChild, mediaType: ociImageIndex, get: 1},
		{name: "review omitted platforms all changed", arch: "arm64", local: old, entries: missingChild + "," + missingOther, mediaType: ociImageIndex, want: true, get: 1},
		{name: "review omitted platform duplicate unchanged", arch: "arm64", local: child, entries: missingChild + "," + missingChild, mediaType: ociImageIndex, get: 1},
		{name: "review omitted platforms all locally recorded", arch: "arm64", local: child, extraLocal: other, entries: missingChild + "," + missingOther, mediaType: ociImageIndex, get: 1},
		{name: "review omitted platforms mixed digests", arch: "arm64", local: child, entries: missingChild + "," + missingOther, mediaType: ociImageIndex, want: true, get: 1},
		{name: "review omitted platforms mixed digests reversed", arch: "arm64", local: child, entries: missingOther + "," + missingChild, mediaType: ociImageIndex, want: true, get: 1},
		{name: "review omitted platform and other architecture changed", arch: "arm64", local: old, entries: amd + "," + missingChild, mediaType: ociImageIndex, want: true, get: 1},
		{name: "review omitted platform and other architecture unchanged", arch: "arm64", local: child, entries: amd + "," + missingChild, mediaType: ociImageIndex, get: 1},
		{name: "review omitted platform precedes matching child", arch: "arm64", local: child, entries: missingOther + "," + arm, mediaType: ociImageIndex, want: true, get: 1},
		{name: "review omitted platform precedes identical child", arch: "arm64", local: child, entries: missingChild + "," + arm, mediaType: ociImageIndex, get: 1},
		{name: "review omitted platform precedes changed child", arch: "arm64", local: other, entries: missingOther + "," + arm, mediaType: ociImageIndex, want: true, get: 1},
		{name: "review omitted platform follows matching child", arch: "arm64", local: child, entries: arm + "," + missingOther, mediaType: ociImageIndex, get: 1},
		{name: "review omitted platform around matching child", arch: "arm64", local: child, entries: missingChild + "," + arm + "," + missingOther, mediaType: ociImageIndex, get: 1},
		{name: "review omitted platform then matching child and later nested", arch: "arm64", local: child, entries: missingChild + "," + arm + "," + nestedMissingPlatform, mediaType: ociImageIndex, get: 1},
		{name: "review omitted platform then nested before matching child", arch: "arm64", local: child, entries: missingChild + "," + nestedMissingPlatform + "," + arm, mediaType: ociImageIndex, wantErr: true, errContains: "image index contains unresolved nested manifests", get: 1},
		{name: "review omitted platform follows changed child", arch: "arm64", local: other, entries: arm + "," + missingOther, mediaType: ociImageIndex, want: true, get: 1},
		{name: "review omitted platform before incompatible nested", arch: "arm64", local: child, entries: missingChild + "," + descriptor(other, "amd64", "", ociImageIndex), mediaType: ociImageIndex, get: 1},
		{name: "review omitted platform before potential nested", arch: "arm64", local: child, entries: missingChild + "," + nestedMissingPlatform, mediaType: ociImageIndex, wantErr: true, errContains: "image index contains unresolved nested manifests", get: 1},
		{name: "review potential nested before omitted platform", arch: "arm64", local: child, entries: nestedMissingPlatform + "," + missingChild, mediaType: ociImageIndex, wantErr: true, errContains: "image index contains unresolved nested manifests", get: 1},
		{name: "review omitted platform invalid digest", arch: "arm64", local: child, entries: missingPlatform("invalid"), mediaType: ociImageIndex, wantErr: true, errContains: "invalid platform manifest digest", get: 1},
		{name: "review omitted platform invalid digest after match", arch: "arm64", local: child, entries: arm + "," + missingPlatform("invalid"), mediaType: ociImageIndex, wantErr: true, errContains: "invalid platform manifest digest", get: 1},
		{name: "review omitted platform index root fast path", arch: "arm64", local: child, entries: missingChild, mediaType: ociImageIndex, rootMatch: true},
		{name: "review Windows OCI omitted platform unchanged", os: "windows", arch: "amd64", local: child, entries: missingChild, mediaType: ociImageIndex, get: 1},
		{name: "review Windows OCI omitted platform changed", os: "windows", arch: "amd64", local: old, entries: missingChild, mediaType: ociImageIndex, want: true, get: 1},
		{name: "review missing platform Docker list", arch: "arm64", local: child, entries: fmt.Sprintf(`{"mediaType":%q,"digest":%q,"size":123}`, dockerImageManifest, child), mediaType: dockerManifestList, wantErr: true, get: 1},
		{name: "review OCI Docker child omitted platform", arch: "arm64", local: old, entries: fmt.Sprintf(`{"mediaType":%q,"digest":%q,"size":123}`, dockerImageManifest, child), mediaType: ociImageIndex, want: true, get: 1},
		{name: "review unknown media type without platform ignored", arch: "arm64", local: child, entries: fmt.Sprintf(`{"mediaType":"application/example","digest":%q,"size":123}`, other) + "," + arm, mediaType: ociImageIndex, get: 1},
		{name: "review partial platform not unqualified", arch: "arm64", local: child, entries: fmt.Sprintf(`{"mediaType":%q,"digest":%q,"size":123,"platform":{"os":"linux"}}`, ociImageManifest, child), mediaType: ociImageIndex, wantErr: true, get: 1},
		{name: "review explicit unknown attestation before matching child", arch: "arm64", local: child, entries: fmt.Sprintf(`{"mediaType":%q,"digest":%q,"size":123,"platform":{"os":"unknown","architecture":"unknown"}}`, ociImageManifest, other) + "," + arm, mediaType: ociImageIndex, get: 1},
		{name: "review invalid nested digest after match", arch: "arm64", local: child, entries: arm + "," + descriptor("invalid", "arm64", "", ociImageIndex), mediaType: ociImageIndex, wantErr: true, errContains: "invalid nested index digest", get: 1},
		{name: "review invalid incompatible nested digest ignored", arch: "arm64", local: child, entries: descriptor("invalid", "amd64", "", ociImageIndex) + "," + arm, mediaType: ociImageIndex, get: 1},
		{name: "review Windows nested precedes exact child", os: "windows", arch: "amd64", osVersion: "10.0.17763.1", local: child, entries: nestedWindows + "," + winOld + "," + winNew, mediaType: dockerManifestList, wantErr: true, errContains: "image index contains unresolved nested manifests", get: 1},
		{name: "review Windows exact child", os: "windows", arch: "amd64", osVersion: "10.0.17763.1", local: child, entries: winNew + "," + winOld, mediaType: dockerManifestList, get: 1},
		{name: "review Windows changed child", os: "windows", arch: "amd64", osVersion: "10.0.20348.1", local: child, entries: winOld + "," + winNew, mediaType: dockerManifestList, want: true, get: 1},
		{name: "review Windows missing local version", os: "windows", arch: "amd64", local: child, entries: winOld + "," + winNew, mediaType: dockerManifestList, want: true, get: 1},
		{name: "review Windows same build matching digest", os: "windows", arch: "amd64", osVersion: "10.0.17763.2", local: child, entries: winOld + "," + winNew, mediaType: dockerManifestList, get: 1},
		{name: "review Windows ambiguous missing descriptor version", os: "windows", arch: "amd64", osVersion: "10.0.17763.1", local: child, entries: winOld + "," + windows(other, ""), mediaType: dockerManifestList, want: true, get: 1},
		{name: "review Windows single child no version", os: "windows", arch: "amd64", local: child, entries: windows(child, ""), mediaType: dockerManifestList, get: 1},
		{name: "review Windows root fast path", os: "windows", arch: "amd64", local: child, entries: winOld + "," + winNew, mediaType: dockerManifestList, rootMatch: true},
		{name: "review ARM explicit first changed", arch: "arm", local: other, entries: descriptor(old, "arm", "v7", ociImageManifest) + "," + armV7, mediaType: ociImageIndex, variant: "v7", want: true, get: 1},
		{name: "review ARM explicit first unchanged", arch: "arm", local: other, entries: armV7 + "," + descriptor(old, "arm", "v7", ociImageManifest), mediaType: ociImageIndex, variant: "v7", get: 1},
		{name: "review ARM unknown first changed", arch: "arm", local: other, entries: descriptor(old, "arm", "v7", ociImageManifest) + "," + armV7, mediaType: ociImageIndex, variant: "", want: true, get: 1},
		{name: "review ARM unknown first unchanged", arch: "arm", local: other, entries: armV7 + "," + descriptor(old, "arm", "v7", ociImageManifest), mediaType: ociImageIndex, variant: "", get: 1},
		{name: "review ARM alias first changed", arch: "arm", local: other, entries: descriptor(old, "arm", "", ociImageManifest) + "," + armV7, mediaType: ociImageIndex, variant: "v7", want: true, get: 1},
		{name: "review ARM64 alias first unchanged", arch: "arm64", local: child, entries: arm + "," + descriptor(other, "arm64", "8", ociImageManifest), mediaType: ociImageIndex, variant: "v8", get: 1},
		{name: "review AMD64 alias first changed", arch: "amd64", local: other, entries: descriptor(child, "amd64", "v1", ociImageManifest) + "," + amd, mediaType: ociImageIndex, variant: "v1", want: true, get: 1},
		{name: "review ARM repeated group invalid digest", arch: "arm", local: other, entries: armV7 + "," + invalidARM, mediaType: ociImageIndex, variant: "v7", wantErr: true, errContains: "invalid platform manifest digest", get: 1},
		{name: "review ARM different groups all current", arch: "arm", local: child, entries: armV6 + "," + armV7, mediaType: ociImageIndex, extraLocal: other, get: 1},
		{name: "review ARM noncontiguous first groups current", arch: "arm", local: child, entries: armV6 + "," + armV7 + "," + descriptor(old, "arm", "v6", ociImageManifest), mediaType: ociImageIndex, extraLocal: other, get: 1},
		{name: "review ARM noncontiguous first group changed", arch: "arm", local: child, entries: descriptor(old, "arm", "v6", ociImageManifest) + "," + armV7 + "," + armV6, mediaType: ociImageIndex, extraLocal: other, want: true, get: 1},
		{name: "review ARM all recorded unqualified first", arch: "arm", local: child, entries: missingChild + "," + armV6 + "," + armV7, mediaType: ociImageIndex, extraLocal: other, get: 1},
		{name: "review ARM mixed unqualified first", arch: "arm", local: child, entries: missingChild + "," + armV6 + "," + armV7, mediaType: ociImageIndex, want: true, get: 1},
		{name: "review feature sets first unchanged", arch: "amd64", local: child, entries: featureDescriptor(child, `"os.features":["a","b"],"features":["x","y"]`) + "," + featureDescriptor(other, `"os.features":["b","a"],"features":["y","x"]`), mediaType: ociImageIndex, get: 1},
		{name: "review feature sets first changed", arch: "amd64", local: child, entries: featureDescriptor(other, `"os.features":["b","a"],"features":["y","x"]`) + "," + featureDescriptor(child, `"os.features":["a","b"],"features":["x","y"]`), mediaType: ociImageIndex, want: true, get: 1},
		{name: "review different OS feature requirements", arch: "amd64", local: child, entries: featureDescriptor(child, `"os.features":["a","b"],"features":["x","y"]`) + "," + featureDescriptor(other, `"os.features":["a","c"],"features":["x","y"]`), mediaType: ociImageIndex, want: true, get: 1},
		{name: "review different CPU feature requirements", arch: "amd64", local: child, entries: featureDescriptor(other, `"os.features":["a","b"],"features":["x","z"]`) + "," + featureDescriptor(child, `"os.features":["a","b"],"features":["x","y"]`), mediaType: ociImageIndex, want: true, get: 1},
		{name: "review different feature groups all current", arch: "amd64", local: child, entries: featureDescriptor(child, `"os.features":["a","b"],"features":["x","y"]`) + "," + featureDescriptor(other, `"os.features":["a","c"],"features":["x","y"]`), mediaType: ociImageIndex, extraLocal: other, get: 1},
		{name: "review empty and omitted feature sets", arch: "amd64", local: other, entries: amd + "," + featureDescriptor(child, `"os.features":[],"features":[]`), mediaType: ociImageIndex, get: 1},
		{name: "review Windows newer revision first", arch: "amd64", local: child, entries: windows(other, "10.0.20348.2000") + "," + windows(child, "10.0.20348.1000"), mediaType: dockerManifestList, os: "windows", osVersion: "10.0.20348.1000", want: true, get: 1},
		{name: "review Windows newer revision last", arch: "amd64", local: child, entries: windows(child, "10.0.20348.1000") + "," + windows(other, "10.0.20348.2000"), mediaType: dockerManifestList, os: "windows", osVersion: "10.0.20348.1000", want: true, get: 1},
		{name: "review Windows same version first current", arch: "amd64", local: child, entries: windows(child, "10.0.20348.1000") + "," + windows(other, "10.0.20348.1000"), mediaType: dockerManifestList, os: "windows", osVersion: "10.0.20348.1000", get: 1},
		{name: "review Windows same version first changed", arch: "amd64", local: child, entries: windows(other, "10.0.20348.1000") + "," + windows(child, "10.0.20348.1000"), mediaType: dockerManifestList, os: "windows", osVersion: "10.0.20348.1000", want: true, get: 1},
		{name: "review Windows same build all current", arch: "amd64", local: child, entries: windows(other, "10.0.20348.2000") + "," + windows(child, "10.0.20348.1000"), mediaType: dockerManifestList, os: "windows", osVersion: "10.0.20348.1000", extraLocal: other, get: 1},
		{name: "review Windows invalid version cannot exclude", arch: "amd64", local: child, entries: windows(other, "unknown") + "," + windows(child, "10.0.20348.1000"), mediaType: dockerManifestList, os: "windows", osVersion: "10.0.20348.1000", want: true, get: 1},
		{name: "review Windows Windows 2016 revision unknown-host", arch: "amd64", local: child, entries: windows(other, "10.0.14393.2") + "," + windows(child, "10.0.14393.1"), mediaType: dockerManifestList, os: "windows", osVersion: "10.0.14393.1", want: true, get: 1},
		{name: "review Windows same build three components", arch: "amd64", local: child, entries: windows(other, "10.0.20348") + "," + windows(child, "10.0.20348.1000"), mediaType: dockerManifestList, os: "windows", osVersion: "10.0.20348.1000", want: true, get: 1},
		{name: "review Windows different build excluded", arch: "amd64", local: child, entries: winOld + "," + windows(child, "10.0.20348.1000"), mediaType: dockerManifestList, os: "windows", osVersion: "10.0.20348.1000", get: 1},
		{name: "review Windows same build invalid digest", arch: "amd64", local: child, entries: windows(child, "10.0.20348.1000") + "," + windows("invalid", "10.0.20348.2000"), mediaType: dockerManifestList, os: "windows", osVersion: "10.0.20348.1000", wantErr: true, errContains: "invalid platform manifest digest", get: 1},
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
			os := tc.os
			if os == "" {
				os = "linux"
			}
			repoDigests := fmt.Sprintf("%q", "docker.io/library/alpine@"+local)
			if tc.extraLocal != "" {
				repoDigests += "," + fmt.Sprintf("%q", "docker.io/library/alpine@"+tc.extraLocal)
			}
			inspect := fmt.Sprintf(`{"Os":%q,"Architecture":%q,"Variant":%q,"OsVersion":%q,"RepoDigests":[%s]}`, os, tc.arch, tc.variant, tc.osVersion, repoDigests)
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
				if tc.errContains != "" {
					require.ErrorContains(t, err, tc.errContains)
				}
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
		{"missing manifests", `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json"}`, "invalid image index"},
		{"null manifests", `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":null}`, "invalid image index"},
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
