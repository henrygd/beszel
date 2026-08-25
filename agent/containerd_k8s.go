package agent

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	v2 "github.com/containerd/cgroups/v3/cgroup2/stats"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/typeurl/v2"

	cntr "github.com/henrygd/beszel/internal/entities/container"
)

type ContainerdK8SManager struct {
	client    *containerd.Client
	nodeName  string
	namespace string
	prevCpu   map[string]CpuReading
}

type CpuReading struct {
	UsageUsec uint64
	Timestamp time.Time
}

var ignoredImages = []string{
	"docker.io/rancher/mirrored-pause",
}

func NewContainerdCollector() (*ContainerdK8SManager, error) {
	nodeName := getEnv("NODE_NAME", "unknown-node")
	socketPath := getEnv("CONTAINERD_ADDR", "")
	namespace := getEnv("CONTAINERD_NAMESPACE", "k8s.io")
	if socketPath == "" {
		return nil, nil
	}
	log.Printf("[Collector] Initializing containerd client (socket: %s, namespace: %s, node: %s)...", socketPath, namespace, nodeName)

	client, err := containerd.New(socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to containerd socket at %s: %w", socketPath, err)
	}

	return &ContainerdK8SManager{
		client:    client,
		nodeName:  nodeName,
		namespace: namespace,
		prevCpu:   make(map[string]CpuReading),
	}, nil
}

func (c *ContainerdK8SManager) Close() error {
	log.Println("[Collector] Closing containerd connection...")
	return c.client.Close()
}

// PollContainers loops through containerd containers and returns populated Beszel Stats structs.
func (c *ContainerdK8SManager) PollContainers() []*cntr.Stats {
	ctx := context.Background()
	k8sCtx := namespaces.WithNamespace(ctx, c.namespace)
	containers, err := c.client.Containers(k8sCtx)
	if err != nil {
		log.Printf("[Collector Error] Failed listing containerd containers: %v", err)
		return nil
	}

	var statsList []*cntr.Stats
	now := time.Now()
	skippedNonK8s := 0

	for _, container := range containers {
		if c.shouldIgnoreImage(k8sCtx, container) {
			continue
		}
		cLabels, err := container.Labels(k8sCtx)
		if err != nil {
			continue
		}

		podName := cLabels["io.kubernetes.pod.name"]
		containerName := cLabels["io.kubernetes.container.name"]

		// Ignore non-K8s containers and pause sandbox containers
		if podName == "" || containerName == "POD" {
			skippedNonK8s++
			continue
		}

		stat, err := c.collectContainerStats(k8sCtx, container, podName, containerName, cLabels, now)
		if err != nil {
			log.Printf("[Collector Debug] Skipping stats collection for container %s: %v", container.ID()[:12], err)
			continue
		}

		statsList = append(statsList, stat)
	}

	log.Printf("[Collector] Polled %d total containers: %d matched k8s pods, %d skipped non-workload/PODs",
		len(containers), len(statsList), skippedNonK8s)

	return statsList
}

// collectContainerStats handles metrics extraction, CPU calculation, and metadata gathering for a single container.
func (c *ContainerdK8SManager) collectContainerStats(ctx context.Context, container containerd.Container, podName, containerName string, cLabels map[string]string, now time.Time) (*cntr.Stats, error) {
	task, err := container.Task(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get container task: %w", err)
	}

	statusStr := c.getContainerUptimeStatus(ctx, container)

	// Fetch task status (running, paused, etc.)
	var taskStatus containerd.Status
	if ts, err := task.Status(ctx); err == nil {
		taskStatus = ts
	}

	// Determine container health
	health := c.getContainerHealth(container, cLabels, taskStatus)

	// Fetch cgroup metrics
	taskMetrics, err := task.Metrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to read task metrics: %w", err)
	}

	data, err := typeurl.UnmarshalAny(taskMetrics.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal task metrics data: %w", err)
	}

	stats, ok := data.(*v2.Metrics)
	if !ok || stats.CPU == nil || stats.Memory == nil {
		return nil, fmt.Errorf("invalid cgroup v2 metrics structure")
	}

	// Compute CPU Usage percentage
	currentCpuUsec := stats.CPU.UsageUsec
	var cpuPercent float64

	if prev, exists := c.prevCpu[container.ID()]; exists {
		timeDeltaSec := now.Sub(prev.Timestamp).Seconds()
		cpuDeltaUsec := float64(currentCpuUsec - prev.UsageUsec)

		if timeDeltaSec > 0 {
			cpuPercent = ((cpuDeltaUsec / 1000000.0) / timeDeltaSec) * 100.0
		}
	}
	c.prevCpu[container.ID()] = CpuReading{
		UsageUsec: currentCpuUsec,
		Timestamp: now,
	}

	// Fetch Image Name
	imageName := ""
	if img, err := container.Image(ctx); err == nil {
		imageName = img.Name()
	}

	namespace := cLabels["io.kubernetes.pod.namespace"]

	// Construct structured display name (Namespace / Pod / Container format)
	displayName := fmt.Sprintf("%s/%s/%s", namespace, podName, containerName)

	return &cntr.Stats{
		Name:         displayName,
		Id:           container.ID(),
		Cpu:          cpuPercent,
		Mem:          float64(stats.Memory.Usage) / 1024 / 1024, // Convert bytes to MB for Beszel
		Status:       statusStr,
		Health:       health,
		Image:        imageName,
		CpuSystem:    stats.CPU.SystemUsec,
		CpuContainer: currentCpuUsec,
		PrevReadTime: now,
	}, nil
}

