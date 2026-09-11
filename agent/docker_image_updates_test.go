//go:build testing

package agent

import (
	"encoding/json"
	"fmt"
	"github.com/fxamacker/cbor/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/stretchr/testify/require"
)

func waitForImageUpdates(t *testing.T, dm *dockerManager) {
	t.Helper()
	require.Eventually(t, func() bool {
		dm.imageUpdatesMutex.RLock()
		defer dm.imageUpdatesMutex.RUnlock()
		return !dm.imageUpdatesRunning
	}, time.Second*3, time.Millisecond)
}

func TestImageUpdateCacheAndStats(t *testing.T) {
	local := "sha256:" + strings.Repeat("a", 64)
	remote := "sha256:" + strings.Repeat("b", 64)
	var inspections, lookups atomic.Int32
	var fail atomic.Bool
	var upToDate atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/images/"):
			inspections.Add(1)
			fmt.Fprintf(w, `{"RepoDigests":["docker.io/library/nginx@%s"]}`, local)
		case r.URL.Path == "/containers/json":
			fmt.Fprint(w, `[{"Id":"aaaaaaaaaaaa","Names":["/one"],"Image":"nginx","Status":"Up 2 hours"},{"Id":"bbbbbbbbbbbb","Names":["/two"],"Image":"docker.io/library/nginx:latest","Status":"Up 2 hours"}]`)
		case strings.Contains(r.URL.Path, "/stats"):
			fmt.Fprint(w, `{"memory_stats":{"usage":1048576},"cpu_stats":{},"networks":{}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	dm := newDockerManagerForVersionTest(server)
	dm.dockerVersionChecked = true
	dm.registryClient = &http.Client{Timeout: time.Second, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if fail.Load() {
			return nil, fmt.Errorf("registry unavailable")
		}
		response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"token":"test"}`))}
		if r.Method == http.MethodHead {
			lookups.Add(1)
			digest := remote
			if upToDate.Load() {
				digest = local
			}
			response.Header.Set("Docker-Content-Digest", digest)
		}
		return response, nil
	})}
	stats, err := dm.getDockerStats(defaultCacheTimeMs)
	require.NoError(t, err)
	require.Len(t, stats, 2)
	waitForImageUpdates(t, dm)
	require.EqualValues(t, 1, lookups.Load())
	require.EqualValues(t, 1, inspections.Load())
	stats, err = dm.getDockerStats(defaultCacheTimeMs)
	require.NoError(t, err)
	for _, stat := range stats {
		require.True(t, stat.UpdateAvailable)
		if stat.Id == "aaaaaaaaaaaa" {
			require.Equal(t, "nginx", stat.Image)
		} else {
			require.Equal(t, "docker.io/library/nginx:latest", stat.Image)
		}
	}
	require.EqualValues(t, 1, lookups.Load())

	expire := func() {
		dm.imageUpdatesMutex.Lock()
		dm.imageUpdates["docker.io/library/nginx:latest"].checkedAt = time.Now().Add(-imageUpdateInterval)
		dm.imageUpdatesMutex.Unlock()
	}
	upToDate.Store(true)
	expire()
	_, err = dm.getDockerStats(defaultCacheTimeMs)
	require.NoError(t, err)
	waitForImageUpdates(t, dm)
	require.EqualValues(t, 2, lookups.Load())
	require.False(t, dm.cachedImageUpdate("nginx:latest"))

	// An expired positive result is cleared on failure, and the failure itself
	// is cached so realtime stats do not retry a broken registry every second.
	dm.imageUpdatesMutex.Lock()
	dm.imageUpdates["docker.io/library/nginx:latest"].available = true
	dm.imageUpdatesMutex.Unlock()
	fail.Store(true)
	expire()
	_, err = dm.getDockerStats(defaultCacheTimeMs)
	require.NoError(t, err)
	waitForImageUpdates(t, dm)
	failedInspections := inspections.Load()
	stats, err = dm.getDockerStats(defaultCacheTimeMs)
	require.NoError(t, err)
	require.Len(t, stats, 2)
	require.Equal(t, failedInspections, inspections.Load())
	for _, stat := range stats {
		require.False(t, stat.UpdateAvailable)
		require.Equal(t, 1.0, stat.Mem)
	}
}

