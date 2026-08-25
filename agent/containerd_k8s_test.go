package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	cntr "github.com/henrygd/beszel/internal/entities/container"
)

// mockContainer embeds containerd.Container to avoid implementing every method.
type mockContainer struct {
	containerd.Container
	image containerd.Image
	info  containers.Container
}

func (m *mockContainer) Image(ctx context.Context) (containerd.Image, error) {
	if m.image != nil {
		return m.image, nil
	}
	return nil, fmt.Errorf("no image")
}

func (m *mockContainer) Info(ctx context.Context, opts ...containerd.InfoOpts) (containers.Container, error) {
	return m.info, nil
}

// mockImage embeds containerd.Image.
type mockImage struct {
	containerd.Image
	name string
}

func (m *mockImage) Name() string {
	return m.name
}

// mockTask embeds containerd.Task.
type mockTask struct {
	containerd.Task
	pid uint32
}

func (m *mockTask) Pid() uint32 {
	return m.pid
}

func TestGetEnv(t *testing.T) {
	// Set an environment variable
	os.Setenv("TEST_ENV_VAR", "my_value")
	defer os.Unsetenv("TEST_ENV_VAR")

	if val := getEnv("TEST_ENV_VAR", "fallback"); val != "my_value" {
		t.Errorf("Expected 'my_value', got %v", val)
	}

	// Test fallback for unset variable
	if val := getEnv("UNSET_ENV_VAR", "fallback"); val != "fallback" {
		t.Errorf("Expected 'fallback', got %v", val)
	}

	// Test fallback for empty variable
	os.Setenv("EMPTY_ENV_VAR", "")
	defer os.Unsetenv("EMPTY_ENV_VAR")
	if val := getEnv("EMPTY_ENV_VAR", "fallback"); val != "fallback" {
		t.Errorf("Expected 'fallback', got %v", val)
	}
}

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{0, "1 second"},
		{1 * time.Second, "1 second"},
		{45 * time.Second, "45 seconds"},
		{1 * time.Minute, "1 minute"},
		{5 * time.Minute, "5 minutes"},
		{1 * time.Hour, "1 hour"},
		{23 * time.Hour, "23 hours"},
		{24 * time.Hour, "1 day"},
		{48 * time.Hour, "2 days"},
		{365 * 24 * time.Hour, "1 year"},
		{731 * 24 * time.Hour, "2 years"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			past := time.Now().Add(-tt.duration)
			result := formatUptime(past)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestGetContainerHealth(t *testing.T) {
	manager := &ContainerdK8SManager{}

	tests := []struct {
		name       string
		status     containerd.Status
		expected   cntr.DockerHealth
	}{
		{"Running", containerd.Status{Status: containerd.Running}, cntr.DockerHealthHealthy},
		{"Paused", containerd.Status{Status: containerd.Paused}, cntr.DockerHealthStarting},
		{"Stopped Cleanly", containerd.Status{Status: containerd.Stopped, ExitStatus: 0}, cntr.DockerHealthNone},
		{"Stopped with Error", containerd.Status{Status: containerd.Stopped, ExitStatus: 1}, cntr.DockerHealthUnhealthy},
		{"Created", containerd.Status{Status: containerd.Created, ExitStatus: 0}, cntr.DockerHealthNone},
		{"Unknown", containerd.Status{Status: containerd.Unknown}, cntr.DockerHealthNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			health := manager.getContainerHealth(tt.status)
			if health != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, health)
			}
		})
	}
}

func TestTailFile(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	file, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("Failed to create temp log file: %v", err)
	}

	// Write 10 lines
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(file, "Line %d\n", i)
	}
	file.Close()

	// Tail last 3 lines
	logs, err := tailFile(logPath, 3)
	if err != nil {
		t.Fatalf("tailFile returned error: %v", err)
	}

	expected := "Line 8\nLine 9\nLine 10"
	if logs != expected {
		t.Errorf("Expected logs:\n%q\nGot:\n%q", expected, logs)
	}

	// Tail more lines than exist
	logs, err = tailFile(logPath, 20)
	if err != nil {
		t.Fatalf("tailFile returned error: %v", err)
	}
	if len(logs) == 0 {
		t.Errorf("Expected full file content, got empty string")
	}
}

func TestReadNetworkStats_InvalidPID(t *testing.T) {
	// 9999999 is highly unlikely to be a valid PID, triggering the open error
	tx, rx := readNetworkStats(9999999)
	if tx != 0 || rx != 0 {
		t.Errorf("Expected 0,0 for invalid PID, got tx:%d rx:%d", tx, rx)
	}
}

