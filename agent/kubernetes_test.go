//go:build testing
// +build testing

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/henrygd/beszel/agent/deltatracker"
	"github.com/henrygd/beszel/internal/entities/kubernetes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	fakecorev1 "k8s.io/client-go/kubernetes/typed/core/v1/fake"
	"k8s.io/client-go/rest"
	restfake "k8s.io/client-go/rest/fake"
	k8stesting "k8s.io/client-go/testing"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

// updatePodMetrics should aggregate container usage and scale CPU by node cores.
func TestUpdatePodMetrics(t *testing.T) {
	km := &kubernetesManager{nodeCpuCores: 4}

	podStat := &kubernetes.PodStats{}
	podMetrics := &metricsv1beta1.PodMetrics{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "demo-pod",
		},
		Containers: []metricsv1beta1.ContainerMetrics{
			{
				Name: "c1",
				Usage: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("250m"),
					corev1.ResourceMemory: resource.MustParse("100Mi"),
				},
			},
			{
				Name: "c2",
				Usage: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("750m"),
					corev1.ResourceMemory: resource.MustParse("150Mi"),
				},
			},
		},
	}

	km.updatePodMetrics(podStat, podMetrics, 1000)

	// 1000m total on a 4-core node -> 25%
	assert.InDelta(t, 25.0, podStat.Cpu, 0.01)
	// 100Mi + 150Mi -> 250Mi -> 250 MB
	assert.InDelta(t, 250.0, podStat.Mem, 0.01)
}

func TestGetNodeCpuCapacity(t *testing.T) {
	client := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node1"},
			Status: corev1.NodeStatus{Capacity: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("4"),
			}},
		},
	)

	km := &kubernetesManager{clientset: client, nodeName: "node1"}
	err := km.getNodeCpuCapacity()
	require.NoError(t, err)
	assert.Equal(t, int64(4), km.nodeCpuCores)
}

func TestGetNodeStats(t *testing.T) {
	client := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node1"},
			Status: corev1.NodeStatus{Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			}},
		},
	)

	metricsClient := metricsfake.NewSimpleClientset()
	metricsClient.Fake.PrependReactor("get", "nodes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &metricsv1beta1.NodeMetrics{
			ObjectMeta: metav1.ObjectMeta{Name: "node1"},
			Usage: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("2Gi"),
			},
		}, nil
	})

	km := &kubernetesManager{clientset: client, metricsClient: metricsClient, nodeName: "node1"}

	cpuPercent, memUsedGB, memTotalGB, err := km.getNodeStats()
	require.NoError(t, err)
	assert.InDelta(t, 25.0, cpuPercent, 0.01)
	assert.InDelta(t, 2.0, memUsedGB, 0.01)
	assert.InDelta(t, 8.0, memTotalGB, 0.01)
}

func TestGetNodeStatsMissingMetricsClient(t *testing.T) {
	client := k8sfake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
	)

	km := &kubernetesManager{clientset: client, nodeName: "node1"}
	_, _, _, err := km.getNodeStats()
	assert.Error(t, err)
}

func TestGetNodeMetricsNoClient(t *testing.T) {
	km := &kubernetesManager{}
	_, err := km.getNodeMetrics()
	assert.Error(t, err)
}

// getClusterHealth should count nodes and pods by status.
func TestGetClusterHealth(t *testing.T) {
	client := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-ready"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-not-ready"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-run", Namespace: "default"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-pending", Namespace: "default"},
			Status:     corev1.PodStatus{Phase: corev1.PodPending},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-failed", Namespace: "default"},
			Status:     corev1.PodStatus{Phase: corev1.PodFailed},
		},
	)

	km := &kubernetesManager{clientset: client}

	health, err := km.getClusterHealth()
	require.NoError(t, err)

	assert.Equal(t, uint16(2), health.NodesTotal)
	assert.Equal(t, uint16(1), health.NodesReady)
	assert.Equal(t, uint32(3), health.PodsTotal)
	assert.Equal(t, uint32(1), health.PodsRunning)
	assert.Equal(t, uint32(1), health.PodsPending)
	assert.Equal(t, uint32(1), health.PodsFailed)
}

func TestGetClusterHealthCaches(t *testing.T) {
	client := k8sfake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
	)

	km := &kubernetesManager{clientset: client}
	first, err := km.getClusterHealth()
	require.NoError(t, err)

	// Mutate the cluster after first call
	_, _ = client.CoreV1().Nodes().Create(context.Background(), &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node2"}}, metav1.CreateOptions{})

	second, err := km.getClusterHealth()
	require.NoError(t, err)

	assert.Same(t, first, second)
	assert.Equal(t, uint16(1), second.NodesTotal)
}

