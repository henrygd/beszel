package agent

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/dns/dnsmessage"
)

func TestMonitorManagerGetResultsIncludesHourResponseRange(t *testing.T) {
	now := time.Now().UTC()
	task := newMonitorTask(monitor.Config{ID: "monitor-1"})
	task.history.addSampleLocked(monitorSample{responseUs: 10, timestamp: now.Add(-30 * time.Minute)})
	task.history.addSampleLocked(monitorSample{responseUs: 20, timestamp: now.Add(-9 * time.Minute)})
	task.history.addSampleLocked(monitorSample{responseUs: 40, timestamp: now.Add(-5 * time.Minute)})
	task.history.addSampleLocked(monitorSample{responseUs: 30, timestamp: now.Add(-50 * time.Second)})
	task.history.addSampleLocked(monitorSample{responseUs: -1, timestamp: now.Add(-30 * time.Second)})

	pm := newMonitorManager()
	pm.monitors = map[string]*monitorTask{"icmp:example.com": task}

	results := pm.GetResults(uint16(time.Minute / time.Millisecond))
	result, ok := results["monitor-1"]
	require.True(t, ok)
	assert.Equal(t, int64(30), result.AvgResponse)
	assert.Equal(t, int64(25), result.AvgResponse1h)
	assert.Equal(t, int64(30), result.MinResponse)
	assert.Equal(t, int64(10), result.MinResponse1h)
	assert.Equal(t, int64(30), result.MaxResponse)
	assert.Equal(t, int64(40), result.MaxResponse1h)
	assert.Equal(t, 50.0, result.PacketLoss)
	assert.Equal(t, 20.0, result.PacketLoss1h)
}

func TestMonitorManagerGetResultsIncludesLossOnlyHourData(t *testing.T) {
	now := time.Now().UTC()
	task := newMonitorTask(monitor.Config{ID: "monitor-1"})
	task.history.addSampleLocked(monitorSample{responseUs: -1, timestamp: now.Add(-30 * time.Second)})
	task.history.addSampleLocked(monitorSample{responseUs: -1, timestamp: now.Add(-10 * time.Second)})

	pm := newMonitorManager()
	pm.monitors = map[string]*monitorTask{"icmp:example.com": task}

	results := pm.GetResults(uint16(time.Minute / time.Millisecond))
	result, ok := results["monitor-1"]
	require.True(t, ok)
	assert.Equal(t, int64(0), result.AvgResponse)
	assert.Equal(t, int64(0), result.AvgResponse1h)
	assert.Equal(t, int64(0), result.MinResponse)
	assert.Equal(t, int64(0), result.MinResponse1h)
	assert.Equal(t, int64(0), result.MaxResponse)
	assert.Equal(t, int64(0), result.MaxResponse1h)
	assert.Equal(t, 100.0, result.PacketLoss)
	assert.Equal(t, 100.0, result.PacketLoss1h)
}

func TestMonitorConfigResultKeyUsesSyncedID(t *testing.T) {
	cfg := monitor.Config{ID: "monitor-1", Target: "1.1.1.1", Protocol: "icmp", Interval: 10}
	assert.Equal(t, "monitor-1", cfg.ID)
}

func TestMonitorManagerSyncMonitorsSkipsConfigsWithoutStableID(t *testing.T) {
	validCfg := monitor.Config{ID: "monitor-1", Target: "ignored", Protocol: "noop", Interval: 10}
	invalidCfg := monitor.Config{Target: "ignored", Protocol: "noop", Interval: 10}

	pm := newMonitorManager()
	pm.SyncMonitors([]monitor.Config{validCfg, invalidCfg})
	defer pm.Stop()

	_, validExists := pm.monitors[validCfg.ID]
	_, invalidExists := pm.monitors[invalidCfg.ID]
	assert.True(t, validExists)
	assert.False(t, invalidExists)
}

