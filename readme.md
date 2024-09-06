# Beszel

A lightweight server resource monitoring hub with historical data, docker stats, and alerts.

[![Docker Image Size (tag)](https://img.shields.io/docker/image-size/henrygd/beszel-agent/0.1.0?logo=docker&label=agent%20image%20size)](https://hub.docker.com/r/henrygd/beszel-agent)
[![Docker Image Size (tag)](https://img.shields.io/docker/image-size/henrygd/beszel/0.1.0?logo=docker&label=hub%20image%20size)](https://hub.docker.com/r/henrygd/beszel)

![Screenshot of the hub](https://henrygd-assets.b-cdn.net/beszel/screenshot.png)

## Features

- **Lightweight**: Smaller and less resource-intensive than leading solutions.
- **Docker stats**: Tracks CPU and memory usage history for each container.
- **Alerts**: Configurable alerts for CPU, memory, disk usage, and system status.
- **Multi-user**: Each user manages their own systems. Admins can share systems across users.
- **Simple**: Easy setup, no need for public internet exposure.
- **OAuth / OIDC**: Supports multiple OAuth2 providers. Password authentication can be disabled.
- **Automatic backups**: Save and restore data from disk or S3-compatible storage.
- **REST API**: Integrate your metrics into your own scripts and applications.

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

If you don't require network stats, remove that line from the compose file and map the port manually.

> **Note**: If disk I/O stats are missing or incorrect, try using the `FILESYSTEM` environment variable ([instructions here](#finding-the-correct-filesystem)). Check agent logs to see the current device being used.

### Binary

> [!TIP]
> If using Linux, see [guides/systemd.md](/supplemental/guides/systemd.md) for a script to install the hub or agent as a system service. The agent installer will be built into the web UI in the future.

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

| Name                    | Default | Description                             |
| ----------------------- | ------- | --------------------------------------- |
| `DISABLE_PASSWORD_AUTH` | false   | Disables password authentication        |
| `NOTIFICATION_TYPE`     | ""      | Defines which notification type to use. |
| `NOTIFICATION_URL`      | ""      | Notification URL to use for shoutrrr.   |

### Agent

| Name                | Default | Description                                                                              |
| ------------------- | ------- | ---------------------------------------------------------------------------------------- |
| `DOCKER_HOST`       | unset   | Overrides the docker host (docker.sock) if using a proxy.[^socket]                       |
| `EXTRA_FILESYSTEMS` | unset   | See [Monitoring additional disks / partitions](#monitoring-additional-disks--partitions) |
| `FILESYSTEM`        | unset   | Device or partition to use for root disk I/O stats.                                      |
| `KEY`               | unset   | Public SSH key to use for authentication. Provided in hub.                               |
| `PORT`              | 45876   | Port or address:port to listen on.                                                       |

[^socket]: Beszel only needs access to read container information. For [linuxserver/docker-socket-proxy](https://github.com/linuxserver/docker-socket-proxy) you would set `CONTAINERS=1`.

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

## Monitoring additional disks / partitions

> [!NOTE]
> This feature is new and has been tested on a limited number of systems. Please report any issues.

You can configure the agent to monitor the usage and I/O of more than one disk or partition. The approach differs depending on the deployment method.

Use `lsblk` to find the names and mount points of your partitions. If you have trouble, check the agent logs.

### Docker

Mount a folder from the partition's filesystem in the container's `/extra-filesystems` directory, like the example below. The charts will use the name of the device or partition, not the name of the folder.

```yaml
volumes:
  - /mnt/disk1/.beszel:/extra-filesystems/disk1:ro
  - /dev/mmcblk0/.beszel:/extra-filesystems/sd-card:ro
```

### Binary

Set the `EXTRA_FILESYSTEMS` environment variable to a comma-separated list of devices or partitions to monitor. For example:

```bash
EXTRA_FILESYSTEMS="sdb,sdc1,mmcblk0"
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

## Alert services

By default, alerts are sent as emails via the built-in PocketBase service, configurable on the PocketBase Settings page.

[shoutrrr](https://containrrr.dev/shoutrrr) can be used instead by setting the `NOTIFICATION_TYPE` config value to `shoutrrr` and setting a valid `NOTIFICATION_URL` to [a supported service URL](https://containrrr.dev/shoutrrr/services/overview/). These values can be configured in one of two ways:

- From an `alerts.env` file in the `beszel_data` directory in the form of `KEY=value` lines, one per setting
- From environment variables

## FAQ / Troubleshooting

### Agent is not connecting

Assuming the agent is running, the connection is probably being blocked by a firewall. You have two options:

1. Add an inbound rule to the agent system's firewall(s) to allow TCP connections to the port. Check any active firewalls, like iptables, and your cloud provider's firewall settings if applicable.
2. Alternatively, use software like [Cloudflare Tunnel](https://www.cloudflare.com/products/tunnel/), [WireGuard](https://www.wireguard.com/), or [Tailscale](https://tailscale.com/) to securely bypass your firewall.

You can test connectivity by running telnet `<agent-ip> <port>`.

### Connecting the hub and agent on the same system using Docker

If using host network mode for the agent but not the hub, add your system using the hostname `host.docker.internal`, which resolves to the internal IP address used by the host. See the [example docker-compose.yml](/supplemental/docker/same-system/docker-compose.yml).

If using host network mode for both, you can use `localhost` as the hostname.

Otherwise, use the agent's `container_name` as the hostname if both are in the same Docker network.

### Finding the correct filesystem

Specify the filesystem/device/partition for disk I/O stats using the `FILESYSTEM` environment variable.

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

### Prepare dependencies

```bash
cd beszel && go mod tidy
```

### Agent

Go to `beszel/cmd/agent` and run the following command to create a binary in the current directory:

```bash
CGO_ENABLED=0 go build -ldflags "-w -s" .
```

### Hub

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

### Cross-compiling

You can cross-compile for different platforms using the `GOOS` and `GOARCH` environment variables.

For example, to build for FreeBSD ARM64:

```bash
GOOS=freebsd GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "-w -s" .
```

You can see a list of valid options by running `go tool dist list`.

## License

Beszel is licensed under the MIT License. See the [LICENSE](LICENSE) file for more details.
