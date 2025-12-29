package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/henrygd/beszel/agent/deltatracker"
	"github.com/henrygd/beszel/internal/entities/kubernetes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

const (
	k8sAPITimeout = 5 * time.Second
	k8sLongTimeout = 10 * time.Second
)

// kubernetesManager manages Kubernetes metrics collection following the dockerManager pattern.
// It collects pod metrics from the Kubernetes Metrics Server API and cluster health information.
type kubernetesManager struct {
	sync.RWMutex
	clientset     k8sclient.Interface // Kubernetes API client
	metricsClient metricsv.Interface  // Metrics Server API client
	nodeName      string              // Current Kubernetes node name
	namespace     string              // Namespace filter (default: "all")
	isInCluster   bool                // Running in-cluster vs out-of-cluster
	nodeCpuCores  int64               // CPU cores available on the node

	// Cache-time-aware tracking for pod stats (like dockerManager)
	// Maps cache time intervals to pod-specific metrics tracking
	podStatsMap   map[uint16]map[string]*kubernetes.PodStats // cacheTimeMs -> podName -> PodStats
	lastPodCpu    map[uint16]map[string]uint64               // cacheTimeMs -> podName -> last CPU nanoseconds
	lastPodMemory map[uint16]map[string]uint64               // cacheTimeMs -> podName -> last memory bytes

	// Network delta trackers - one per cache time to avoid interference (like dockerManager)
	podNetworkSentTrackers  map[uint16]*deltatracker.DeltaTracker[string, uint64]
	podNetworkRecvTrackers  map[uint16]*deltatracker.DeltaTracker[string, uint64]
	nodeNetworkSentTrackers map[uint16]*deltatracker.DeltaTracker[string, uint64]
	nodeNetworkRecvTrackers map[uint16]*deltatracker.DeltaTracker[string, uint64]

	// Cluster-wide metrics (cached for 60s to avoid API rate limiting)
	clusterHealth    *kubernetes.ClusterHealth
	lastClusterSync  time.Time
	workloadMetrics  *kubernetes.WorkloadMetrics
	lastWorkloadSync time.Time
}

// newKubernetesManager creates a new Kubernetes manager if running in a Kubernetes environment.
// Returns nil if Kubernetes is not detected or explicitly disabled.
func newKubernetesManager() (*kubernetesManager, error) {
	// Skip if explicitly disabled
	if skip, _ := GetEnv("SKIP_K8S"); skip == "true" {
		return nil, nil
	}

	// Detect if we're running in Kubernetes by checking for ServiceAccount token
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); os.IsNotExist(err) {
		slog.Debug("Not running in Kubernetes environment")
		return nil, nil
	}

	km := &kubernetesManager{
		podStatsMap:             make(map[uint16]map[string]*kubernetes.PodStats),
		lastPodCpu:              make(map[uint16]map[string]uint64),
		lastPodMemory:           make(map[uint16]map[string]uint64),
		podNetworkSentTrackers:  make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
		podNetworkRecvTrackers:  make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
		nodeNetworkSentTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
		nodeNetworkRecvTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
	}

	// Create Kubernetes clients
	if err := km.createClients(); err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clients: %w", err)
	}

	// Detect node name
	if err := km.detectNodeName(); err != nil {
		return nil, fmt.Errorf("failed to detect node name: %w", err)
	}

	// Get node CPU capacity for percentage calculations
	if err := km.getNodeCpuCapacity(); err != nil {
		slog.Warn("Failed to get node CPU capacity, using fallback", "err", err)
		km.nodeCpuCores = 1
	}

	// Get namespace filter (empty string means all namespaces)
	km.namespace, _ = GetEnv("K8S_NAMESPACE")

	slog.Info("Kubernetes manager initialized",
		"node", km.nodeName,
		"namespace", km.namespace,
		"cpuCores", km.nodeCpuCores,
		"inCluster", km.isInCluster)

	return km, nil
}

// createClients initializes the Kubernetes and Metrics Server API clients.
func (km *kubernetesManager) createClients() error {
	var config *rest.Config
	var err error

	// Try in-cluster config first
	config, err = rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("not running in Kubernetes cluster: %w", err)
	}
	km.isInCluster = true

	// Create clientset for standard Kubernetes API
	km.clientset, err = k8sclient.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}

	// Create metrics client for Metrics Server API
	km.metricsClient, err = metricsv.NewForConfig(config)
	if err != nil {
		// Non-fatal - Metrics Server might not be installed
		slog.Warn("Metrics Server not available - pod metrics will be limited", "err", err)
	}

	return nil
}