// getWorkloadMetrics should return workload counts and replica info.
func TestGetWorkloadMetrics(t *testing.T) {
	client := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "deploy", Namespace: "app"},
			Status: appsv1.DeploymentStatus{
				Replicas:          3,
				ReadyReplicas:     2,
				AvailableReplicas: 2,
			},
		},
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "stateful", Namespace: "db"},
			Status: appsv1.StatefulSetStatus{
				Replicas:      5,
				ReadyReplicas: 4,
			},
		},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "daemon", Namespace: "ops"},
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: 6,
				NumberReady:            5,
			},
		},
	)

	km := &kubernetesManager{clientset: client}

	metrics, err := km.getWorkloadMetrics()
	require.NoError(t, err)

	if assert.Len(t, metrics.Deployments, 1) {
		deploy := metrics.Deployments[0]
		assert.Equal(t, "deploy", deploy.Name)
		assert.Equal(t, "app", deploy.Namespace)
		assert.Equal(t, int32(3), deploy.Replicas)
		assert.Equal(t, int32(2), deploy.Ready)
		assert.Equal(t, int32(2), deploy.Available)
	}

	if assert.Len(t, metrics.StatefulSets, 1) {
		stateful := metrics.StatefulSets[0]
		assert.Equal(t, "stateful", stateful.Name)
		assert.Equal(t, "db", stateful.Namespace)
		assert.Equal(t, int32(5), stateful.Replicas)
		assert.Equal(t, int32(4), stateful.Ready)
	}

	if assert.Len(t, metrics.DaemonSets, 1) {
		daemon := metrics.DaemonSets[0]
		assert.Equal(t, "daemon", daemon.Name)
		assert.Equal(t, "ops", daemon.Namespace)
		assert.Equal(t, int32(6), daemon.Replicas)
		assert.Equal(t, int32(5), daemon.Ready)
	}
}

func TestGetWorkloadMetricsCaches(t *testing.T) {
	client := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "deploy", Namespace: "app"}},
	)

	km := &kubernetesManager{clientset: client}
	first, err := km.getWorkloadMetrics()
	require.NoError(t, err)

	// Add another deployment after first call; should not change cached result
	_, _ = client.AppsV1().Deployments("app").Create(context.Background(), &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "deploy2", Namespace: "app"}}, metav1.CreateOptions{})

	second, err := km.getWorkloadMetrics()
	require.NoError(t, err)

	assert.Same(t, first, second)
	assert.Len(t, second.Deployments, 1)
}

// updatePodNetworkStats should compute deltas per cache interval.
func TestUpdatePodNetworkStats(t *testing.T) {
	cacheTimeMs := uint16(1000)
	km := &kubernetesManager{
		podNetworkSentTrackers: map[uint16]*deltatracker.DeltaTracker[string, uint64]{
			cacheTimeMs: deltatracker.NewDeltaTracker[string, uint64](),
		},
		podNetworkRecvTrackers: map[uint16]*deltatracker.DeltaTracker[string, uint64]{
			cacheTimeMs: deltatracker.NewDeltaTracker[string, uint64](),
		},
	}

	podKey := "default/demo"
	podStat := &kubernetes.PodStats{}

	// Baseline sample
	km.updatePodNetworkStats(podStat, podNetworkStats{RxBytes: 0, TxBytes: 0}, podKey, cacheTimeMs)
	km.podNetworkRecvTrackers[cacheTimeMs].Cycle()
	km.podNetworkSentTrackers[cacheTimeMs].Cycle()

	// Next sample with 50MB recv and 25MB sent over 1 second
	rxBytes := uint64(50 * 1024 * 1024)
	txBytes := uint64(25 * 1024 * 1024)
	km.updatePodNetworkStats(podStat, podNetworkStats{RxBytes: rxBytes, TxBytes: txBytes}, podKey, cacheTimeMs)

	assert.InDelta(t, 50.0, podStat.NetworkRecv, 0.001)
	assert.InDelta(t, 25.0, podStat.NetworkSent, 0.001)
}

