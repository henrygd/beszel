# Beszel (Containerd & K8s Labels Edition)

> **Note:** This is a fork of [Beszel](https://github.com/henrygd/beszel) by [henrygd](https://github.com/henrygd). 
> 
> **This fork adds native `containerd` runtime support with Kubernetes label formatting.** It enables the Beszel agent to directly monitor `containerd` containers and present them using their Kubernetes metadata (`namespace / pod name / container name`), bringing full metrics, inspect details, and log viewing to containerd environments.

Beszel is a lightweight server monitoring platform that includes historical data, system metrics, container statistics, and alert functions. It features a friendly web interface, simple configuration, and is ready to use out of the box.

[![MIT license](https://img.shields.io/github/license/henrygd/beszel?color=%239944ee)](https://github.com/henrygd/beszel/blob/main/LICENSE)

![Screenshot of Beszel dashboard and system page](https://henrygd-assets.b-cdn.net/beszel/screenshot-new.png)

## 🌟 What's New in this Fork

Standard Beszel agents collect container metrics via Docker or Podman sockets. This fork introduces a dedicated `ContainerdK8SManager` to collect container metrics directly from the **containerd** runtime while leveraging Kubernetes labels for clean UI representation:

- **Native Containerd Collector:** Connects directly to the `containerd` socket (via cgroups v2) without requiring the Docker daemon.
- **K8s Label Representation:** Extracts `io.kubernetes.*` container labels to format display names as `namespace / pod name / container name` instead of raw container IDs.
- **Full Container Metrics:** Calculates real-time CPU usage, memory consumption, and network I/O delta (`/proc/<pid>/net/dev`) for containerd containers.
- **Detailed Container View:** Provides full JSON metadata inspection (ID, image, status, pid, exit status, labels, snapshotter info).
- **Log Viewer:** Tails and streams logs directly from host paths (`/var/log/containers/`) with ANSI striping for clean UI rendering.
- **Sandbox & Pause Filtering:** Automatically ignores non-Kubernetes background processes and pause containers (e.g. `rancher/mirrored-pause` or `POD` containers).

### Environment Variables
To configure the containerd collector on the agent:
- `CONTAINERD_ADDR`: Path to the containerd socket (e.g., `/run/containerd/containerd.sock`).
- `CONTAINERD_NAMESPACE`: The containerd namespace to target (defaults to `k8s.io`).

## Features

- **Lightweight**: Smaller and less resource-intensive than leading solutions.
- **Simple**: Easy setup with little manual configuration required.
- **Container Stats**: Tracks CPU, memory, and network usage history for Docker, Podman, and **Containerd** containers.
- **Alerts**: Configurable alerts for CPU, memory, disk, bandwidth, temperature, fan speed, load average, and status.
- **Multi-user**: Users manage their own systems. Admins can share systems across users.
- **OAuth / OIDC**: Supports many OAuth2 providers. Password auth can be disabled.
- **Automatic Backups**: Save to and restore from disk or S3-compatible storage.

## Architecture

Beszel consists of two main components: the **hub** and the **agent**.

- **Hub**: A web application built on [PocketBase](https://pocketbase.io/) that provides a dashboard for viewing and managing connected systems.
- **Agent**: Runs on each system you want to monitor and communicates system/container metrics to the hub.

## Getting Started

For general Hub setup, refer to the [official Beszel documentation](https://beszel.dev/guide/getting-started).

To collect containerd metrics using this fork, mount the containerd socket (e.g., `/run/containerd/containerd.sock`) and log path (`/var/log/containers/`) into the agent container and set `CONTAINERD_ADDR`.

## Supported Metrics

- **CPU usage** - Host system and containers.
- **Memory usage** - Host system and containers. Includes swap and ZFS ARC.
- **Disk usage** - Host system. Supports multiple partitions and devices.
- **Disk I/O** - Host system. Supports multiple partitions and devices.
- **Network usage** - Host system and containers.
- **Load average** - Host system.
- **Temperature** - Host system sensors.
- **Fan speed** - Host system sensors (Linux, via `/sys/class/hwmon`).
- **GPU usage / power draw** - Nvidia, AMD, and Intel.
- **Battery** - Host system battery charge.
- **Containers** - Status, metrics, details, and logs for Docker, Podman, and **Containerd** containers.
- **S.M.A.R.T.** - Host system disk health.

## Help and Discussion

- For general Beszel features, hub support, or core issues, visit the [upstream repository](https://github.com/henrygd/beszel).
- For containerd collector or label formatting issues, open an issue in *this* repository.

## License

Beszel is licensed under the MIT License. See the [LICENSE](LICENSE) file for more details.