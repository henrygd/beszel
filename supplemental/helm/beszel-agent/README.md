# Beszel Agent Helm Chart

A Kubernetes Helm chart for deploying [Beszel Agent](https://www.beszel.dev/) - a lightweight monitoring agent that collects system metrics and sends them to a central Beszel Hub.

## Overview

This Helm chart simplifies the deployment of Beszel Agent in Kubernetes environments. By default, it deploys as a DaemonSet to run one agent on each node in the cluster. The agent monitors system resources, containers, and services, providing detailed metrics to the Beszel Hub for centralized monitoring and alerting.

## Features

- ✅ DaemonSet deployment by default (one agent per node)
- ✅ GPU support via NVIDIA runtime (optional)
- ✅ Additional filesystem mounting for multi-disk monitoring
- ✅ Flexible deployment as DaemonSet or single Deployment
- ✅ Environment variable configuration for agent authentication
- ✅ Host network support for detailed network monitoring
- ✅ Automatic handling of tainted nodes via tolerations

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- Beszel Hub instance running and accessible
- SSH public key for agent authentication

## Quick Start

### 1. Install the Chart

```bash
helm install beszel-agent ./beszel-agent \
  --set env.KEY="ssh-ed25519 AAAA... your-public-key" \
  --set env.TOKEN="your-token-value" \
  --set env.HUB_URL="http://beszel-hub:8090"
```

Or with custom values:

```bash
helm install beszel-agent ./beszel-agent -f custom-values.yaml
```

### 2. Verify the Agent is Running

```bash
kubectl get pods -l app.kubernetes.io/name=beszel-agent
kubectl logs -l app.kubernetes.io/name=beszel-agent
```

## Configuration

### Basic Configuration

Essential parameters to configure:

| Parameter | Default | Description |
|-----------|---------|-------------|
| `daemonset.enabled` | `true` | Deploy as DaemonSet (one pod per node) |
| `env.KEY` | Required | SSH public key for Hub authentication |
| `env.TOKEN` | Empty | Authentication token (optional) |
| `env.HUB_URL` | Empty | Hub URL (e.g., http://beszel-hub:8090) |
| `env.PORT` | `45876` | Port the agent listens on |
| `image.repository` | `henrygd/beszel-agent` | Container image |
| `image.tag` | Chart AppVersion (0.17.0) | Image version |
| `hostNetwork` | `false` | Use host network for network monitoring |
| `tolerations` | Allows all taints | Tolerations for running on tainted nodes |

### Minimal Configuration

```bash
helm install beszel-agent ./beszel-agent \
  --set env.KEY="ssh-ed25519 AAAA... your-public-key" \
  --set env.TOKEN="your-token-value" \
  --set env.HUB_URL="http://beszel-hub:8090"
```

### Standard Configuration

```yaml
# values.yaml
image:
  repository: henrygd/beszel-agent
  tag: ""  # Uses chart appVersion

env:
  PORT: "45876"
  KEY: "ssh-ed25519 AAAA... your-public-key"
  TOKEN: "your-token-value"
  HUB_URL: "http://beszel-hub:8090"

# Use host network for accurate network monitoring
hostNetwork: false
```

### GPU Support (NVIDIA)

For systems with NVIDIA GPUs:

```yaml
image:
  repository: henrygd/beszel-agent-nvidia

# Enable NVIDIA runtime
gpuRuntime: nvidia

env:
  PORT: "45876"
  KEY: "ssh-ed25519 AAAA... your-public-key"
  TOKEN: "your-token-value"
  HUB_URL: "http://beszel-hub:8090"
  NVIDIA_VISIBLE_DEVICES: "all"
  NVIDIA_DRIVER_CAPABILITIES: "compute,video,utility"
```

Or via CLI:

```bash
helm install beszel-agent ./beszel-agent \
  --set image.repository=henrygd/beszel-agent-nvidia \
  --set gpuRuntime=nvidia \
  --set env.NVIDIA_VISIBLE_DEVICES=all \
  --set env.NVIDIA_DRIVER_CAPABILITIES="compute,video,utility" \
  --set env.KEY="ssh-ed25519 AAAA... your-public-key" \
  --set env.TOKEN="your-token-value" \
  --set env.HUB_URL="http://beszel-hub:8090"
```

### Monitor Additional Filesystems

To monitor additional disks or partitions:

```yaml
volumes:
  - name: extra-filesystems
    hostPath:
      path: /mnt/disk/.beszel
      type: DirectoryOrCreate

volumeMounts:
  - name: extra-filesystems
    mountPath: /extra-filesystems
    readOnly: true

env:
  PORT: "45876"
  KEY: "ssh-ed25519 AAAA... your-public-key"
  TOKEN: "your-token-value"
  HUB_URL: "http://beszel-hub:8090"
```

### Advanced Configuration

#### Resource Limits

```yaml
resources:
  limits:
    cpu: 500m
    memory: 256Mi
  requests:
    cpu: 100m
    memory: 128Mi
```

#### Node Selection

Run agents on specific nodes:

```yaml
nodeSelector:
  monitoring: "true"

tolerations:
  - key: monitoring
    operator: Equal
    value: "true"
    effect: NoSchedule

affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchExpressions:
              - key: app.kubernetes.io/name
                operator: In
                values:
                  - beszel-agent
          topologyKey: kubernetes.io/hostname
```

#### Host Network

For detailed network statistics:

```yaml
hostNetwork: true

env:
  PORT: "45876"
  KEY: "ssh-ed25519 AAAA... your-public-key"
  TOKEN: "your-token-value"
  HUB_URL: "http://beszel-hub:8090"
```

#### DaemonSet Mode

By default, the agent is deployed as a DaemonSet, running one pod on each cluster node:

```yaml
daemonset:
  enabled: true  # Default - one agent per node

# Or disable for single Deployment deployment
daemonset:
  enabled: false
replicaCount: 1
```

#### Tolerations

By default, tolerations are set to allow agents to run on all nodes, including tainted ones:

```yaml
tolerations:
  - operator: Exists
    effect: NoSchedule
  - operator: Exists
    effect: NoExecute
```

To restrict agents to specific nodes:

```yaml
tolerations: []
nodeSelector:
  monitoring: "true"
```

## Deployment Examples

### Full Cluster Monitoring (DaemonSet - Default)

```bash
helm install beszel-agent ./beszel-agent \
  --set env.KEY="ssh-ed25519 AAAA... your-public-key" \
  --set env.TOKEN="your-token-value" \
  --set env.HUB_URL="http://beszel-hub:8090"
```

This deploys one agent on every node in the cluster automatically.

### Single Agent Deployment (Non-DaemonSet)

```yaml
# values.yaml
daemonset:
  enabled: false

replicaCount: 1

env:
  PORT: "45876"
  KEY: "ssh-ed25519 AAAA... your-public-key"
  TOKEN: "your-token-value"
  HUB_URL: "http://beszel-hub:8090"
```

Or via CLI:

```bash
helm install beszel-agent ./beszel-agent \
  --set daemonset.enabled=false \
  --set replicaCount=1 \
  --set env.KEY="ssh-ed25519 AAAA... your-public-key" \
  --set env.TOKEN="your-token-value" \
  --set env.HUB_URL="http://beszel-hub:8090"
```

### Network Monitoring with Host Network

```yaml
hostNetwork: true

env:
  PORT: "45876"
  KEY: "ssh-ed25519 AAAA... your-public-key"
  TOKEN: "your-token-value"
  HUB_URL: "http://beszel-hub:8090"

podSecurityContext:
  hostNetwork: true
```

## Managing the Agent

### Check Agent Status

```bash
# List agent pods
kubectl get pods -l app.kubernetes.io/name=beszel-agent

# View agent logs
kubectl logs -l app.kubernetes.io/name=beszel-agent -f

# Describe a specific pod
kubectl describe pod <pod-name>
```

### Update Configuration

```bash
# Update the SSH key
helm upgrade beszel-agent ./beszel-agent \
  --set env.KEY="ssh-ed25519 AAAA... your-public-key" \
  --set env.TOKEN="your-token-value" \
  --set env.HUB_URL="http://beszel-hub:8090"

# Change image version
helm upgrade beszel-agent ./beszel-agent \
  --set image.tag="0.17.0"
```

### Restart All Agents

```bash
kubectl rollout restart deployment beszel-agent
```

### Uninstall

```bash
helm uninstall beszel-agent
```

### View Helm Release History

```bash
helm history beszel-agent
helm rollback beszel-agent 1  # Rollback to previous version
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `45876` | Port the agent listens on |
| `KEY` | Required | SSH public key for Hub authentication |
| `TOKEN` | Empty | Authentication token (optional) |
| `HUB_URL` | Empty | Hub URL (e.g., http://beszel-hub:8090) |
| `NVIDIA_VISIBLE_DEVICES` | Not set | GPU visibility (GPU agents only) |
| `NVIDIA_DRIVER_CAPABILITIES` | Not set | GPU capabilities (GPU agents only) |

## Troubleshooting

### Agent Pod Won't Start

```bash
# Check pod events and logs
kubectl describe pod <pod-name>
kubectl logs <pod-name>

# Check if Docker socket is accessible
kubectl exec <pod-name> -- ls -la /var/run/docker.sock
```

### Cannot Connect to Hub

- Verify Hub is accessible from the pod's network
- Check DNS resolution: `kubectl exec <pod-name> -- nslookup beszel-hub.default.svc.cluster.local`
- Verify SSH key is correctly configured
- Check firewall rules for port 8090 (Hub) and 45876 (Agent)

### Docker Monitoring Not Working

- Verify Docker socket is mounted: `kubectl exec <pod-name> -- ls -la /var/run/docker.sock`
- Check socket permissions
- Ensure agent has Docker socket read permission
- Verify the socket path is correct for your system

### GPU Not Detected

- Confirm image is `henrygd/beszel-agent-nvidia`
- Verify NVIDIA runtime is installed on nodes
- Check GPU visibility: `kubectl exec <pod-name> -- nvidia-smi`
- Verify runtimeClassName matches your GPU runtime

### SSH Key Authentication Failed

- Verify key format (should be valid SSH public key)
- Check key is correctly set in `env.KEY`
- Ensure Hub has the corresponding private key
- Verify Hub can authenticate agents with this key

### High Memory Usage

Adjust resource limits:

```yaml
resources:
  limits:
    memory: 512Mi
  requests:
    memory: 256Mi
```

## Security Considerations

- Store SSH keys securely (use Kubernetes Secrets)
- Restrict container to read-only root filesystem if possible
- Limit resource usage with resource limits
- Use network policies to restrict traffic
- Run with minimal privileges
- Regularly update agent image to latest version
- Use private container registries if applicable

### Using Kubernetes Secrets for Configuration

The chart automatically creates a Kubernetes Secret to store sensitive authentication data:

```bash
# Install with all configuration options
helm install beszel-agent ./beszel-agent \
  --set env.KEY="ssh-ed25519 AAAA... your-public-key" \
  --set env.TOKEN="your-optional-token" \
  --set env.HUB_URL="http://beszel-hub:8090"
```

Or create the installation with a values file:

```yaml
# values.yaml
env:
  KEY: "ssh-ed25519 AAAA... your-public-key"
  TOKEN: "your-optional-token"
  HUB_URL: "http://beszel-hub:8090"
```

Configuration stored in Kubernetes Secrets (encrypted at rest):
- `KEY` - SSH public key for authentication (required)
- `TOKEN` - Authentication token (optional)

Configuration as regular environment variables:
- `HUB_URL` - Hub address (e.g., http://beszel-hub:8090 or https://beszel.example.com)

To verify the secret was created:

```bash
kubectl get secret beszel-agent
kubectl get secret beszel-agent -o jsonpath='{.data.ssh-key}' | base64 -d
```

## Support and Documentation

- **Project Homepage**: https://www.beszel.dev/
- **GitHub Repository**: https://github.com/henrygd/beszel
- **Agent Documentation**: https://www.beszel.dev/

## Chart Information

- **Chart Version**: 0.1.0
- **App Version**: 0.17.0
- **Kubernetes Version**: 1.19+
- **Maintainer**: cloudwithdan (nikoloskid@pm.me)

## License

Please refer to the main Beszel project repository for license information.