func TestImageDiscoveryDoesNotBlockStats(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/images/") {
			fmt.Fprintf(w, `{"RepoDigests":["example.com/app@sha256:%s"]}`, strings.Repeat("a", 64))
		} else {
			fmt.Fprint(w, `{"memory_stats":{"usage":1048576}}`)
		}
	}))
	defer server.Close()
	dm := newDockerManagerForVersionTest(server)
	defer func() { close(release); waitForImageUpdates(t, dm) }()
	dm.registryClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		started <- struct{}{}
		<-release
		return nil, fmt.Errorf("timeout")
	})}
	ctr := &container.ApiInfo{IdShort: "aaaaaaaaaaaa", Image: "example.com/app", Names: []string{"/one"}}
	dm.refreshImageUpdates([]*container.ApiInfo{ctr}, time.Now())
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("check did not start")
	}
	done := make(chan error, 1)
	go func() { done <- dm.updateContainerStats(ctr, defaultCacheTimeMs) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("registry blocked stats")
	}
	dm.imageUpdatesMutex.RLock()
	require.True(t, dm.imageUpdatesRunning)
	dm.imageUpdatesMutex.RUnlock()
}

func TestNormalizeImageUpdateReferences(t *testing.T) {
	require.Equal(t, normalizedImageReference("nginx"), normalizedImageReference("docker.io/library/nginx:latest"))
	require.Empty(t, normalizedImageReference("bad reference"))
	require.Empty(t, normalizedImageReference("nginx@sha256:"+strings.Repeat("a", 64)))
}

// A stats request can return headers promptly and then stall while reading its
// body. The stats-map mutex must remain available during that read.
func TestStatsResponseBodyDoesNotHoldStatsLock(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		<-release
		fmt.Fprint(w, `{"memory_stats":{"usage":1048576}}`)
	}))
	defer server.Close()
	dm := newDockerManagerForVersionTest(server)
	done := make(chan error, 1)
	go func() {
		done <- dm.updateContainerStats(&container.ApiInfo{IdShort: "aaaaaaaaaaaa", Names: []string{"/one"}, Image: "nginx"}, defaultCacheTimeMs)
	}()
	<-started
	locked := make(chan struct{})
	go func() { dm.containerStatsMutex.Lock(); dm.containerStatsMutex.Unlock(); close(locked) }()
	select {
	case <-locked:
	case <-time.After(time.Second):
		close(release)
		<-done
		t.Fatal("Docker response body held the stats mutex")
	}
	close(release)
	require.NoError(t, <-done)
}

func TestImageUpdateStatsEncoding(t *testing.T) {
	original := container.Stats{Image: "nginx:latest", UpdateAvailable: true}
	encoded, err := cbor.Marshal(original)
	require.NoError(t, err)
	var fields map[int]any
	require.NoError(t, cbor.Unmarshal(encoded, &fields))
	require.Equal(t, true, fields[11])
	require.Equal(t, "nginx:latest", fields[8])
	var decoded container.Stats
	require.NoError(t, cbor.Unmarshal(encoded, &decoded))
	require.True(t, decoded.UpdateAvailable)
	require.Equal(t, original.Image, decoded.Image)
	encoded, err = json.Marshal(original)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"u":true`)
}

func TestImageUpdateCacheExpiryBoundaryAndPruning(t *testing.T) {
	now := time.Now()
	key := normalizedImageReference("nginx")
	dm := &dockerManager{imageUpdates: map[string]*imageUpdateStatus{
		key:                           {available: true, checkedAt: now},
		"unused.example/image:latest": {checkedAt: now},
	}}
	dm.refreshImageUpdates([]*container.ApiInfo{{Image: "nginx"}}, now.Add(imageUpdateInterval-time.Nanosecond))
	require.False(t, dm.imageUpdatesRunning)
	require.Len(t, dm.imageUpdates, 1)
	require.True(t, dm.cachedImageUpdate("nginx:latest"))
	dm.refreshImageUpdates(nil, now)
	require.Empty(t, dm.imageUpdates)
}