// detectNodeName determines the current Kubernetes node name.
func (km *kubernetesManager) detectNodeName() error {
	// First try NODE_NAME environment variable (set via downward API)
	if nodeName := os.Getenv("NODE_NAME"); nodeName != "" {
		km.nodeName = nodeName
		return nil
	}

	// Fallback to hostname
	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}

	// Verify node exists in cluster
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = km.clientset.CoreV1().Nodes().Get(ctx, hostname, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to find node %s in cluster: %w", hostname, err)
	}

	km.nodeName = hostname
	return nil
}

// getNodeCpuCapacity retrieves the CPU capacity of the current node.
func (km *kubernetesManager) getNodeCpuCapacity() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	node, err := km.clientset.CoreV1().Nodes().Get(ctx, km.nodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get node: %w", err)
	}

	// Get CPU capacity in millicores
	cpuCapacity := node.Status.Capacity.Cpu()
	if cpuCapacity == nil {
		return fmt.Errorf("node CPU capacity not available")
	}

	// Convert to number of cores
	km.nodeCpuCores = cpuCapacity.Value()
	if km.nodeCpuCores == 0 {
		km.nodeCpuCores = 1 // Fallback
	}

	return nil
}

// getNodeMetrics retrieves CPU and memory metrics for the current node from Metrics Server.
// This can be used as a fallback when host access is not available.
func (km *kubernetesManager) getNodeMetrics() (*metricsv1beta1.NodeMetrics, error) {
	if km.metricsClient == nil {
		return nil, fmt.Errorf("metrics server client not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	nodeMetrics, err := km.metricsClient.MetricsV1beta1().NodeMetricses().Get(ctx, km.nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get node metrics: %w", err)
	}

	return nodeMetrics, nil
}

// getNodeStats returns basic CPU and memory stats from Metrics Server.
// Returns CPU percentage and memory in GB.
func (km *kubernetesManager) getNodeStats() (cpuPercent float64, memUsedGB float64, memTotalGB float64, err error) {
	// Get node object for capacity
	ctx, cancel := context.WithTimeout(context.Background(), k8sAPITimeout)
	defer cancel()

	node, err := km.clientset.CoreV1().Nodes().Get(ctx, km.nodeName, metav1.GetOptions{})
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to get node: %w", err)
	}

	// Get node metrics from Metrics Server
	nodeMetrics, err := km.getNodeMetrics()
	if err != nil {
		return 0, 0, 0, err
	}

	// CPU calculation
	cpuCapacity := node.Status.Capacity.Cpu().MilliValue()
	cpuUsage := nodeMetrics.Usage.Cpu().MilliValue()
	if cpuCapacity > 0 {
		cpuPercent = float64(cpuUsage) / float64(cpuCapacity) * 100.0
	}

	// Memory calculation
	memCapacity := node.Status.Capacity.Memory().Value()
	memUsage := nodeMetrics.Usage.Memory().Value()
	memTotalGB = float64(memCapacity) / 1024.0 / 1024.0 / 1024.0
	memUsedGB = float64(memUsage) / 1024.0 / 1024.0 / 1024.0

	return cpuPercent, memUsedGB, memTotalGB, nil
}

