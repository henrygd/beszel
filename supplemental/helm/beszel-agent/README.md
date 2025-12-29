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

### 1. Get the SSH Key for Authentication

You need an SSH public key to authenticate the agent with the Hub. Generate one if you don't have it:

```bash
# Generate SSH key (if you don't have one)
ssh-keygen -t ed25519 -f ~/.ssh/beszel_agent -N ""
cat ~/.ssh/beszel_agent.pub
```

### 2. Install the Chart

```bash
helm install beszel-agent ./beszel-agent \
  --set env.KEY="ssh-ed25519 AAAA... your-public-key"
```

Or with custom values:

```bash
helm install beszel-agent ./beszel-agent -f custom-values.yaml
```

### 3. Verify the Agent is Running

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
| `env.KEY` | `""` | **REQUIRED** - SSH public key for authentication |
| `env.PORT` | `45876` | Port the agent listens on |
| `image.repository` | `henrygd/beszel-agent` | Container image |
| `image.tag` | Chart AppVersion (0.9) | Image version |
| `hostNetwork` | `false` | Use host network for network monitoring |
| `tolerations` | Allows all taints | Tolerations for running on tainted nodes |

### Minimal Configuration

```bash
helm install beszel-agent ./beszel-agent \
  --set env.KEY="ssh-ed25519 AAAA... your-public-key"
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
  --set env.KEY="ssh-ed25519 AAAA... your-public-key"
```

### Monitor Additional Filesystems

To monitor additional disks or partitions:

```yaml
volumes:
  - name: docker-sock
    hostPath:
      path: /var/run/docker.sock
      type: Socket
  - name: extra-filesystems
    hostPath:
      path: /mnt/disk/.beszel
      type: DirectoryOrCreate

volumeMounts:
  - name: docker-sock
    mountPath: /var/run/docker.sock
    readOnly: true
  - name: extra-filesystems
    mountPath: /extra-filesystems
    readOnly: true

env:
  PORT: "45876"
  KEY: "ssh-ed25519 AAAA... your-public-key"
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
  --set env.KEY="ssh-ed25519 AAAA... your-public-key"
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
```

Or via CLI:

```bash
helm install beszel-agent ./beszel-agent \
  --set daemonset.enabled=false \
  --set replicaCount=1 \
  --set env.KEY="ssh-ed25519 AAAA... your-public-key"
```

### Network Monitoring with Host Network

```yaml
hostNetwork: true

env:
  PORT: "45876"
  KEY: "ssh-ed25519 AAAA... your-public-key"

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
  --set env.KEY="ssh-ed25519 AAAA... new-public-key"

# Change image version
helm upgrade beszel-agent ./beszel-agent \
  --set image.tag="0.10.0"
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

### Using Kubernetes Secrets for SSH Key

```bash
# Create secret
kubectl create secret generic beszel-agent-key \
  --from-literal=key="ssh-ed25519 AAAA... your-public-key"

# Reference in values
env:
  PORT: "45876"
  # KEY will be set from secret in deployment
```

Then update deployment.yaml to reference the secret.

## Support and Documentation

- **Project Homepage**: https://www.beszel.dev/
- **GitHub Repository**: https://github.com/henrygd/beszel
- **Agent Documentation**: https://www.beszel.dev/

## Chart Information

- **Chart Version**: 0.1.0
- **App Version**: 0.9
- **Kubernetes Version**: 1.19+
- **Maintainer**: cloudwithdan (nikoloskid@pm.me)

## License

Please refer to the main Beszel project repository for license information.