func TestShouldIgnoreImage(t *testing.T) {
	manager := &ContainerdK8SManager{}
	ctx := context.Background()

	tests := []struct {
		name     string
		image    string
		expected bool
	}{
		{"Ignored Mirror Pause", "docker.io/rancher/mirrored-pause:3.6", true},
		{"Normal Image", "docker.io/library/nginx:latest", false},
		{"No Image", "", false}, // Triggers the missing image path
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var container *mockContainer
			if tt.image == "" {
				container = &mockContainer{} // returns error on Image()
			} else {
				container = &mockContainer{
					image: &mockImage{name: tt.image},
				}
			}

			result := manager.shouldIgnoreImage(ctx, container)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetContainerUptimeStatus(t *testing.T) {
	manager := &ContainerdK8SManager{}
	ctx := context.Background()

	t.Run("Valid CreatedAt", func(t *testing.T) {
		c := &mockContainer{
			info: containers.Container{
				CreatedAt: time.Now().Add(-2 * time.Hour),
			},
		}
		status := manager.getContainerUptimeStatus(ctx, c)
		if status != "Up 2 hours" {
			t.Errorf("Expected 'Up 2 hours', got '%s'", status)
		}
	})

	t.Run("Valid UpdatedAt Fallback", func(t *testing.T) {
		c := &mockContainer{
			info: containers.Container{
				UpdatedAt: time.Now().Add(-5 * time.Minute),
			},
		}
		status := manager.getContainerUptimeStatus(ctx, c)
		if status != "Up 5 minutes" {
			t.Errorf("Expected 'Up 5 minutes', got '%s'", status)
		}
	})

	t.Run("Zero Times", func(t *testing.T) {
		c := &mockContainer{
			info: containers.Container{},
		}
		status := manager.getContainerUptimeStatus(ctx, c)
		if status != "Up unknown" {
			t.Errorf("Expected 'Up unknown', got '%s'", status)
		}
	})
}

func TestGetNetworkBandwidthDelta(t *testing.T) {
	manager := &ContainerdK8SManager{
		prevNet: make(map[string]NetReading),
	}
	now := time.Now()

	t.Run("PID 0 returns early", func(t *testing.T) {
		task := &mockTask{pid: 0}
		tx, rx := manager.getNetworkBandwidthDelta("cid-1", task, now)
		if tx != 0 || rx != 0 {
			t.Errorf("Expected 0,0, got %d,%d", tx, rx)
		}
	})

	t.Run("Counter reset logic", func(t *testing.T) {
		task := &mockTask{pid: 999999} // triggers readNetworkStats to return 0,0
		manager.prevNet["cid-2"] = NetReading{
			Sent:      1000,
			Recv:      1000,
			Timestamp: now.Add(-time.Minute),
		}

		// Since readNetworkStats returns 0,0 (simulating counter reset via restart)
		// delta should just be the current reading (0, 0)
		tx, rx := manager.getNetworkBandwidthDelta("cid-2", task, now)
		if tx != 0 || rx != 0 {
			t.Errorf("Expected delta 0,0 on counter reset, got %d,%d", tx, rx)
		}

		// Verify the prevNet state updated to 0,0
		state := manager.prevNet["cid-2"]
		if state.Sent != 0 || state.Recv != 0 {
			t.Errorf("Expected state to be updated to 0,0, got %d,%d", state.Sent, state.Recv)
		}
	})
}

func TestNewContainerdCollector(t *testing.T) {
	// Test blank addr early exit
	os.Setenv("CONTAINERD_ADDR", "")
	collector, err := NewContainerdCollector()
	if collector != nil || err != nil {
		t.Errorf("Expected nil, nil for empty address, got %v, %v", collector, err)
	}

	// Test invalid socket connection (should error)
	os.Setenv("CONTAINERD_ADDR", "/tmp/non-existent-beszel-test.sock")
	defer os.Unsetenv("CONTAINERD_ADDR")
	collector, err = NewContainerdCollector()
	if err == nil {
		t.Errorf("Expected error for non-existent socket")
	}
	if collector != nil {
		t.Errorf("Expected collector to be nil on error")
	}
}

func TestEarlyExitsOnUntrackedContainers(t *testing.T) {
	// To prevent interacting with an uninitialized pointer (c.client) when a 
	// genuine socket isn't present, test the fast-paths that bypass the API.
	manager := &ContainerdK8SManager{
		prevCpu: make(map[string]CpuReading),
	}
	ctx := context.Background()

	t.Run("getLogs missing ID", func(t *testing.T) {
		logs, ok, err := manager.getLogs(ctx, "untracked-id")
		if logs != "" || ok != false || err != nil {
			t.Errorf("Expected empty early exit for untracked logs")
		}
	})

	t.Run("getContainerInfo missing ID", func(t *testing.T) {
		info, ok, err := manager.getContainerInfo(ctx, "untracked-id")
		if info != nil || ok != false || err != nil {
			t.Errorf("Expected nil early exit for untracked container info")
		}
	})
}