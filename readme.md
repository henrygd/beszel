# Beszel

A lightweight server resource monitoring hub with historical data, docker stats, and alerts.

[![Docker Image Size (tag)](https://img.shields.io/docker/image-size/henrygd/beszel-agent/0.0.1-alpha.9?logo=docker&label=agent%20image%20size)](https://hub.docker.com/r/henrygd/beszel-agent)
[![Docker Image Size (tag)](https://img.shields.io/docker/image-size/henrygd/beszel/0.0.1-alpha.9?logo=docker&label=hub%20image%20size)](https://hub.docker.com/r/henrygd/beszel)

![Screenshot of the hub](https://henrygd-assets.b-cdn.net/beszel/screenshot.png)

## Features

- **Lightweight**: Much smaller and less demanding than leading solutions.
- **Docker stats**: CPU and memory usage history for each container.
- **Alerts**: Configurable alerts for CPU, memory, and disk usage, and system status.
- **Multi-user**: Each user has their own systems. Admins can share systems across users.
- **Simple**: Easy setup and doesn't require anything to be publicly available online.
- **OAuth / OIDC**: Supports many OAuth2 providers. Password auth can be disabled.
- **Automatic backups**: Save and restore your data to / from disk or S3-compatible storage.
- **REST API**: Use your metrics in your own scripts and applications.

## Introduction

Beszel has two components: the hub and the agent.

The hub is a web application that provides a dashboard to view and manage your connected systems. It's built on top of [PocketBase](https://pocketbase.io/).

The agent runs on each system you want to monitor. It creates a minimal SSH server through which it communicates system metrics to the hub.

## Getting started

If using the binary instead of docker, ignore 4-5 and run the agent using the binary instead.

1. Start the hub (see [installation](#installation)). The binary command is `beszel serve`.
2. Open http://localhost:8090 and create an admin user.
3. Click "Add system." Enter the name and host of the system you want to monitor.
4. Click "Copy docker compose" to copy the agent's docker-compose.yml file to your clipboard.
5. On the agent system, create the compose file and run `docker compose up` to start the agent.
6. Back in the hub, click the "Add system" button in the dialog to finish adding the system.

If all goes well, you should see the system flip to green. If it goes red, check the Logs page, and see [troubleshooting tips](#faq--troubleshooting).

## Installation

You may choose to install the hub and agent as single binaries, or as docker images.

### Docker

**Hub**: See the example [docker-compose.yml](/hub/docker-compose.yml) file.

**Agent**: The hub provides compose content for the agent, but you can also reference the example [docker-compose.yml](/agent/docker-compose.yml) file.

The agent uses the host network mode so it can access network interface stats. This automatically exposes the port, so change the port using an environment variable if you need to.

If you don't need network stats, remove that line from the compose file and map the port manually.

> **Note**: The docker version of the agent cannot automatically detect the filesystem to use for disk I/O stats, so include the `FILESYSTEM` environment variable if you want that to work ([instructions here](#finding-the-correct-filesystem)).

### Binary

Download and run the latest binaries from the [releases page](https://github.com/henrygd/beszel/releases) or use the commands below.

#### Hub:

```bash
curl -sL "https://github.com/henrygd/beszel/releases/latest/download/beszel_$(uname -s)_$(uname -m | sed 's/x86_64/amd64/' | sed 's/armv7l/arm/' | sed 's/aarch64/arm64/').tar.gz" | tar -xz -O beszel | tee ./beszel >/dev/null && chmod +x beszel && ls beszel
```

##### Running the hub directly

```bash
./beszel serve
```

##### Running the hub as a system service (Linux)

This runs the hub in the background continuously.

1. Create the system service at `/etc/systemd/system/beszel.service`

```bash
[Unit]
Description=Beszel Hub Service
After=network.target

[Service]
# update the values in the curly braces below (remove the braces)
ExecStart={/path/to/working/directory}/beszel serve
WorkingDirectory={/path/to/working/directory}
User={YOUR_USERNAME}
Restart=always

[Install]
WantedBy=multi-user.target
```

2. Start and enable the service to let it run after system boot

```bash
sudo systemctl daemon-reload
sudo systemctl enable beszel.service
sudo systemctl start beszel.service
```

#### Agent:

```bash
curl -sL "https://github.com/henrygd/beszel/releases/latest/download/beszel-agent_$(uname -s)_$(uname -m | sed 's/x86_64/amd64/' | sed 's/armv7l/arm/' | sed 's/aarch64/arm64/').tar.gz" | tar -xz -O beszel-agent | tee ./beszel-agent >/dev/null && chmod +x beszel-agent && ls beszel-agent
```

##### Running the agent directly

```bash
PORT=45876 KEY="{PASTE_YOUR_KEY}" ./beszel-agent
```

##### Running the agent as a system service (Linux)

This runs the agent in the background continuously.

1. Create the system service at `/etc/systemd/system/beszel-agent.service`

```bash
[Unit]
Description=Beszel Agent Service
After=network.target

[Service]
# update the values in curly braces below (remove the braces)
Environment="PORT={PASTE_YOUR_PORT_HERE}"
Environment="KEY={PASTE_YOUR_KEY_HERE}"
ExecStart={/path/to/directory}/beszel-agent
User={YOUR_USERNAME}
Restart=always

[Install]
WantedBy=multi-user.target
```

2. Start and enable the service to let it run after system boot

```bash
sudo systemctl daemon-reload
sudo systemctl enable beszel-agent.service
sudo systemctl start beszel-agent.service
```

#### Updating

Use `beszel update` and `beszel-agent update` to update to the latest version.

## Environment Variables

### Hub

| Name                    | Default | Description                      |
| ----------------------- | ------- | -------------------------------- |
| `DISABLE_PASSWORD_AUTH` | false   | Disables password authentication |

### Agent

| Name         | Default | Description                                                |
| ------------ | ------- | ---------------------------------------------------------- |
| `FILESYSTEM` | unset   | Filesystem / partition to use for disk I/O stats           |
| `KEY`        | unset   | Public SSH key to use for authentication. Provided in hub. |
| `PORT`       | 45876   | Port to listen on                                          |

## OAuth / OIDC setup

Beszel supports OpenID Connect and many OAuth2 authentication providers (see list below). To enable, do the following:

1. Make sure your "Application URL" is set correctly in the PocketBase settings.
2. Create an OAuth2 application using your provider of choice. The redirect / callback URL should be `<your-beszel-url>/api/oauth2-redirect`.
3. When you have the client ID and secret, go to the "Auth providers" page and enable your provider.

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

## REST API

Because Beszel is built on PocketBase, you can use the PocketBase [Web APIs](https://pocketbase.io/docs/api-records/) and [Client-side SDKs](https://pocketbase.io/docs/client-side-sdks/) to read or update data from outside Beszel itself.

## Security

The hub and agent communicate over SSH, so they don't need to be exposed to the internet. And the connection won't break if you put your own auth gateway, such as Authelia, in front of the hub.

When the hub is started for the first time, it generates an ED25519 key pair.

The agent's SSH server is configured to accept connections only using this key. It does not provide a pseudo-terminal or accept input, so it's not possible to execute commands on the agent even if your private key is compromised.

## User roles

### Admin

Assumed to have an admin account in PocketBase, so links to backups, SMTP settings, etc., are shown in the hub.

The first user created automatically becomes an admin and can log into PocketBase.

Please note that changing a user's role will not create a PocketBase admin account for them. If you want to do that, go to Settings > Admins in PocketBase and add them there.

### User

Can create their own systems and alerts. Links to PocketBase settings are not shown in the hub.

### Read only

Cannot create systems, but can view any system that has been shared with them by an admin. Can create alerts.

## FAQ / Troubleshooting

### Agent is not connecting

Assuming the agent is running, the connection is probably being blocked by a firewall. You need to add an inbound rule on the agent system to allow TCP connections to the port. Check any active firewalls, like iptables or ufw, and in your cloud provider account if applicable.

Connectivity can be tested by running `telnet <agent-ip> <port>` or `nc -zv <agent-ip> <port>` from a remote machine.

### Finding the correct filesystem

The filesystem / partition to use for disk I/O stats is specified in the `FILESYSTEM` environment variable.

If it's not set, the agent will try to find the filesystem mounted on `/` and use that. This doesn't seem to work in a container, so it's recommended to set this value. One of the following methods should work (you usually want the option mounted on `/`):

- Run `df -h` and choose an option under "Filesystem"
- Run `lsblk` and choose an option under "NAME"
- Run `sudo fdisk -l` and choose an option under "Device"

### Docker containers are not populating reliably

Try upgrading your docker version on the agent system. I had this issue on a machine running version 24. It was fixed by upgrading to version 27.

### Month / week records are not populating reliably

Records for longer time periods are made by averaging stats from the shorter time periods. They require the agent to be running uninterrupted for long enough to get a full set of data.

If you pause / unpause the agent for longer than one minute, the data will be incomplete and the timing for the current interval will reset.

<!--
## Support

My country, the USA, and many others, are actively involved in the genocide of the Palestinian people. I would greatly appreciate any effort you could make to pressure your government to stop enabling this violence. -->