// getContainerHealth maps container status or custom labels to Beszel DockerHealth states.
func (c *ContainerdK8SManager) getContainerHealth(container containerd.Container, cLabels map[string]string, taskStatus containerd.Status) cntr.DockerHealth {
	switch taskStatus.Status {
	case containerd.Running:
		return cntr.DockerHealthHealthy
	case containerd.Paused:
		return cntr.DockerHealthStarting
	case containerd.Stopped, containerd.Created:
		if taskStatus.ExitStatus != 0 {
			return cntr.DockerHealthUnhealthy
		}
		return cntr.DockerHealthNone
	default:
		return cntr.DockerHealthNone
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists && value != "" {
		return value
	}
	return fallback
}

// shouldIgnoreImage checks if a container's image matches the filter list.
func (c *ContainerdK8SManager) shouldIgnoreImage(ctx context.Context, container containerd.Container) bool {
	img, err := container.Image(ctx)
	if err != nil {
		return false
	}
	imageName := img.Name()
	for _, ignored := range ignoredImages {
		if strings.HasPrefix(imageName, ignored) {
			return true
		}
	}
	return false
}

// getContainerUptimeStatus extracts CreatedAt or UpdatedAt from container info and formats it into an uptime string.
func (c *ContainerdK8SManager) getContainerUptimeStatus(ctx context.Context, container containerd.Container) string {
	info, err := container.Info(ctx)
	if err != nil {
		return "Up unknown"
	}

	t := info.CreatedAt
	if t.IsZero() {
		t = info.UpdatedAt
	}
	if t.IsZero() {
		return "Up unknown"
	}

	return fmt.Sprintf("Up %s", formatUptime(t))
}

// formatUptime converts a time duration into minutes, hours, days, or years.
func formatUptime(t time.Time) string {
	d := time.Since(t)
	if d < 0 {
		d = 0
	}

	years := int(d.Hours() / 24 / 365)
	if years > 0 {
		if years == 1 {
			return "1 year"
		}
		return fmt.Sprintf("%d years", years)
	}

	days := int(d.Hours() / 24)
	if days > 0 {
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	}

	hours := int(d.Hours())
	if hours > 0 {
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}

	mins := int(d.Minutes())
	if mins > 0 {
		if mins == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", mins)
	}

	secs := int(d.Seconds())
	if secs <= 1 {
		return "1 second"
	}
	return fmt.Sprintf("%d seconds", secs)
}

// readNetworkStats reads /proc/<pid>/net/dev to bypass heavy network metric libraries
func readNetworkStats(pid uint32) (uint64, uint64) {
	path := fmt.Sprintf("/proc/%d/net/dev", pid)
	file, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer file.Close()

	var totalTx, totalRx uint64
	scanner := bufio.NewScanner(file)

	// Skip the first two header lines of /proc/net/dev
	if !scanner.Scan() {
		return 0, 0
	}
	if !scanner.Scan() {
		return 0, 0
	}

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		ifName := strings.TrimSpace(parts[0])
		if ifName == "lo" {
			continue // Skip loopback interface
		}

		var rxBytes, txBytes uint64
		// Format layout: rxBytes, [7 skipped fields], txBytes
		_, err := fmt.Sscanf(parts[1], "%d %*d %*d %*d %*d %*d %*d %*d %d", &rxBytes, &txBytes)
		if err == nil {
			totalRx += rxBytes
			totalTx += txBytes
		}
	}

	return totalTx, totalRx
}