func TestMonitorManagerSyncMonitorsStopsRemovedTasksButKeepsExisting(t *testing.T) {
	keepCfg := monitor.Config{ID: "monitor-1", Target: "ignored", Protocol: "noop", Interval: 10}
	removeCfg := monitor.Config{ID: "monitor-2", Target: "ignored", Protocol: "noop", Interval: 10}

	keptTask := newMonitorTask(keepCfg)
	removedTask := newMonitorTask(removeCfg)
	pm := newMonitorManager()
	pm.monitors = map[string]*monitorTask{
		keepCfg.ID:   keptTask,
		removeCfg.ID: removedTask,
	}

	pm.SyncMonitors([]monitor.Config{keepCfg})

	assert.Same(t, keptTask, pm.monitors[keepCfg.ID])
	_, exists := pm.monitors[removeCfg.ID]
	assert.False(t, exists)

	select {
	case <-removedTask.ctx.Done():
	default:
		t.Fatal("expected removed monitor task to be cancelled")
	}

	select {
	case <-keptTask.ctx.Done():
		t.Fatal("expected existing monitor task to remain active")
	default:
	}
}

func TestMonitorManagerSyncMonitorsRestartsChangedConfig(t *testing.T) {
	originalCfg := monitor.Config{ID: "monitor-1", Target: "ignored-a", Protocol: "noop", Interval: 10}
	updatedCfg := monitor.Config{ID: "monitor-1", Target: "ignored-b", Protocol: "noop", Interval: 10}
	originalTask := newMonitorTask(originalCfg)
	pm := newMonitorManager()
	pm.monitors = map[string]*monitorTask{
		originalCfg.ID: originalTask,
	}

	pm.SyncMonitors([]monitor.Config{updatedCfg})
	defer pm.Stop()

	restartedTask := pm.monitors[updatedCfg.ID]
	assert.NotSame(t, originalTask, restartedTask)
	assert.Equal(t, updatedCfg, restartedTask.config)

	select {
	case <-originalTask.ctx.Done():
	default:
		t.Fatal("expected changed monitor task to be cancelled")
	}
}

func TestMonitorManagerApplySyncUpsertRunsImmediatelyAndReturnsResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	pm := &MonitorManager{
		monitors: make(map[string]*monitorTask),
		probe:    networkMonitorProbe(server.Client()),
	}

	resp, err := pm.HandleSyncRequest(monitor.SyncRequest{
		Action: monitor.SyncActionUpsert,
		Config: monitor.Config{ID: "monitor-1", Target: server.URL, Protocol: "http", Interval: 10},
		RunNow: true,
	})
	defer pm.Stop()

	require.NoError(t, err)
	assert.GreaterOrEqual(t, resp.Result.AvgResponse, int64(0))
	assert.Equal(t, 0.0, resp.Result.PacketLoss)
	assert.Equal(t, 0.0, resp.Result.PacketLoss1h)

	task := pm.monitors["monitor-1"]
	require.NotNil(t, task)
	task.history.mu.Lock()
	defer task.history.mu.Unlock()
	require.Len(t, task.history.samples, 1)
}

func TestMonitorManagerUpsertMonitorKeepsHistoryWhenOnlyIntervalChanges(t *testing.T) {
	originalCfg := monitor.Config{ID: "monitor-1", Target: "1.1.1.1", Protocol: "icmp", Interval: 10}
	updatedCfg := monitor.Config{ID: "monitor-1", Target: "1.1.1.1", Protocol: "icmp", Interval: 30}
	now := time.Now().UTC()

	existingTask := newMonitorTask(originalCfg)
	existingTask.history.addSampleLocked(monitorSample{responseUs: 12, timestamp: now.Add(-50 * time.Minute)})
	existingTask.history.addSampleLocked(monitorSample{responseUs: 24, timestamp: now.Add(-30 * time.Second)})

	pm := newMonitorManager()
	pm.monitors = map[string]*monitorTask{originalCfg.ID: existingTask}

	result, err := pm.UpsertMonitor(updatedCfg, false)
	defer pm.Stop()

	require.NoError(t, err)
	assert.Nil(t, result)

	updatedTask := pm.monitors[updatedCfg.ID]
	require.NotNil(t, updatedTask)
	assert.NotSame(t, existingTask, updatedTask)
	assert.Equal(t, updatedCfg, updatedTask.config)

	updatedTask.history.mu.Lock()
	defer updatedTask.history.mu.Unlock()
	require.Len(t, updatedTask.history.samples, 1)
	assert.Equal(t, int64(24), updatedTask.history.samples[0].responseUs)

	agg := updatedTask.history.aggregateLocked(time.Hour, now)
	require.True(t, agg.hasData())
	assert.Equal(t, int64(2), agg.totalCount)
	assert.Equal(t, int64(2), agg.successCount)
	assert.Equal(t, int64(18), agg.avgResponse())

	select {
	case <-existingTask.ctx.Done():
	default:
		t.Fatal("expected original monitor task to be cancelled")
	}
}

