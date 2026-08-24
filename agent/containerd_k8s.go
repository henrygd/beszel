package agent

import (
	"context"
	"fmt"
	"log"
	"os"
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

		stat, err := c.collectContainerStats(k8sCtx, container, podName, containerName, now)
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
func (c *ContainerdK8SManager) collectContainerStats(ctx context.Context, container containerd.Container, podName, containerName string, now time.Time) (*cntr.Stats, error) {
	task, err := container.Task(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get container task: %w", err)
	}

	// Fetch task status (running, paused, etc.)
	statusStr := ""
	if taskStatus, err := task.Status(ctx); err == nil {
		statusStr = string(taskStatus.Status)
	}

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

	// Construct structured display name (Pod / Container format)
	displayName := fmt.Sprintf("%s/%s", podName, containerName)

	return &cntr.Stats{
		Name:         displayName,
		Id:           container.ID(),
		Cpu:          cpuPercent,
		Mem:          float64(stats.Memory.Usage) / 1024 / 1024, // Convert bytes to MB for Beszel
		Status:       statusStr,
		Image:        imageName,
		CpuSystem:    stats.CPU.SystemUsec,
		CpuContainer: currentCpuUsec,
		PrevReadTime: now,
	}, nil
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists && value != "" {
		return value
	}
	return fallback
}
