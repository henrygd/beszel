//go:build testing

package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckCertReadsUnverifiedLeaf(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	// httptest uses a self-signed certificate, which must still be reported.
	info, err := checkCert(context.Background(), server.URL)
	require.NoError(t, err)
	leaf := server.Certificate()
	assert.Equal(t, leaf.NotAfter.UnixMilli(), info.Expires)
	assert.Equal(t, leaf.Issuer.CommonName, info.Issuer)
}

func TestCertAddress(t *testing.T) {
	tests := []struct {
		target, address, host string
		wantErr               bool
	}{
		{target: "https://example.com", address: "example.com:443", host: "example.com"},
		{target: "https://example.com:8443/path?q=1", address: "example.com:8443", host: "example.com"},
		{target: "HTTPS://[::1]:9443", address: "[::1]:9443", host: "::1"},
		{target: "http://example.com", wantErr: true},
		{target: "https://", wantErr: true},
	}
	for _, tt := range tests {
		address, host, err := certAddress(tt.target)
		if tt.wantErr {
			assert.Error(t, err, tt.target)
			continue
		}
		require.NoError(t, err, tt.target)
		assert.Equal(t, tt.address, address)
		assert.Equal(t, tt.host, host)
	}
}

func TestRefreshCertCadence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		task := newMonitorTask(monitor.Config{ID: "test", Target: "https://example.test", Protocol: "http", CheckCert: true})
		defer task.cancel()
		var calls int
		var fail error
		// Far enough out that the regular interval applies for the whole test.
		expires := time.Now().Add(365 * 24 * time.Hour).UnixMilli()
		check := func(context.Context, string) (monitor.CertInfo, error) {
			calls++
			if fail != nil {
				return monitor.CertInfo{}, fail
			}
			return monitor.CertInfo{Expires: expires + int64(calls)}, nil
		}

		task.refreshCert(check)
		require.NotNil(t, task.certInfo())
		assert.Equal(t, expires+1, task.certInfo().Expires)

		// Not due again until the check interval passes.
		time.Sleep(certCheckInterval - time.Second)
		task.refreshCert(check)
		assert.Equal(t, 1, calls)
		time.Sleep(time.Second)
		task.refreshCert(check)
		assert.Equal(t, 2, calls)

		// Failures keep the last known certificate and retry sooner.
		fail = errors.New("connection refused")
		time.Sleep(certCheckInterval)
		task.refreshCert(check)
		assert.Equal(t, 3, calls)
		assert.Equal(t, expires+2, task.certInfo().Expires)
		time.Sleep(certCheckRetryInterval)
		fail = nil
		task.refreshCert(check)
		assert.Equal(t, 4, calls)
		assert.Equal(t, expires+4, task.certInfo().Expires)
	})
}

func TestRefreshCertRetriesSoonerNearExpiry(t *testing.T) {
	for _, tc := range []struct {
		name     string
		expires  time.Duration // relative to the check
		interval time.Duration
	}{
		{"expired", -time.Hour, certCheckRetryInterval},
		{"expires before next regular check", certCheckInterval - time.Minute, certCheckRetryInterval},
		{"expires after next regular check", certCheckInterval + time.Minute, certCheckInterval},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				task := newMonitorTask(monitor.Config{ID: "test", Target: "https://example.test", Protocol: "http", CheckCert: true})
				defer task.cancel()
				var calls int
				check := func(context.Context, string) (monitor.CertInfo, error) {
					calls++
					return monitor.CertInfo{Expires: time.Now().Add(tc.expires).UnixMilli()}, nil
				}
				task.refreshCert(check)
				time.Sleep(tc.interval - time.Second)
				task.refreshCert(check)
				assert.Equal(t, 1, calls)
				time.Sleep(time.Second)
				task.refreshCert(check)
				assert.Equal(t, 2, calls)
			})
		})
	}
}

func TestRefreshCertDisabled(t *testing.T) {
	task := newMonitorTask(monitor.Config{ID: "test", Target: "https://example.test", Protocol: "http"})
	defer task.cancel()
	task.refreshCert(func(context.Context, string) (monitor.CertInfo, error) {
		t.Fatal("certificate check must not run when disabled")
		return monitor.CertInfo{}, nil
	})
	assert.Nil(t, task.certInfo())
}

func TestUpsertMonitorRunNowIncludesCert(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	pm := newMonitorManagerWithProbe(func(context.Context, monitor.Config) (int64, error) { return 100, nil })
	defer pm.Stop()
	config := monitor.Config{ID: "cert", Target: server.URL, Protocol: "http", Interval: 60, CheckCert: true}
	result, err := pm.UpsertMonitor(config, true)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Cert)
	assert.Equal(t, server.Certificate().NotAfter.UnixMilli(), result.Cert.Expires)

	// Realtime results never carry the certificate, and the default interval
	// sends it only once per check.
	assert.Nil(t, pm.GetResults(1000)["cert"].Cert)
	results := pm.GetResults(defaultDataCacheTimeMs)
	require.NotNil(t, results["cert"].Cert)
	assert.Equal(t, result.Cert.Expires, results["cert"].Cert.Expires)
	assert.Nil(t, pm.GetResults(defaultDataCacheTimeMs)["cert"].Cert)

	// Changing the interval keeps the known certificate without resending it;
	// disabling drops it.
	config.Interval = 30
	_, err = pm.UpsertMonitor(config, false)
	require.NoError(t, err)
	pm.mu.RLock()
	task := pm.monitors["cert"]
	pm.mu.RUnlock()
	assert.NotNil(t, task.certInfo())
	assert.Nil(t, pm.GetResults(defaultDataCacheTimeMs)["cert"].Cert)
	config.CheckCert = false
	_, err = pm.UpsertMonitor(config, false)
	require.NoError(t, err)
	pm.mu.RLock()
	task = pm.monitors["cert"]
	pm.mu.RUnlock()
	assert.Nil(t, task.certInfo())
}