// getNodeNetworkStats returns node network RX/TX deltas in megabytes per second
// from kubelet stats endpoint. Uses delta tracking per cache interval.
func (km *kubernetesManager) getNodeNetworkStats(cacheTimeMs uint16) (rxMBps float64, txMBps float64, err error) {
	km.Lock()
	defer km.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), k8sAPITimeout)
	defer cancel()

	// Get stats from kubelet
	result := km.clientset.CoreV1().RESTClient().Get().
		Resource("nodes").
		Name(km.nodeName).
		SubResource("proxy", "stats", "summary").
		Do(ctx)

	if result.Error() != nil {
		return 0, 0, fmt.Errorf("failed to get kubelet stats: %w", result.Error())
	}

	data, err := result.Raw()
	if err != nil {
		return 0, 0, err
	}

	var summary kubeletStatsResponse
	if err := json.Unmarshal(data, &summary); err != nil {
		return 0, 0, err
	}

	if summary.Node.Network == nil {
		return 0, 0, fmt.Errorf("node network stats not available")
	}

	// Validate that RxBytes and TxBytes are set
	if summary.Node.Network.RxBytes == 0 && summary.Node.Network.TxBytes == 0 {
		slog.Debug("Node network stats are zero, metrics may not be available yet")
	}

	// Initialize trackers if needed
	if km.nodeNetworkSentTrackers[cacheTimeMs] == nil {
		km.nodeNetworkSentTrackers[cacheTimeMs] = deltatracker.NewDeltaTracker[string, uint64]()
	}
	if km.nodeNetworkRecvTrackers[cacheTimeMs] == nil {
		km.nodeNetworkRecvTrackers[cacheTimeMs] = deltatracker.NewDeltaTracker[string, uint64]()
	}

	// Set current values
	km.nodeNetworkRecvTrackers[cacheTimeMs].Set(km.nodeName, summary.Node.Network.RxBytes)
	km.nodeNetworkSentTrackers[cacheTimeMs].Set(km.nodeName, summary.Node.Network.TxBytes)

	// Get deltas
	rxDelta := km.nodeNetworkRecvTrackers[cacheTimeMs].Delta(km.nodeName)
	txDelta := km.nodeNetworkSentTrackers[cacheTimeMs].Delta(km.nodeName)

	// Cycle trackers for next interval
	km.nodeNetworkRecvTrackers[cacheTimeMs].Cycle()
	km.nodeNetworkSentTrackers[cacheTimeMs].Cycle()

	// Convert bytes to MB/s
	interval := float64(cacheTimeMs) / 1000.0
	rxMBps = float64(rxDelta) / 1024.0 / 1024.0 / interval
	txMBps = float64(txDelta) / 1024.0 / 1024.0 / interval

	return rxMBps, txMBps, nil
}

// getPodStats collects pod metrics for pods running on the current node.
// Follows the dockerManager pattern with cache-time-aware delta tracking.
func (km *kubernetesManager) getPodStats(cacheTimeMs uint16) ([]*kubernetes.PodStats, error) {
	km.Lock()
	defer km.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), k8sLongTimeout)
	defer cancel()

	// Initialize cache maps for this cache time if needed
	if km.podStatsMap[cacheTimeMs] == nil {
		km.podStatsMap[cacheTimeMs] = make(map[string]*kubernetes.PodStats)
		km.lastPodCpu[cacheTimeMs] = make(map[string]uint64)
		km.lastPodMemory[cacheTimeMs] = make(map[string]uint64)
	}

	// Initialize delta trackers for this cache time if needed
	if km.podNetworkSentTrackers[cacheTimeMs] == nil {
		km.podNetworkSentTrackers[cacheTimeMs] = deltatracker.NewDeltaTracker[string, uint64]()
	}
	if km.podNetworkRecvTrackers[cacheTimeMs] == nil {
		km.podNetworkRecvTrackers[cacheTimeMs] = deltatracker.NewDeltaTracker[string, uint64]()
	}

	// List pods on this node
	fieldSelector := fmt.Sprintf("spec.nodeName=%s", km.nodeName)

	// Apply namespace filter
	listOptions := metav1.ListOptions{
		FieldSelector: fieldSelector,
	}

	var podList *corev1.PodList
	var err error

	if km.namespace != "" {
		// List pods in specific namespace
		podList, err = km.clientset.CoreV1().Pods(km.namespace).List(ctx, listOptions)
	} else {
		// List pods in all namespaces (empty string means all)
		podList, err = km.clientset.CoreV1().Pods("").List(ctx, listOptions)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	// Fetch pod metrics from Metrics Server if available
	var podMetricsMap map[string]*metricsv1beta1.PodMetrics
	if km.metricsClient != nil {
		podMetricsMap = km.fetchPodMetrics(ctx)
	}

	// Fetch network stats from kubelet (if we have access)
	networkStatsMap := make(map[string]podNetworkStats)
	if km.clientset != nil {
		rc := km.clientset.CoreV1().RESTClient()
		if rc != nil {
			if typed, ok := rc.(*rest.RESTClient); ok {
				if typed != nil && typed.Client != nil {
					networkStatsMap = km.fetchNetworkStats(ctx, podList.Items)
				}
			} else {
				networkStatsMap = km.fetchNetworkStats(ctx, podList.Items)
			}
		}
	}

	stats := make([]*kubernetes.PodStats, 0, len(podList.Items))
	for i := range podList.Items {
		pod := &podList.Items[i]
		podKey := pod.Namespace + "/" + pod.Name

		podStat := &kubernetes.PodStats{
			Name:           pod.Name,
			Namespace:      pod.Namespace,
			Node:           pod.Spec.NodeName,
			Status:         string(pod.Status.Phase),
			ContainerCount: uint8(len(pod.Spec.Containers)),
		}

		// Count restarts
		var restartCount int32
		for _, containerStatus := range pod.Status.ContainerStatuses {
			restartCount += containerStatus.RestartCount
		}
		podStat.RestartCount = restartCount

		// Add metrics from Metrics Server if available
		if podMetrics, exists := podMetricsMap[podKey]; exists {
			km.updatePodMetrics(podStat, podMetrics, cacheTimeMs)
		}

		// Add network stats if available
		if netStats, exists := networkStatsMap[podKey]; exists {
			km.updatePodNetworkStats(podStat, netStats, podKey, cacheTimeMs)
		}

		stats = append(stats, podStat)
	}

	// Cycle network delta trackers for next interval
	if km.podNetworkSentTrackers[cacheTimeMs] != nil {
		km.podNetworkSentTrackers[cacheTimeMs].Cycle()
	}
	if km.podNetworkRecvTrackers[cacheTimeMs] != nil {
		km.podNetworkRecvTrackers[cacheTimeMs].Cycle()
	}

	return stats, nil
}

