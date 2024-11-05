# Beszel

A lightweight server resource monitoring hub with historical data, docker stats, and alerts.

[![Docker Image Size (tag)](https://img.shields.io/docker/image-size/henrygd/beszel-agent/0.1.0?logo=docker&label=agent%20image%20size)](https://hub.docker.com/r/henrygd/beszel-agent)
[![Docker Image Size (tag)](https://img.shields.io/docker/image-size/henrygd/beszel/0.1.0?logo=docker&label=hub%20image%20size)](https://hub.docker.com/r/henrygd/beszel)
[![Crowdin](https://badges.crowdin.net/beszel/localized.svg)](https://crowdin.com/project/beszel)

![Screenshot of the hub](https://henrygd-assets.b-cdn.net/beszel/screenshot.png)

## Features

- **Lightweight**: Smaller and less resource-intensive than leading solutions.
- **Simple**: Easy setup, no need for public internet exposure.
- **Docker stats**: Tracks CPU, memory, and network usage history for each container.
- **Alerts**: Configurable alerts for CPU, memory, disk, bandwidth, temperature, and system status.
- **Multi-user**: Each user manages their own systems. Admins can share systems across users.
- **OAuth / OIDC**: Supports multiple OAuth2 providers. Password authentication can be disabled.
- **Automatic backups**: Save and restore data from disk or S3-compatible storage.
- **REST API**: Use or update your data in your own scripts and applications.

## Introduction

Beszel consists of two main components: the hub and the agent.

- **Hub:** A web application that provides a dashboard for viewing and managing connected systems. Built on [PocketBase](https://pocketbase.io/).

- **Agent:** Runs on each system you want to monitor, creating a minimal SSH server to communicate system metrics to the hub.

## Getting started

If not using docker, skip steps 4-5 and run the agent using the binary.

1. Start the hub (see [installation](#installation)).
2. Open <http://localhost:8090> and create an admin user.
3. Click "Add system." Enter the name and host of the system you want to monitor.
4. Click "Copy docker compose" to copy the agent's docker-compose.yml file to your clipboard.
5. On the agent system, create the compose file and run `docker compose up` to start the agent.
6. Back in the hub, click the "Add system" button in the dialog to finish adding the system.

If all goes well, the system should flip to green. If it turns red, check the Logs page and refer to [troubleshooting tips](#faq--troubleshooting).

### Tutoriel en français

Pour le tutoriel en français, consultez <https://belginux.com/installer-beszel-avec-docker/>

## Installation

You can install the hub and agent as single binaries or using Docker.

### Docker

**Hub**: See the example [docker-compose.yml](/supplemental/docker/hub/docker-compose.yml) file.

**Agent**: The hub provides compose content for the agent, but you can also reference the example [docker-compose.yml](/supplemental/docker/agent/docker-compose.yml) file.

The agent uses host network mode to access network interface stats, which automatically exposes the port. Change the port using an environment variable if needed.

If you don't need network stats, remove that line from the compose file and map the port manually.

### Binary

> [!TIP]
> If using Linux, see [guides/systemd.md](/supplemental/guides/systemd.md) for a script to install the hub or agent as a system service. This is also built into the web UI.

Download and run the latest binaries from the [releases page](https://github.com/henrygd/beszel/releases) or use the commands below.

#### Hub

```bash
curl -sL "https://github.com/henrygd/beszel/releases/latest/download/beszel_$(uname -s)_$(uname -m | sed 's/x86_64/amd64/' | sed 's/armv7l/arm/' | sed 's/aarch64/arm64/').tar.gz" | tar -xz -O beszel | tee ./beszel >/dev/null && chmod +x beszel && ls beszel
```

Running the hub directly:

```bash
./beszel serve
```

#### Agent

```bash
curl -sL "https://github.com/henrygd/beszel/releases/latest/download/beszel-agent_$(uname -s)_$(uname -m | sed 's/x86_64/amd64/' | sed 's/armv7l/arm/' | sed 's/aarch64/arm64/').tar.gz" | tar -xz -O beszel-agent | tee ./beszel-agent >/dev/null && chmod +x beszel-agent && ls beszel-agent
```

Running the agent directly:

```bash
PORT=45876 KEY="{PASTE_YOUR_KEY}" ./beszel-agent
```

#### Updating

Use `./beszel update` and `./beszel-agent update` to update to the latest version.

## Environment variables

### Hub

| Name                    | Default | Description                                                                                                                                 |
| ----------------------- | ------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `CSP`                   | unset   | Adds a [Content-Security-Policy](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Content-Security-Policy) header with this value. |
| `DISABLE_PASSWORD_AUTH` | false   | Disables password authentication.                                                                                                           |

### Agent

| Name                | Default | Description                                                                                                               |
| ------------------- | ------- | ------------------------------------------------------------------------------------------------------------------------- |
| `DOCKER_HOST`       | unset   | Overrides the docker host (docker.sock) if using a proxy.[^socket]                                                        |
| `EXTRA_FILESYSTEMS` | unset   | See [Monitoring additional disks, partitions, or remote mounts](#monitoring-additional-disks-partitions-or-remote-mounts) |
| `FILESYSTEM`        | unset   | Device, partition, or mount point to use for root disk stats.                                                             |
| `KEY`               | unset   | Public SSH key to use for authentication. Provided in hub.                                                                |
| `LOG_LEVEL`         | info    | Logging level. Valid values: "debug", "info", "warn", "error".                                                            |
| `MEM_CALC`          | unset   | Overrides the default memory calculation.[^memcalc]                                                                       |
| `NICS`              | unset   | Whitelist of network interfaces to monitor for bandwidth chart.                                                           |
| `PORT`              | 45876   | Port or address:port to listen on.                                                                                        |
| `SENSORS`           | unset   | Whitelist of temperature sensors to monitor.                                                                              |
| `SYS_SENSORS`       | unset   | Overrides sys path for sensors. See [#160](https://github.com/henrygd/beszel/discussions/160).                            |

[^socket]: Beszel only needs access to read container information. For [linuxserver/docker-socket-proxy](https://github.com/linuxserver/docker-socket-proxy) you would set `CONTAINERS=1`.
[^memcalc]: The default value for used memory is based on gopsutil's [Used](https://pkg.go.dev/github.com/shirou/gopsutil/v4@v4.24.6/mem#VirtualMemoryStat) calculation, which should align fairly closely with `free`. Set `MEM_CALC` to `htop` to align with htop's calculation.

## OAuth / OIDC Setup

Beszel supports OpenID Connect and many OAuth2 authentication providers (see list below).

Visit the "Auth providers" page to enable your provider. The redirect / callback URL should be `<your-beszel-url>/api/oauth2-redirect`.

<details>
  <summary>Supported provider list</summary>

- Apple
- Bitbucket
- Discord
- Facebook
- Gitea
- Gitee
- GitHub
- GitLab
- Google
- Instagram
- Kakao
- LiveChat
- mailcow
- Microsoft
- OpenID Connect
- Patreon (v2)
- Spotify
- Strava
- Twitch
- Twitter
- VK
- Yandex

</details>

## Monitoring additional disks, partitions, or remote mounts

The method for adding additional disks differs depending on your deployment method.

Use `lsblk` to find the names and mount points of your partitions. If you have trouble, check the agent logs.

> Note: The charts will use the name of the device or partition if available, and fall back to the folder name. You will not get I/O stats for network mounted drives.

### Docker

Mount a folder from the target filesystem in the container's `/extra-filesystems` directory. For example:

```yaml
volumes:
  - /mnt/disk1/.beszel:/extra-filesystems/sdb1:ro
  - /dev/mmcblk0/.beszel:/extra-filesystems/mmcblk0:ro
```

### Binary

Set the `EXTRA_FILESYSTEMS` environment variable to a comma-separated list of devices, partitions, or mount points to monitor. For example:

```bash
EXTRA_FILESYSTEMS="sdb,sdc1,mmcblk0,/mnt/network-share"
```

## REST API

Because Beszel is built on PocketBase, you can use the PocketBase [web APIs](https://pocketbase.io/docs/api-records/) and [client-side SDKs](https://pocketbase.io/docs/client-side-sdks/) to read or update data from outside Beszel itself.

## Security

The hub and agent communicate over SSH, so they don't need to be exposed to the internet. Even if you place an external auth gateway, such as Authelia, in front of the hub, it won't disrupt or break the connection between the hub and agent.

When the hub is started for the first time, it generates an ED25519 key pair.

The agent's SSH server is configured to accept connections using this key only. It does not provide a pseudo-terminal or accept input, so it's impossible to execute commands on the agent even if your private key is compromised.

## User roles

### Admin

Admins have access to additional links in the hub, such as backups, SMTP settings, etc. The first user created is automatically an admin and can log into PocketBase.

Changing a user's role does not create a PocketBase admin account for them. To do that, go to Settings > Admins in PocketBase and add them manually.

### User

Users can create their own systems and alerts. Links to PocketBase settings are not shown in the hub.

### Read only

Read-only users cannot create systems but can view any system shared with them by an admin and create alerts.

## FAQ / Troubleshooting

### Agent is not connecting

Assuming the agent is running, the connection is probably being blocked by a firewall. You have two options:

1. Add an inbound rule to the agent system's firewall(s) to allow TCP connections to the port. Check any active firewalls, like iptables, and your cloud provider's firewall settings if applicable.
2. Alternatively, use software like [Cloudflare Tunnel](https://www.cloudflare.com/products/tunnel/), [WireGuard](https://www.wireguard.com/), or [Tailscale](https://tailscale.com/) to securely bypass your firewall.

You can test connectivity by running `telnet <agent-ip> <port>`.

### Connecting the hub and agent on the same system using Docker

If using host network mode for the agent but not the hub, add your system using the hostname `host.docker.internal`, which resolves to the internal IP address used by the host. See the [example docker-compose.yml](/supplemental/docker/same-system/docker-compose.yml).

If using host network mode for both, you can use `localhost` as the hostname.

Otherwise, use the agent's `container_name` as the hostname if both are in the same Docker network.

### Finding the correct filesystem

Specify the filesystem/device/partition for root disk stats using the `FILESYSTEM` environment variable.

If not set, the agent will try to find the partition mounted on `/` and use that. This may not work correctly in a container, so it's recommended to set this value. Use one of the following methods to find the correct filesystem:

- Run `lsblk` and choose an option under "NAME."
- Run `df -h` and choose an option under "Filesystem."
- Run `sudo fdisk -l` and choose an option under "Device."

### Docker container charts are empty or missing

If container charts show empty data or don't appear at all, you may need to enable cgroup memory accounting. To verify, run `docker stats`. If that shows zero memory usage, follow this guide to fix the issue:

<https://akashrajpurohit.com/blog/resolving-missing-memory-stats-in-docker-stats-on-raspberry-pi/>

### Docker Containers Are Not Populating Reliably

Try upgrading your Docker version on the agent system. This issue was observed on a machine running version 24 and was resolved by upgrading to version 27.

### Month / week records are not populating reliably

Records for longer time periods are created by averaging stats from shorter periods. The agent must run uninterrupted for a full set of data to populate these records.

Pausing/unpausing the agent for longer than one minute will result in incomplete data, resetting the timing for the current interval.

## Compiling

Both the hub and agent are written in Go, so you can easily build them yourself, or cross-compile for different platforms. Please [install Go](https://go.dev/doc/install) first if you haven't already.

### Using Makefile

Run `make` in `/beszel`. This creates a `build` directory containing the binaries.

```bash
cd beszel && make
```

You can also build for different platforms:

```bash
make OS=freebsd ARCH=arm64
```

See a list of valid options by running `go tool dist list`.

### Manual compilation

#### Prepare dependencies

```bash
cd beszel && go mod tidy
```

#### Agent

Go to `beszel/cmd/agent` and run the following command to create a binary in the current directory:

```bash
CGO_ENABLED=0 go build -ldflags "-w -s" .
```

#### Hub

The hub embeds the web UI in the binary, so you must build the website first. I use [Bun](https://bun.sh/), but you may use Node.js if you prefer:

```bash
cd beszel/site
bun install
bun run build
```

Then in `beszel/cmd/hub`:

```bash
CGO_ENABLED=0 go build -ldflags "-w -s" .
```

#### Cross-compiling

You can cross-compile for different platforms using the `GOOS` and `GOARCH` environment variables.

For example, to build for FreeBSD ARM64:

```bash
GOOS=freebsd GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "-w -s" .
```

See a list of valid options by running `go tool dist list`.

## Contributing

Contributions are welcome, but it's a good idea to check with us first in a discussion / issue if you plan on doing anything significant.

We use [Crowdin](https://crowdin.com/project/beszel) to manage translations. New languages or improvements to existing translations are appreciated!

We'll have more helpful information about contributing to Beszel in the near future.

## License

Beszel is licensed under the MIT License. See the [LICENSE](LICENSE) file for more details.