func TestMonitorManagerApplySyncDeleteRemovesTask(t *testing.T) {
	config := monitor.Config{ID: "monitor-1", Target: "1.1.1.1", Protocol: "icmp", Interval: 10}
	task := newMonitorTask(config)
	pm := newMonitorManager()
	pm.monitors = map[string]*monitorTask{config.ID: task}

	_, err := pm.HandleSyncRequest(monitor.SyncRequest{
		Action: monitor.SyncActionDelete,
		Config: monitor.Config{ID: config.ID},
	})

	require.NoError(t, err)
	_, exists := pm.monitors[config.ID]
	assert.False(t, exists)

	select {
	case <-task.ctx.Done():
	default:
		t.Fatal("expected deleted monitor task to be cancelled")
	}
}

func TestMonitorManagerGetRandomDelay(t *testing.T) {
	for i := 1000; i < 360_000; i += 1000 {
		delay := getStagger(int64(i))
		assert.GreaterOrEqual(t, delay, time.Duration(i/2)*time.Millisecond)
		assert.LessOrEqual(t, delay, time.Duration(i)*time.Millisecond)
	}
}

func TestMonitorHTTP(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Beszel-Agent/"+beszel.Version+" (+https://beszel.dev)", r.Header.Get("User-Agent"))
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()

		responseUs, err := monitorHTTP(context.Background(), server.Client(), server.URL)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, responseUs, int64(0))
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		defer server.Close()

		responseUs, err := monitorHTTP(context.Background(), server.Client(), server.URL)
		assert.Equal(t, int64(-1), responseUs)
		require.Error(t, err)
	})
}

func TestMonitorTCP(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer listener.Close()

		accepted := make(chan struct{})
		go func() {
			defer close(accepted)
			conn, err := listener.Accept()
			if err == nil {
				_ = conn.Close()
			}
		}()

		port := uint16(listener.Addr().(*net.TCPAddr).Port)
		responseUs, err := monitorTCP(context.Background(), "127.0.0.1", port)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, responseUs, int64(0))
		<-accepted
	})

	t.Run("connection failure", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)

		port := uint16(listener.Addr().(*net.TCPAddr).Port)
		require.NoError(t, listener.Close())

		responseUs, err := monitorTCP(context.Background(), "127.0.0.1", port)
		assert.Equal(t, int64(-1), responseUs)
		require.Error(t, err)
	})
}

func TestMonitorTCPAddressFallback(t *testing.T) {
	for _, tc := range []struct {
		name string
		ips  []string
		loss bool
	}{
		{"first address fails", []string{"127.0.0.2", "127.0.0.1"}, false},
		{"first address succeeds", []string{"127.0.0.1", "127.0.0.2"}, false},
		{"all addresses fail", []string{"127.0.0.2", "127.0.0.3"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			require.NoError(t, err)
			defer listener.Close()
			original := net.DefaultResolver
			net.DefaultResolver = tcpMonitorTestResolver(tc.ips)
			defer func() { net.DefaultResolver = original }()

			// Verify the resolver preserves the intended order, so success cannot
			// accidentally bypass the failed first address in the regression case.
			ips, err := net.DefaultResolver.LookupHost(t.Context(), "tcp-monitor.invalid.")
			require.NoError(t, err)
			require.Equal(t, tc.ips, ips)
			responseUs, err := monitorTCP(t.Context(), "tcp-monitor.invalid.", uint16(listener.Addr().(*net.TCPAddr).Port))
			if tc.loss {
				require.Error(t, err)
				assert.Equal(t, int64(-1), responseUs)
			} else {
				require.NoError(t, err)
				assert.GreaterOrEqual(t, responseUs, int64(0))
			}
		})
	}
}