// fetchPodMetrics retrieves pod metrics from the Metrics Server API.
func (km *kubernetesManager) fetchPodMetrics(ctx context.Context) map[string]*metricsv1beta1.PodMetrics {
	metricsMap := make(map[string]*metricsv1beta1.PodMetrics)

	var podMetricsList *metricsv1beta1.PodMetricsList
	var err error

	if km.namespace != "" {
		// Get metrics for specific namespace
		podMetricsList, err = km.metricsClient.MetricsV1beta1().PodMetricses(km.namespace).List(ctx, metav1.ListOptions{})
	} else {
		// Get metrics for all namespaces (empty string means all)
		podMetricsList, err = km.metricsClient.MetricsV1beta1().PodMetricses("").List(ctx, metav1.ListOptions{})
	}

	if err != nil {
		slog.Debug("Failed to fetch pod metrics from Metrics Server", "err", err)
		return metricsMap
	}

	for i := range podMetricsList.Items {
		metric := &podMetricsList.Items[i]
		key := metric.Namespace + "/" + metric.Name
		metricsMap[key] = metric
	}

	return metricsMap
}

// updatePodMetrics updates a PodStats object with metrics from the Metrics Server.
func (km *kubernetesManager) updatePodMetrics(podStat *kubernetes.PodStats, podMetrics *metricsv1beta1.PodMetrics, cacheTimeMs uint16) {
	// Aggregate CPU and memory across all containers in the pod
	var totalCpuMillicores float64
	var totalMemBytes uint64

	for _, container := range podMetrics.Containers {
		// CPU usage from Kubernetes Metrics Server is instantaneous (not cumulative)
		// It's returned in millicores: "100m" = 0.1 core = 10% of one CPU
		cpuMillicores := float64(container.Usage.Cpu().MilliValue())
		totalCpuMillicores += cpuMillicores

		// Memory in bytes
		memBytes := uint64(container.Usage.Memory().Value())
		totalMemBytes += memBytes
	}

	// Store memory in megabytes (to match container format)
	podStat.Mem = float64(totalMemBytes) / 1024.0 / 1024.0

	// CPU: Convert millicores to percentage based on node CPU capacity
	nodeTotalMillicores := float64(km.nodeCpuCores) * 1000.0
	if nodeTotalMillicores > 0 {
		podStat.Cpu = (totalCpuMillicores / nodeTotalMillicores) * 100.0
	} else {
		// Fallback: assume single core (100m = 10%)
		slog.Warn("Node CPU cores not set, using single-core fallback for CPU calculation", "pod", podStat.Name)
		podStat.Cpu = totalCpuMillicores / 10.0
	}
}

