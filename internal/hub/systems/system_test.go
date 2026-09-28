//go:build testing

package systems

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestCombinedData_MigrateDeprecatedFields(t *testing.T) {
	t.Run("Migrate NetworkSent and NetworkRecv to Bandwidth", func(t *testing.T) {
		cd := &system.CombinedData{
			Stats: system.Stats{
				NetworkSent: 1.5, // 1.5 MB
				NetworkRecv: 2.5, // 2.5 MB
			},
		}
		migrateDeprecatedFields(cd, true)

		expectedSent := uint64(1.5 * 1024 * 1024)
		expectedRecv := uint64(2.5 * 1024 * 1024)

		if cd.Stats.Bandwidth[0] != expectedSent {
			t.Errorf("expected Bandwidth[0] %d, got %d", expectedSent, cd.Stats.Bandwidth[0])
		}
		if cd.Stats.Bandwidth[1] != expectedRecv {
			t.Errorf("expected Bandwidth[1] %d, got %d", expectedRecv, cd.Stats.Bandwidth[1])
		}
		if cd.Stats.NetworkSent != 0 || cd.Stats.NetworkRecv != 0 {
			t.Errorf("expected NetworkSent and NetworkRecv to be reset, got %f, %f", cd.Stats.NetworkSent, cd.Stats.NetworkRecv)
		}
	})

	t.Run("Migrate Info.Bandwidth to Info.BandwidthBytes", func(t *testing.T) {
		cd := &system.CombinedData{
			Info: system.Info{
				Bandwidth: 10.0, // 10 MB
			},
		}
		migrateDeprecatedFields(cd, true)

		expected := uint64(10 * 1024 * 1024)
		if cd.Info.BandwidthBytes != expected {
			t.Errorf("expected BandwidthBytes %d, got %d", expected, cd.Info.BandwidthBytes)
		}
		if cd.Info.Bandwidth != 0 {
			t.Errorf("expected Info.Bandwidth to be reset, got %f", cd.Info.Bandwidth)
		}
	})

	t.Run("Migrate DiskReadPs and DiskWritePs to DiskIO", func(t *testing.T) {
		cd := &system.CombinedData{
			Stats: system.Stats{
				DiskReadPs:  3.0, // 3 MB
				DiskWritePs: 4.0, // 4 MB
			},
		}
		migrateDeprecatedFields(cd, true)

		expectedRead := uint64(3 * 1024 * 1024)
		expectedWrite := uint64(4 * 1024 * 1024)

		if cd.Stats.DiskIO[0] != expectedRead {
			t.Errorf("expected DiskIO[0] %d, got %d", expectedRead, cd.Stats.DiskIO[0])
		}
		if cd.Stats.DiskIO[1] != expectedWrite {
			t.Errorf("expected DiskIO[1] %d, got %d", expectedWrite, cd.Stats.DiskIO[1])
		}
		if cd.Stats.DiskReadPs != 0 || cd.Stats.DiskWritePs != 0 {
			t.Errorf("expected DiskReadPs and DiskWritePs to be reset, got %f, %f", cd.Stats.DiskReadPs, cd.Stats.DiskWritePs)
		}
	})

	t.Run("Migrate Info fields to Details struct", func(t *testing.T) {
		cd := &system.CombinedData{
			Stats: system.Stats{
				Mem: 16.0, // 16 GB
			},
			Info: system.Info{
				Hostname:      "test-host",
				KernelVersion: "6.8.0",
				Cores:         8,
				Threads:       16,
				CpuModel:      "Intel i7",
				Podman:        true,
				Os:            system.Linux,
			},
		}
		migrateDeprecatedFields(cd, true)

		if cd.Details == nil {
			t.Fatal("expected Details struct to be created")
		}
		if cd.Details.Hostname != "test-host" {
			t.Errorf("expected Hostname 'test-host', got '%s'", cd.Details.Hostname)
		}
		if cd.Details.Kernel != "6.8.0" {
			t.Errorf("expected Kernel '6.8.0', got '%s'", cd.Details.Kernel)
		}
		if cd.Details.Cores != 8 {
			t.Errorf("expected Cores 8, got %d", cd.Details.Cores)
		}
		if cd.Details.Threads != 16 {
			t.Errorf("expected Threads 16, got %d", cd.Details.Threads)
		}
		if cd.Details.CpuModel != "Intel i7" {
			t.Errorf("expected CpuModel 'Intel i7', got '%s'", cd.Details.CpuModel)
		}
		if cd.Details.Podman != true {
			t.Errorf("expected Podman true, got %v", cd.Details.Podman)
		}
		if cd.Details.Os != system.Linux {
			t.Errorf("expected Os Linux, got %d", cd.Details.Os)
		}
		expectedMem := uint64(16 * 1024 * 1024 * 1024)
		if cd.Details.MemoryTotal != expectedMem {
			t.Errorf("expected MemoryTotal %d, got %d", expectedMem, cd.Details.MemoryTotal)
		}

		if cd.Info.Hostname != "" || cd.Info.KernelVersion != "" || cd.Info.Cores != 0 || cd.Info.CpuModel != "" || cd.Info.Podman != false || cd.Info.Os != 0 {
			t.Errorf("expected Info fields to be reset, got %+v", cd.Info)
		}
	})

	t.Run("Do not migrate if Details already exists", func(t *testing.T) {
		cd := &system.CombinedData{
			Details: &system.Details{Hostname: "existing-host"},
			Info: system.Info{
				Hostname: "deprecated-host",
			},
		}
		migrateDeprecatedFields(cd, true)

		if cd.Details.Hostname != "existing-host" {
			t.Errorf("expected Hostname 'existing-host', got '%s'", cd.Details.Hostname)
		}
		if cd.Info.Hostname != "deprecated-host" {
			t.Errorf("expected Info.Hostname to remain 'deprecated-host', got '%s'", cd.Info.Hostname)
		}
	})

	t.Run("Do not create details if migrateDetails is false", func(t *testing.T) {
		cd := &system.CombinedData{
			Info: system.Info{
				Hostname: "deprecated-host",
			},
		}
		migrateDeprecatedFields(cd, false)

		if cd.Details != nil {
			t.Fatal("expected Details struct to not be created")
		}

		if cd.Info.Hostname != "" {
			t.Errorf("expected Info.Hostname to be reset, got '%s'", cd.Info.Hostname)
		}
	})
}

func TestSetDownAfterContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// manager is nil on purpose: setDown must bail out before touching the app
	sys := &System{Status: up, ctx: ctx}

	if err := sys.setDown(nil); err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if sys.Status != up {
		t.Fatalf("status should be untouched, got %q", sys.Status)
	}
}

func TestSSHDisabledSkipsSSHFallback(t *testing.T) {
	// manager is nil on purpose: any SSH attempt would panic
	sys := &System{}
	sys.sshDisabled.Store(true)

	var result string
	err := sys.request(context.Background(), common.GetContainerInfo, nil, &result)
	require.ErrorIs(t, err, errSSHDisabled)

	_, err = sys.fetchDataFromAgent(common.DataRequestOptions{})
	require.ErrorIs(t, err, errSSHDisabled)
}

func TestCreateRecordsSavesSSHDisabledColumn(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	for _, disabled := range []bool{true, false} {
		_, err := sys.createRecords(&system.CombinedData{Info: system.Info{SSHDisabled: disabled}})
		require.NoError(t, err)
		record, err := app.FindRecordById("systems", sys.Id)
		require.NoError(t, err)
		require.Equal(t, disabled, record.GetBool("ssh_disabled"))
		require.NotContains(t, record.GetString("info"), `"sd"`, "flag belongs in its own column, not info")
	}
}

func TestSSHFallbackDialsAgentUnlessDisabled(t *testing.T) {
	for _, tc := range []struct {
		name          string
		agentDisabled bool
		hubEnv        string
		wantDial      bool
	}{
		{name: "enabled", wantDial: true},
		{name: "disabled on agent", agentDisabled: true},
		{name: "disabled on hub", hubEnv: "DISABLE_SSH"},
		{name: "disabled on hub with prefix", hubEnv: "BESZEL_HUB_DISABLE_SSH"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.hubEnv != "" {
				t.Setenv(tc.hubEnv, "true")
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer listener.Close()
			accepted := make(chan struct{}, 1)
			go func() {
				if conn, err := listener.Accept(); err == nil {
					accepted <- struct{}{}
					conn.Close()
				}
			}()

			host, port, _ := net.SplitHostPort(listener.Addr().String())
			sm := &SystemManager{sshConfig: &ssh.ClientConfig{HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: time.Second}}
			sys := &System{Host: host, Port: port, Status: down, manager: sm, ctx: context.Background()}
			sys.sshDisabled.Store(tc.agentDisabled)

			_, err = sys.fetchDataFromAgent(common.DataRequestOptions{})
			require.Error(t, err) // listener isn't a real agent
			if !tc.wantDial {
				require.ErrorIs(t, err, errSSHDisabled)
			}

			select {
			case <-accepted:
				require.True(t, tc.wantDial, "hub must not dial SSH when it is disabled")
			case <-time.After(200 * time.Millisecond):
				require.False(t, tc.wantDial, "hub must dial SSH when it is enabled")
			}
		})
	}
}