func TestGetPodStatsFiltersByNodeAndNamespace(t *testing.T) {
	client := k8sfake.NewClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
			Spec:       corev1.PodSpec{NodeName: "nodeA", Containers: []corev1.Container{{Name: "c1"}, {Name: "c2"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{RestartCount: 1}, {RestartCount: 2}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "ns1"},
			Spec:       corev1.PodSpec{NodeName: "nodeA", Containers: []corev1.Container{{Name: "c1"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodPending},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p3", Namespace: "ns2"},
			Spec:       corev1.PodSpec{NodeName: "nodeA"},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p4", Namespace: "ns1"},
			Spec:       corev1.PodSpec{NodeName: "nodeB"},
		},
	)
	addPodListReactor(client)

	km := newTestKubernetesManager()
	km.clientset = client
	km.nodeName = "nodeA"
	km.namespace = "ns1"

	stats, err := km.getPodStats(1000)
	require.NoError(t, err)
	assert.Len(t, stats, 2)

	byName := map[string]*kubernetes.PodStats{}
	for _, s := range stats {
		byName[s.Name] = s
	}

	if p1, ok := byName["p1"]; assert.True(t, ok) {
		assert.Equal(t, "ns1", p1.Namespace)
		assert.Equal(t, "nodeA", p1.Node)
		assert.Equal(t, uint8(2), p1.ContainerCount)
		assert.Equal(t, int32(3), p1.RestartCount)
		assert.Equal(t, "Running", p1.Status)
	}

	if p2, ok := byName["p2"]; assert.True(t, ok) {
		assert.Equal(t, "Pending", p2.Status)
		assert.Equal(t, uint8(1), p2.ContainerCount)
	}
}

func TestFetchPodMetrics(t *testing.T) {
	t.Run("fetches all namespaces", func(t *testing.T) {
		metricsClient := metricsfake.NewSimpleClientset()
		// Add reactor to return metrics
		metricsClient.Fake.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &metricsv1beta1.PodMetricsList{
				Items: []metricsv1beta1.PodMetrics{
					{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "ns2"}},
				},
			}, nil
		})

		km := &kubernetesManager{metricsClient: metricsClient, namespace: "all"}
		m := km.fetchPodMetrics(context.Background())

		assert.Len(t, m, 2)
		_, hasNS1 := m["ns1/p1"]
		assert.True(t, hasNS1)
		_, hasNS2 := m["ns2/p2"]
		assert.True(t, hasNS2)
	})

	t.Run("filters by namespace", func(t *testing.T) {
		metricsClient := metricsfake.NewSimpleClientset()
		// Add reactor that only returns metrics for requested namespace
		metricsClient.Fake.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
			listAction := action.(k8stesting.ListAction)
			ns := listAction.GetNamespace()
			if ns == "ns1" {
				return true, &metricsv1beta1.PodMetricsList{
					Items: []metricsv1beta1.PodMetrics{
						{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"}},
					},
				}, nil
			}
			return true, &metricsv1beta1.PodMetricsList{}, nil
		})

		km := &kubernetesManager{metricsClient: metricsClient, namespace: "ns1"}
		m := km.fetchPodMetrics(context.Background())

		assert.Len(t, m, 1)
		_, hasNS1 := m["ns1/p1"]
		assert.True(t, hasNS1)
	})
}

// addPodListReactor filters list calls by namespace and spec.nodeName to mimic kube API behavior in tests.
func addPodListReactor(client *k8sfake.Clientset) {
	client.Fake.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		listAction := action.(k8stesting.ListAction)
		ns := listAction.GetNamespace()
		fieldSel := listAction.GetListRestrictions().Fields.String()

		var nodeName string
		if fieldSel != "" {
			if parts := strings.Split(fieldSel, "="); len(parts) == 2 {
				nodeName = parts[1]
			}
		}

		objs, _ := client.Tracker().List(
			corev1.SchemeGroupVersion.WithResource("pods"),
			corev1.SchemeGroupVersion.WithKind("Pod"),
			ns,
			metav1.ListOptions{},
		)
		podList := &corev1.PodList{}
		for _, obj := range objs.(*corev1.PodList).Items {
			if nodeName != "" && obj.Spec.NodeName != nodeName {
				continue
			}
			podList.Items = append(podList.Items, obj)
		}

		return true, podList, nil
	})
}

// newRESTClient returns a REST client that serves kubelet summary responses in order.
func newRESTClient(t *testing.T, responses *[]kubeletStatsResponse) rest.Interface {
	t.Helper()

	return &restfake.RESTClient{
		NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		Client: restfake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			if len(*responses) == 0 {
				return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader("no response"))}, nil
			}
			resp := (*responses)[0]
			*responses = (*responses)[1:]
			data, _ := json.Marshal(resp)

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewReader(data)),
			}, nil
		}),
	}
}

// clientsetWithREST injects a custom REST client while keeping fake client reactions.
type clientsetWithREST struct {
	*k8sfake.Clientset
	rest rest.Interface
}

func newClientsetWithREST(restClient rest.Interface) *clientsetWithREST {
	return &clientsetWithREST{Clientset: k8sfake.NewSimpleClientset(), rest: restClient}
}

func (c *clientsetWithREST) CoreV1() corev1client.CoreV1Interface {
	base := c.Clientset.CoreV1().(*fakecorev1.FakeCoreV1)
	return &coreWithREST{FakeCoreV1: base, rest: c.rest}
}

type coreWithREST struct {
	*fakecorev1.FakeCoreV1
	rest rest.Interface
}

func (c *coreWithREST) RESTClient() rest.Interface { return c.rest }