// fetchNetworkStats fetches network statistics from kubelet's stats/summary endpoint.
// Makes a single API call to get stats for all pods on the current node.
func (km *kubernetesManager) fetchNetworkStats(ctx context.Context, pods []corev1.Pod) map[string]podNetworkStats {
	statsMap := make(map[string]podNetworkStats)

	if km.clientset == nil || km.clientset.CoreV1().RESTClient() == nil {
		return statsMap
	}

	rc := km.clientset.CoreV1().RESTClient()
	if typed, ok := rc.(*rest.RESTClient); ok && typed.Client == nil {
		return statsMap
	}

	// Get stats from kubelet for this node (single API call)
	// The API path is: /api/v1/nodes/{nodeName}/proxy/stats/summary
	result := rc.Get().
		Resource("nodes").
		Name(km.nodeName).
		SubResource("proxy", "stats", "summary").
		Do(ctx)

	if result.Error() != nil {
		// Permission denied or not available - log for debugging
		// This is expected if RBAC doesn't grant nodes/stats or nodes/proxy access
		slog.Debug("Failed to fetch network stats from kubelet", "err", result.Error())
		return statsMap
	}

	data, err := result.Raw()
	if err != nil {
		return statsMap
	}

	// Parse the stats summary
	var summary kubeletStatsResponse
	if err := json.Unmarshal(data, &summary); err != nil {
		slog.Debug("Failed to parse kubelet stats", "err", err)
		return statsMap
	}

	// Build map of pod network stats
	for _, podStats := range summary.Pods {
		if podStats.Network != nil {
			podKey := podStats.PodRef.Namespace + "/" + podStats.PodRef.Name
			statsMap[podKey] = podNetworkStats{
				RxBytes: podStats.Network.RxBytes,
				TxBytes: podStats.Network.TxBytes,
			}
		}
	}

	return statsMap
}

// updatePodNetworkStats updates network statistics using delta tracking
func (km *kubernetesManager) updatePodNetworkStats(podStat *kubernetes.PodStats, netStats podNetworkStats, podKey string, cacheTimeMs uint16) {
	km.podNetworkSentTrackers[cacheTimeMs].Set(podKey, netStats.TxBytes)
	km.podNetworkRecvTrackers[cacheTimeMs].Set(podKey, netStats.RxBytes)

	// Get deltas (bytes since last measurement)
	sentDelta := km.podNetworkSentTrackers[cacheTimeMs].Delta(podKey)
	recvDelta := km.podNetworkRecvTrackers[cacheTimeMs].Delta(podKey)

	// Convert to MB/s
	cacheTimeSec := float64(cacheTimeMs) / 1000.0
	if sentDelta > 0 && cacheTimeSec > 0 {
		podStat.NetworkSent = float64(sentDelta) / cacheTimeSec / 1024.0 / 1024.0
	}
	if recvDelta > 0 && cacheTimeSec > 0 {
		podStat.NetworkRecv = float64(recvDelta) / cacheTimeSec / 1024.0 / 1024.0
	}
}

// kubeletStatsResponse represents the response from kubelet's /stats/summary endpoint
type kubeletStatsResponse struct {
	Node kubeletNodeStats  `json:"node"`
	Pods []kubeletPodStats `json:"pods"`
}

type kubeletNodeStats struct {
	Network *kubeletNetworkStats `json:"network,omitempty"`
}

type kubeletPodStats struct {
	PodRef  kubeletPodRef        `json:"podRef"`
	Network *kubeletNetworkStats `json:"network,omitempty"`
}

type kubeletPodRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type kubeletNetworkStats struct {
	RxBytes uint64 `json:"rxBytes"`
	TxBytes uint64 `json:"txBytes"`
}

type podNetworkStats struct {
	RxBytes uint64
	TxBytes uint64
}