// tcpMonitorTestResolver supplies multiple A records without external DNS.
func tcpMonitorTestResolver(ips []string) *net.Resolver {
	return &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			// net.Resolver uses TCP framing when its connection is not a PacketConn.
			var size uint16
			if err := binary.Read(server, binary.BigEndian, &size); err != nil {
				return
			}
			packet := make([]byte, size)
			if _, err := io.ReadFull(server, packet); err != nil {
				return
			}
			var msg dnsmessage.Message
			if err := msg.Unpack(packet); err != nil {
				return
			}
			msg.Header.Response = true
			msg.Header.RecursionAvailable = true
			for _, question := range msg.Questions {
				if question.Type != dnsmessage.TypeA {
					continue
				}
				for _, ip := range ips {
					msg.Answers = append(msg.Answers, dnsmessage.Resource{
						Header: dnsmessage.ResourceHeader{Name: question.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET},
						Body:   &dnsmessage.AResource{A: [4]byte(net.ParseIP(ip).To4())},
					})
				}
			}
			packet, err := msg.Pack()
			if err != nil {
				return
			}
			response := binary.BigEndian.AppendUint16(nil, uint16(len(packet)))
			_, _ = server.Write(append(response, packet...))
		}()
		return client, nil
	}}
}

// udpDNSTestServer starts a UDP server on loopback that answers A queries with the
// given IPs, and returns its listen address (host:port).
func udpDNSTestServer(t *testing.T, ips []string) string {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	go func() {
		buf := make([]byte, 512)
		for {
			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			var msg dnsmessage.Message
			if err := msg.Unpack(buf[:n]); err != nil {
				continue
			}
			msg.Header.Response = true
			msg.Header.RecursionAvailable = true
			for _, question := range msg.Questions {
				if question.Type != dnsmessage.TypeA {
					continue
				}
				for _, ip := range ips {
					msg.Answers = append(msg.Answers, dnsmessage.Resource{
						Header: dnsmessage.ResourceHeader{Name: question.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET},
						Body:   &dnsmessage.AResource{A: [4]byte(net.ParseIP(ip).To4())},
					})
				}
			}
			packet, err := msg.Pack()
			if err != nil {
				continue
			}
			_, _ = conn.WriteToUDP(packet, addr)
		}
	}()

	return conn.LocalAddr().String()
}

func TestMonitorDNS(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		responseUs, err := monitorDNS(context.Background(), "localhost", "")
		require.NoError(t, err)
		assert.GreaterOrEqual(t, responseUs, int64(0))
	})

	t.Run("lookup failure", func(t *testing.T) {
		responseUs, err := monitorDNS(context.Background(), "", "")
		assert.Equal(t, int64(-1), responseUs)
		require.Error(t, err)
	})

	t.Run("custom server", func(t *testing.T) {
		serverAddr := udpDNSTestServer(t, []string{"192.0.2.10"})
		responseUs, err := monitorDNS(context.Background(), "example.test.", serverAddr)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, responseUs, int64(0))
	})

	t.Run("custom server without port defaults to 53", func(t *testing.T) {
		resolver := dnsResolverForServer("127.0.0.1")
		conn, err := resolver.Dial(context.Background(), "udp", "")
		require.NoError(t, err)
		defer conn.Close()
		assert.Equal(t, "127.0.0.1:53", conn.RemoteAddr().String())
	})

	t.Run("custom server unreachable", func(t *testing.T) {
		responseUs, err := monitorDNS(context.Background(), "example.test.", "127.0.0.1:1")
		assert.Equal(t, int64(-1), responseUs)
		require.Error(t, err)
	})
}