func (c *coreWithREST) Pods(namespace string) corev1client.PodInterface {
	base := c.FakeCoreV1.Pods(namespace)
	return &podsWithREST{PodInterface: base, ns: namespace, rest: c.rest}
}

type podsWithREST struct {
	corev1client.PodInterface
	ns   string
	rest rest.Interface
}

// GetLogs uses the injected REST client so we can stream fake pod logs in tests.
func (p *podsWithREST) GetLogs(name string, opts *corev1.PodLogOptions) *rest.Request {
	return p.rest.Get().Namespace(p.ns).
		Resource("pods").
		Name(name).
		SubResource("log").
		VersionedParams(opts, scheme.ParameterCodec)
}

// newTestKubernetesManager initializes maps used by delta tracking to avoid nil panics in tests.
func newTestKubernetesManager() *kubernetesManager {
	return &kubernetesManager{
		podStatsMap:             make(map[uint16]map[string]*kubernetes.PodStats),
		lastPodCpu:              make(map[uint16]map[string]uint64),
		lastPodMemory:           make(map[uint16]map[string]uint64),
		podNetworkSentTrackers:  make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
		podNetworkRecvTrackers:  make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
		nodeNetworkSentTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
		nodeNetworkRecvTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
	}
}

func TestDetectNodeNamePrefersEnv(t *testing.T) {
	const envNode = "env-node"
	os.Setenv("NODE_NAME", envNode)
	defer os.Unsetenv("NODE_NAME")

	km := &kubernetesManager{}
	require.NoError(t, km.detectNodeName())
	assert.Equal(t, envNode, km.nodeName)
}

func TestGetPodInfo(t *testing.T) {
	client := k8sfake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-info", Namespace: "ns1"}},
	)

	km := &kubernetesManager{clientset: client}
	data, err := km.getPodInfo(context.Background(), "ns1", "pod-info")
	require.NoError(t, err)

	var pod corev1.Pod
	require.NoError(t, json.Unmarshal(data, &pod))
	assert.Equal(t, "pod-info", pod.Name)
	assert.Equal(t, "ns1", pod.Namespace)
}

func TestGetNodeNetworkStats(t *testing.T) {
	cacheTimeMs := uint16(1000)

	responses := []kubeletStatsResponse{
		{Node: kubeletNodeStats{Network: &kubeletNetworkStats{RxBytes: 0, TxBytes: 0}}},
		{Node: kubeletNodeStats{Network: &kubeletNetworkStats{RxBytes: 50 * 1024 * 1024, TxBytes: 25 * 1024 * 1024}}},
	}

	restClient := newRESTClient(t, &responses)
	client := newClientsetWithREST(restClient)

	km := newTestKubernetesManager()
	km.clientset = client
	km.nodeName = "nodeA"

	// First call seeds trackers (delta should be zero)
	rx, tx, err := km.getNodeNetworkStats(cacheTimeMs)
	require.NoError(t, err)
	assert.InDelta(t, 0.0, rx, 0.001)
	assert.InDelta(t, 0.0, tx, 0.001)

	// Second call calculates deltas over cacheTimeMs
	rx, tx, err = km.getNodeNetworkStats(cacheTimeMs)
	require.NoError(t, err)
	assert.InDelta(t, 50.0, rx, 0.001)
	assert.InDelta(t, 25.0, tx, 0.001)
}

func TestFetchNetworkStats(t *testing.T) {
	responses := []kubeletStatsResponse{
		{
			Pods: []kubeletPodStats{
				{PodRef: kubeletPodRef{Name: "p1", Namespace: "ns1"}, Network: &kubeletNetworkStats{RxBytes: 100, TxBytes: 200}},
				{PodRef: kubeletPodRef{Name: "p2", Namespace: "ns2"}, Network: &kubeletNetworkStats{RxBytes: 300, TxBytes: 400}},
			},
		},
	}

	restClient := newRESTClient(t, &responses)
	client := newClientsetWithREST(restClient)

	km := newTestKubernetesManager()
	km.clientset = client
	km.nodeName = "nodeA"

	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "ns2"}},
	}

	stats := km.fetchNetworkStats(context.Background(), pods)

	if assert.Len(t, stats, 2) {
		assert.Equal(t, uint64(100), stats["ns1/p1"].RxBytes)
		assert.Equal(t, uint64(200), stats["ns1/p1"].TxBytes)
		assert.Equal(t, uint64(300), stats["ns2/p2"].RxBytes)
		assert.Equal(t, uint64(400), stats["ns2/p2"].TxBytes)
	}
}

func TestGetPodLogs(t *testing.T) {
	client := k8sfake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "ns1"}},
	)

	km := newTestKubernetesManager()
	km.clientset = client

	logs, err := km.getPodLogs(context.Background(), "ns1", "pod1")
	require.NoError(t, err)
	// The fake client returns "fake logs" by default
	assert.Equal(t, "fake logs", logs)
}
