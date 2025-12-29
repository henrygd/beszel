// Package kubernetes provides entities for Kubernetes metrics collection.
//
// This package defines data structures for pod statistics, cluster health,
// and workload metrics collected from Kubernetes API and Metrics Server.
package kubernetes

import "time"

// PodStats represents metrics for a single pod running on a Kubernetes node.
// Similar to container.Stats but for Kubernetes pods with additional K8s-specific fields.
type PodStats struct {
	Name           string    `json:"name" cbor:"0,keyasint"`                     // Pod name
	Namespace      string    `json:"namespace" cbor:"1,keyasint"`                // Kubernetes namespace
	Node           string    `json:"node,omitempty" cbor:"2,keyasint,omitempty"` // Node name (optional)
	Cpu            float64   `json:"cpu" cbor:"3,keyasint"`                      // CPU usage percentage
	Mem            float64   `json:"m" cbor:"4,keyasint"`                        // Memory usage in megabytes
	MemLimit       float64   `json:"ml,omitempty" cbor:"5,keyasint,omitempty"`   // Memory limit in megabytes
	NetworkSent    float64   `json:"ns" cbor:"6,keyasint"`                       // Network sent per second (MB/s)
	NetworkRecv    float64   `json:"nr" cbor:"7,keyasint"`                       // Network received per second (MB/s)
	Status         string    `json:"s" cbor:"8,keyasint"`                        // Pod phase (Running, Pending, etc.)
	RestartCount   int32     `json:"rc,omitempty" cbor:"9,keyasint,omitempty"`   // Total container restart count
	ContainerCount uint8     `json:"cc" cbor:"10,keyasint"`                      // Number of containers in pod
	PrevReadTime   time.Time `json:"-" cbor:"-"`                                 // Last read time for delta calculation
}

// ClusterHealth represents cluster-wide health metrics.
// Collected less frequently (60s cache) to avoid API rate limiting.
type ClusterHealth struct {
	NodesTotal  uint16 `json:"nt" cbor:"0,keyasint"` // Total nodes in cluster
	NodesReady  uint16 `json:"nr" cbor:"1,keyasint"` // Nodes in Ready state
	PodsTotal   uint32 `json:"pt" cbor:"2,keyasint"` // Total pods across all namespaces
	PodsRunning uint32 `json:"pr" cbor:"3,keyasint"` // Pods in Running phase
	PodsPending uint32 `json:"pp" cbor:"4,keyasint"` // Pods in Pending phase
	PodsFailed  uint32 `json:"pf" cbor:"5,keyasint"` // Pods in Failed phase
}

// WorkloadMetrics represents deployment/statefulset/daemonset metrics.
// Tracks workload objects and their replica status.
type WorkloadMetrics struct {
	Deployments  []WorkloadInfo `json:"d,omitempty" cbor:"0,keyasint,omitempty"`  // Deployment status
	StatefulSets []WorkloadInfo `json:"s,omitempty" cbor:"1,keyasint,omitempty"`  // StatefulSet status
	DaemonSets   []WorkloadInfo `json:"ds,omitempty" cbor:"2,keyasint,omitempty"` // DaemonSet status
}

// WorkloadInfo contains status information for a single workload object.
type WorkloadInfo struct {
	Name      string `json:"n" cbor:"0,keyasint"`                     // Workload name
	Namespace string `json:"ns" cbor:"1,keyasint"`                    // Kubernetes namespace
	Replicas  int32  `json:"r" cbor:"2,keyasint"`                     // Desired replica count
	Ready     int32  `json:"rr" cbor:"3,keyasint"`                    // Ready replica count
	Available int32  `json:"a,omitempty" cbor:"4,keyasint,omitempty"` // Available replica count (deployments only)
}