func TestMonitorManagerCancelsActiveProbe(t *testing.T) {
	for _, action := range []string{"stop", "delete", "upsert", "sync replace", "sync remove"} {
		t.Run(action, func(t *testing.T) {
			started := make(chan struct{})
			canceled := make(chan struct{})
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				select {
				case <-r.Context().Done():
					close(canceled)
				case <-release:
				}
			}))
			defer server.Close()
			defer close(release)
			pm := newMonitorManager()
			defer pm.Stop()
			cfg := monitor.Config{ID: "test", Protocol: "http", Target: server.URL, Interval: 3600}
			task := newMonitorTask(cfg)
			// Seed history to ensure a canceled RunNow does not return an old result.
			task.history.addSampleLocked(monitorSample{responseUs: 123, timestamp: time.Now()})
			pm.monitors[cfg.ID] = task
			done := make(chan *monitor.Result, 1)
			go func() {
				result, _ := pm.UpsertMonitor(cfg, true)
				done <- result
			}()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("probe did not start")
			}
			updated := cfg
			updated.Interval--
			switch action {
			case "stop":
				pm.Stop()
			case "delete":
				pm.DeleteMonitor(cfg.ID)
			case "upsert":
				_, err := pm.UpsertMonitor(updated, false)
				require.NoError(t, err)
			case "sync replace":
				pm.SyncMonitors([]monitor.Config{updated})
			case "sync remove":
				pm.SyncMonitors(nil)
			}
			select {
			case <-canceled:
			case <-time.After(time.Second):
				t.Fatal("active HTTP request was not canceled")
			}
			select {
			case result := <-done:
				assert.Nil(t, result)
			case <-time.After(time.Second):
				t.Fatal("RunNow did not return after cancellation")
			}
			task.history.mu.Lock()
			assert.Len(t, task.history.samples, 1, "cancellation must not record packet loss")
			task.history.mu.Unlock()
		})
	}
}

func TestMonitorResolutionCancellation(t *testing.T) {
	for _, protocol := range []string{"tcp", "dns", "icmp"} {
		t.Run(protocol, func(t *testing.T) {
			started := make(chan struct{}, 1)
			original := net.DefaultResolver
			net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				select {
				case started <- struct{}{}:
				default:
				}
				<-ctx.Done()
				return nil, ctx.Err()
			}}
			defer func() { net.DefaultResolver = original }()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				var err error
				switch protocol {
				case "tcp":
					_, err = monitorTCP(ctx, "monitor-cancellation.invalid.", 80)
				case "dns":
					_, err = monitorDNS(ctx, "monitor-cancellation.invalid.", "")
				case "icmp":
					_, err = monitorICMP(ctx, "monitor-cancellation.invalid.")
				}
				done <- err
			}()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("lookup did not start")
			}
			cancel()
			select {
			case err := <-done:
				require.Error(t, err)
			case <-time.After(time.Second):
				t.Fatal("lookup did not cancel")
			}
		})
	}
}

func TestMonitorProbeTimeoutRecordsLoss(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	pm := newMonitorManager()
	pm.probe = networkMonitorProbe(&http.Client{Timeout: 20 * time.Millisecond})
	task := newMonitorTask(monitor.Config{ID: "timeout", Protocol: "http", Target: server.URL})
	defer task.cancel()

	result := task.runProbe(pm.probe)
	require.NotNil(t, result)
	assert.Equal(t, 100.0, result.PacketLoss)
	assert.Equal(t, 100.0, result.PacketLoss1h)
	require.Len(t, task.history.samples, 1)
	assert.Equal(t, int64(-1), task.history.samples[0].responseUs)
	assert.NoError(t, task.ctx.Err(), "a probe timeout must not cancel the task")
}
