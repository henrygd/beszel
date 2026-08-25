package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
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
	prevNet   map[string]NetReading
}

type CpuReading struct {
	UsageUsec uint64
	Timestamp time.Time
}

type NetReading struct {
	Sent      uint64
	Recv      uint64
	Timestamp time.Time
}

// ContainerInfo represents detailed inspect metadata for a containerd/Kubernetes container.
type ContainerInfo struct {
	ID            string            `json:"id"`
	Namespace     string            `json:"namespace"`
	PodName       string            `json:"pod_name,omitempty"`
	ContainerName string            `json:"container_name,omitempty"`
	Image         string            `json:"image"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	Labels        map[string]string `json:"labels,omitempty"`
	Status        string            `json:"status"`
	Pid           uint32            `json:"pid,omitempty"`
	ExitStatus    uint32            `json:"exit_status,omitempty"`
	Snapshotter   string            `json:"snapshotter,omitempty"`
	SnapshotKey   string            `json:"snapshot_key,omitempty"`
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
		prevNet:   make(map[string]NetReading),
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
			continue
		}

		statsList = append(statsList, stat)
	}

	log.Printf("[Collector] Polled %d total containers: %d matched k8s pods, %d skipped non-workload/PODs",
		len(containers), len(statsList), skippedNonK8s)

	return statsList
}

// collectContainerStats handles metrics extraction, CPU calculation, network bandwidth delta, and metadata gathering for a single container.
func (c *ContainerdK8SManager) collectContainerStats(ctx context.Context, container containerd.Container, podName, containerName string, cLabels map[string]string, now time.Time) (*cntr.Stats, error) {
	task, err := container.Task(ctx, nil)
	if err != nil {
		// Return nil, nil so we skip stopped/completed containers silently without logging errors
		return nil, fmt.Errorf("container not running: %w", err)
	}

	statusStr := c.getContainerUptimeStatus(ctx, container)

	// Fetch task status (running, paused, etc.)
	var taskStatus containerd.Status
	if ts, err := task.Status(ctx); err == nil {
		taskStatus = ts
	}

	// Determine container health
	health := c.getContainerHealth(taskStatus)

	// Calculate network bandwidth delta
	sentDelta, recvDelta := c.getNetworkBandwidthDelta(container.ID(), task, now)

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
		Bandwidth:    [2]uint64{sentDelta, recvDelta},          // [sent bytes, recv bytes] delta
		Status:       statusStr,
		Health:       health,
		Image:        imageName,
		CpuSystem:    stats.CPU.SystemUsec,
		CpuContainer: currentCpuUsec,
		PrevReadTime: now,
	}, nil
}

// getContainerInfo retrieves meaningful containerd and Kubernetes container metadata, returning a JSON byte array.
func (c *ContainerdK8SManager) getContainerInfo(ctx context.Context, containerID string) ([]byte, error) {
	log.Printf("[Collector Debug] Fetching container info for ID: %s", containerID[:12])
	k8sCtx := namespaces.WithNamespace(ctx, c.namespace)
	container, err := c.client.LoadContainer(k8sCtx, containerID)
	if err != nil {
		log.Printf("[Collector Debug] Failed to load container %s: %v", containerID[:12], err)
		return nil, fmt.Errorf("failed to load container %s: %w", containerID[:12], err)
	}

	info, err := container.Info(k8sCtx)
	if err != nil {
		log.Printf("[Collector Debug] Failed to get container info for %s: %v", containerID[:12], err)
		return nil, fmt.Errorf("failed to get container info for %s: %w", containerID[:12], err)
	}

	labels, _ := container.Labels(k8sCtx)
	if labels == nil {
		labels = info.Labels
	}

	imageName := info.Image
	if img, err := container.Image(k8sCtx); err == nil {
		imageName = img.Name()
	}

	statusStr := "Stopped"
	var pid uint32
	var exitStatus uint32
	if task, err := container.Task(k8sCtx, nil); err == nil {
		pid = task.Pid()
		if ts, err := task.Status(k8sCtx); err == nil {
			statusStr = string(ts.Status)
			exitStatus = ts.ExitStatus
		} else {
			statusStr = "Running"
		}
	}

	podName := labels["io.kubernetes.pod.name"]
	containerName := labels["io.kubernetes.container.name"]
	namespace := labels["io.kubernetes.pod.namespace"]
	if namespace == "" {
		namespace = c.namespace
	}

	containerInfo := ContainerInfo{
		ID:            container.ID(),
		Namespace:     namespace,
		PodName:       podName,
		ContainerName: containerName,
		Image:         imageName,
		CreatedAt:     info.CreatedAt,
		UpdatedAt:     info.UpdatedAt,
		Labels:        labels,
		Status:        statusStr,
		Pid:           pid,
		ExitStatus:    exitStatus,
		Snapshotter:   info.Snapshotter,
		SnapshotKey:   info.SnapshotKey,
	}

	jsonData, err := json.MarshalIndent(containerInfo, "", "  ")
	if err != nil {
		log.Printf("[Collector Debug] Failed to marshal container info for %s: %v", containerID[:12], err)
		return nil, fmt.Errorf("failed to marshal container info: %w", err)
	}

	log.Printf("[Collector Debug] Successfully generated container info JSON for %s", containerID[:12])
	return jsonData, nil
}


// getLogs fetches the logs for a container by reading /var/log/containers/<pod>_<namespace>_<container>-*.log
func (c *ContainerdK8SManager) getLogs(ctx context.Context, containerID string) (string, error) {
	log.Printf("[Collector Debug] Fetching logs for container ID: %s", containerID[:12])
	k8sCtx := namespaces.WithNamespace(ctx, c.namespace)
	container, err := c.client.LoadContainer(k8sCtx, containerID)
	if err != nil {
		log.Printf("[Collector Debug] Failed to load container %s: %v", containerID[:12], err)
		return "", fmt.Errorf("failed to load container %s: %w", containerID[:12], err)
	}

	labels, err := container.Labels(k8sCtx)
	if err != nil {
		log.Printf("[Collector Debug] Failed to get labels for container %s: %v", containerID[:12], err)
		return "", fmt.Errorf("failed to get container labels: %w", err)
	}

	podName := labels["io.kubernetes.pod.name"]
	namespace := labels["io.kubernetes.pod.namespace"]
	containerName := labels["io.kubernetes.container.name"]

	if podName == "" || containerName == "" {
		log.Printf("[Collector Debug] Missing labels for container %s (podName: %s, containerName: %s)", containerID[:12], podName, containerName)
		return "", fmt.Errorf("missing kubernetes pod or container name labels for container %s", containerID[:12])
	}

	// Kubernetes standard log path pattern under /var/log/containers/
	pattern := fmt.Sprintf("/var/log/containers/%s_%s_%s-*.log", podName, namespace, containerName)
	log.Printf("[Collector Debug] Searching for log files matching primary pattern: %s", pattern)
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		log.Printf("[Collector Debug] Primary pattern match failed or empty (err: %v). Trying fallback pattern...", err)
		// Fallback to searching by container name if exact pod match fails
		fallbackPattern := fmt.Sprintf("/var/log/containers/*_%s_*_%s-*.log", namespace, containerName)
		log.Printf("[Collector Debug] Searching for log files matching fallback pattern: %s", fallbackPattern)
		matches, err = filepath.Glob(fallbackPattern)
		if err != nil || len(matches) == 0 {
			log.Printf("[Collector Debug] Fallback pattern also returned no matches for pod %s, container %s", podName, containerName)
			return "", fmt.Errorf("log file not found for pod %s, container %s", podName, containerName)
		}
	}

	log.Printf("[Collector Debug] Found %d matching log file(s): %v", len(matches), matches)

	// Pick the most recently modified log file if multiple exist (e.g. restarts)
	logFile := matches[0]
	var newestTime time.Time
	for _, m := range matches {
		if info, err := os.Stat(m); err == nil {
			if info.ModTime().After(newestTime) {
				newestTime = info.ModTime()
				logFile = m
			}
		} else {
			log.Printf("[Collector Debug] Failed to stat log file candidate %s: %v", m, err)
		}
	}

	log.Printf("[Collector Debug] Selected newest log file: %s (mod time: %v)", logFile, newestTime)

	// Tail the log file efficiently
	logs, err := tailFile(logFile, dockerLogsTail)
	if err != nil {
		log.Printf("[Collector Debug] Failed to tail log file %s: %v", logFile, err)
		return "", fmt.Errorf("failed to read log file %s: %w", logFile, err)
	}

	// Strip ANSI escape sequences from logs for clean display in web UI
	if strings.Contains(logs, "\x1b") {
		logs = ansiEscapePattern.ReplaceAllString(logs, "")
	}

	log.Printf("[Collector Debug] Successfully read %d characters of logs from %s", len(logs), logFile)
	return logs, nil
}

// tailFile reads the last N lines of a file without loading the entire file into memory.
func tailFile(path string, maxLines int) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > maxLines {
			lines = lines[1:] // keep sliding window of last maxLines
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return strings.Join(lines, "\n"), nil
}

// getNetworkBandwidthDelta computes network traffic delta from /proc/<pid>/net/dev. Returns 0 on the first poll.
func (c *ContainerdK8SManager) getNetworkBandwidthDelta(containerID string, task containerd.Task, now time.Time) (uint64, uint64) {
	pid := task.Pid()
	if pid == 0 {
		return 0, 0
	}

	txBytes, rxBytes := readNetworkStats(pid)
	var sentDelta, recvDelta uint64

	if prevN, exists := c.prevNet[containerID]; exists {
		if txBytes >= prevN.Sent {
			sentDelta = txBytes - prevN.Sent
		} else {
			sentDelta = txBytes // Handle counter reset / container restart
		}
		if rxBytes >= prevN.Recv {
			recvDelta = rxBytes - prevN.Recv
		} else {
			recvDelta = rxBytes // Handle counter reset / container restart
		}
	}

	c.prevNet[containerID] = NetReading{
		Sent:      txBytes,
		Recv:      rxBytes,
		Timestamp: now,
	}

	return sentDelta, recvDelta
}

// getContainerHealth maps container status or custom labels to Beszel DockerHealth states.
func (c *ContainerdK8SManager) getContainerHealth(taskStatus containerd.Status) cntr.DockerHealth {
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

// readNetworkStats reads /proc/<pid>/net/dev using strings.Fields to avoid format string incompatibilities
func readNetworkStats(pid uint32) (uint64, uint64) {
	path := fmt.Sprintf("/proc/%d/net/dev", pid)
	file, err := os.Open(path)
	if err != nil {
		log.Printf("[Collector Debug] Failed to open network stats file %s: %v", path, err)
		return 0, 0
	}
	defer file.Close()

	var totalTx, totalRx uint64
	scanner := bufio.NewScanner(file)

	// Skip the first two header lines of /proc/net/dev
	if !scanner.Scan() {
		log.Printf("[Collector Debug] Failed to read header line 1 from %s", path)
		return 0, 0
	}
	if !scanner.Scan() {
		log.Printf("[Collector Debug] Failed to read header line 2 from %s", path)
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

		// Split space-separated values
		fields := strings.Fields(parts[1])
		if len(fields) < 9 {
			continue
		}

		// Index 0 = Receive bytes, Index 8 = Transmit bytes
		rxBytes, errRx := strconv.ParseUint(fields[0], 10, 64)
		txBytes, errTx := strconv.ParseUint(fields[8], 10, 64)

		if errRx == nil && errTx == nil {
			totalRx += rxBytes
			totalTx += txBytes
		} else {
			log.Printf("[Collector Debug] Failed to parse network fields for %s (iface: %s): rxErr=%v, txErr=%v", path, ifName, errRx, errTx)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[Collector Debug] Error scanning %s: %v", path, err)
	}

	return totalTx, totalRx
}