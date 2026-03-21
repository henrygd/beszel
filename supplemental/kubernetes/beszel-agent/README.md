# Beszel Agent Helm Chart

This Helm chart deploys the Beszel monitoring agent as a DaemonSet in your Kubernetes cluster with full RBAC permissions for pod monitoring.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- Kubernetes Metrics Server installed

## Installation

### 1. Install the chart

```bash
helm install beszel-agent ./beszel-agent \
  --namespace monitoring \
  --create-namespace \
  --set agent.hub.url="your-hub-url:45876" \
  --set agent.hub.key="your-ssh-key-here" \
  --set agent.hub.token="your-api-token-here"
```

### 3. Verify the installation

```bash
kubectl get daemonset -n monitoring
kubectl get pods -n monitoring -l app.kubernetes.io/name=beszel-agent
```

## Configuration

The following table lists the configurable parameters of the Beszel Agent chart and their default values.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | Agent image repository | `henrygd/beszel-agent` |
| `image.tag` | Agent image tag | Chart appVersion |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `serviceAccount.create` | Create service account | `true` |
| `serviceAccount.name` | Service account name | `beszel-agent` |
| `rbac.create` | Create RBAC resources | `true` |
| `agent.hub.url` | Beszel hub URL | `""` (required) |
| `agent.hub.key` | SSH key for authentication | `""` (required) |
| `agent.hub.token` | API token for authentication | `""` (required) |
| `agent.kubernetes.clusterMetrics` | Enable cluster-wide metrics | `true` |
| `agent.kubernetes.namespace` | Namespace to monitor pods (empty for all) | `""` |
| `agent.monitorHost` | Enable host node monitoring (requires privileged mode) | `true` |
| `resources.requests.cpu` | CPU request | `100m` |
| `resources.requests.memory` | Memory request | `128Mi` |
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `512Mi` |

## RBAC Permissions

The chart automatically creates the following RBAC resources:

- **ServiceAccount**: `beszel-agent` (in the release namespace)
- **ClusterRole**: Read-only permissions for:
  - Nodes (metrics and health)
  - Nodes/stats and nodes/proxy (for network metrics from kubelet)
  - Pods (metrics and status)
  - Pods/log (for log viewing)
  - Pod metrics from Metrics Server
  - Deployments, StatefulSets, DaemonSets
- **ClusterRoleBinding**: Grants permissions to the ServiceAccount

These permissions are **read-only** and follow the principle of least privilege.

## Security Considerations

### Privileged Mode (Host Monitoring)
When `agent.monitorHost: true` (default), the DaemonSet runs with privileged access to monitor the underlying host node's CPU, memory, disk, and network. This grants:
- `privileged: true` security context
- `SYS_ADMIN` and `NET_ADMIN` capabilities
- Host filesystem mount at `/host`
- Host network and PID namespace access

**⚠️ Security Impact**: Privileged mode provides broad access to the host system. Only enable this if you need to monitor node-level metrics.

### Kubernetes-Only Mode (Recommended for Production)
Set `agent.monitorHost: false` to run with minimal privileges:
- `runAsNonRoot: true` and `runAsUser: 1000`
- `readOnlyRootFilesystem: true`
- No privileged access or host mounts
- Only monitors Kubernetes pods and cluster resources

This mode is recommended when you only need pod-level metrics and don't require host system monitoring.

## Features

- **Pod Monitoring**: CPU, memory, and network metrics per pod
- **Network Stats**: Fetches network I/O from kubelet stats/summary endpoint
- **Cluster Health**: Node and pod status across the cluster
- **Workload Metrics**: Deployment, StatefulSet, and DaemonSet status
- **Pod Logs**: Access pod logs directly from the Beszel UI
- **Auto-Detection**: Automatically enables Kubernetes features when running in-cluster

## Uninstallation

```bash
helm uninstall beszel-agent -n monitoring
```

## Upgrading

```bash
helm upgrade beszel-agent ./beszel-agent \
  --namespace monitoring \
  --reuse-values
```

## Troubleshooting

### Check agent logs
```bash
kubectl logs -n monitoring -l app.kubernetes.io/name=beszel-agent
```

### Verify RBAC permissions
```bash
kubectl auth can-i get pods --as=system:serviceaccount:monitoring:beszel-agent
kubectl auth can-i get nodes/proxy --as=system:serviceaccount:monitoring:beszel-agent
```

### Check Metrics Server
```bash
kubectl get deployment metrics-server -n kube-system
kubectl top nodes
kubectl top pods --all-namespaces
```

## Support

For issues and questions:
- GitHub: https://github.com/henrygd/beszel
- Documentation: https://github.com/henrygd/beszel#readme