// getClusterHealth collects cluster-wide health metrics.
// Cached for 60s to avoid API rate limiting.
func (km *kubernetesManager) getClusterHealth() (*kubernetes.ClusterHealth, error) {
	km.Lock()
	defer km.Unlock()

	// Return cached data if fresh (within 60s)
	if time.Since(km.lastClusterSync) < 60*time.Second && km.clusterHealth != nil {
		return km.clusterHealth, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	health := &kubernetes.ClusterHealth{}

	// Get node stats
	nodes, err := km.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err == nil {
		health.NodesTotal = uint16(len(nodes.Items))
		for i := range nodes.Items {
			node := &nodes.Items[i]
			for _, cond := range node.Status.Conditions {
				if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
					health.NodesReady++
					break
				}
			}
		}
	}

	// Get pod stats across all namespaces
	pods, err := km.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err == nil {
		health.PodsTotal = uint32(len(pods.Items))
		for i := range pods.Items {
			pod := &pods.Items[i]
			switch pod.Status.Phase {
			case corev1.PodRunning:
				health.PodsRunning++
			case corev1.PodPending:
				health.PodsPending++
			case corev1.PodFailed:
				health.PodsFailed++
			}
		}
	}

	km.clusterHealth = health
	km.lastClusterSync = time.Now()

	return health, nil
}

// getWorkloadMetrics collects deployment, statefulset, and daemonset metrics.
// Cached for 60s to avoid API rate limiting.
func (km *kubernetesManager) getWorkloadMetrics() (*kubernetes.WorkloadMetrics, error) {
	km.Lock()
	defer km.Unlock()

	// Return cached data if fresh (within 60s)
	if time.Since(km.lastWorkloadSync) < 60*time.Second && km.workloadMetrics != nil {
		return km.workloadMetrics, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	metrics := &kubernetes.WorkloadMetrics{}

	// Get deployments
	deployments, err := km.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err == nil {
		metrics.Deployments = make([]kubernetes.WorkloadInfo, 0, len(deployments.Items))
		for i := range deployments.Items {
			d := &deployments.Items[i]
			metrics.Deployments = append(metrics.Deployments, kubernetes.WorkloadInfo{
				Name:      d.Name,
				Namespace: d.Namespace,
				Replicas:  d.Status.Replicas,
				Ready:     d.Status.ReadyReplicas,
				Available: d.Status.AvailableReplicas,
			})
		}
	}

	// Get statefulsets
	statefulsets, err := km.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err == nil {
		metrics.StatefulSets = make([]kubernetes.WorkloadInfo, 0, len(statefulsets.Items))
		for i := range statefulsets.Items {
			s := &statefulsets.Items[i]
			metrics.StatefulSets = append(metrics.StatefulSets, kubernetes.WorkloadInfo{
				Name:      s.Name,
				Namespace: s.Namespace,
				Replicas:  s.Status.Replicas,
				Ready:     s.Status.ReadyReplicas,
			})
		}
	}

	// Get daemonsets
	daemonsets, err := km.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	if err == nil {
		metrics.DaemonSets = make([]kubernetes.WorkloadInfo, 0, len(daemonsets.Items))
		for i := range daemonsets.Items {
			ds := &daemonsets.Items[i]
			metrics.DaemonSets = append(metrics.DaemonSets, kubernetes.WorkloadInfo{
				Name:      ds.Name,
				Namespace: ds.Namespace,
				Replicas:  ds.Status.DesiredNumberScheduled,
				Ready:     ds.Status.NumberReady,
			})
		}
	}

	km.workloadMetrics = metrics
	km.lastWorkloadSync = time.Now()

	return metrics, nil
}

// getPodLogs retrieves logs from a specific pod
func (km *kubernetesManager) getPodLogs(ctx context.Context, namespace, podName string) (string, error) {
	// Create a pod logs options with tail lines
	podLogOpts := corev1.PodLogOptions{
		TailLines: func(i int64) *int64 { return &i }(500),
	}

	// Get logs from the pod
	req := km.clientset.CoreV1().Pods(namespace).GetLogs(podName, &podLogOpts)
	podLogs, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get pod logs: %w", err)
	}
	defer podLogs.Close()

	// Read logs
	buf := make([]byte, 0, 1024*100) // 100KB buffer
	tmp := make([]byte, 1024)
	for {
		n, err := podLogs.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}

	return string(buf), nil
}

// getPodInfo retrieves detailed information about a specific pod
func (km *kubernetesManager) getPodInfo(ctx context.Context, namespace, podName string) ([]byte, error) {
	pod, err := km.clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod info: %w", err)
	}

	// Marshal pod info to JSON
	return json.Marshal(pod)
}
