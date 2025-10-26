# Beszel

Beszel is a lightweight server monitoring platform that includes Docker statistics, historical data, and alert functions.

It has a friendly web interface, simple configuration, and is ready to use out of the box. It supports automatic backup, multi-user, OAuth authentication, and API access.

[![agent Docker Image Size](https://img.shields.io/docker/image-size/henrygd/beszel-agent/latest?logo=docker&label=agent%20image%20size)](https://hub.docker.com/r/henrygd/beszel-agent)
[![hub Docker Image Size](https://img.shields.io/docker/image-size/henrygd/beszel/latest?logo=docker&label=hub%20image%20size)](https://hub.docker.com/r/henrygd/beszel)
[![MIT license](https://img.shields.io/github/license/henrygd/beszel?color=%239944ee)](https://github.com/henrygd/beszel/blob/main/LICENSE)
[![Crowdin](https://badges.crowdin.net/beszel/localized.svg)](https://crowdin.com/project/beszel)

![Screenshot of Beszel dashboard and system page, side by side. The dashboard shows metrics from multiple connected systems, while the system page shows detailed metrics for a single system.](https://henrygd-assets.b-cdn.net/beszel/screenshot-new.png)

## Features

- **Lightweight**: Smaller and less resource-intensive than leading solutions.
- **Simple**: Easy setup with little manual configuration required.
- **Docker stats**: Tracks CPU, memory, and network usage history for each container.
- **Alerts**: Configurable alerts for CPU, memory, disk, bandwidth, temperature, load average, and status.
- **Multi-user**: Users manage their own systems. Admins can share systems across users.
- **OAuth / OIDC**: Supports many OAuth2 providers. Password auth can be disabled.
- **Automatic backups**: Save to and restore from disk or S3-compatible storage.
<!-- - **REST API**: Use or update your data in your own scripts and applications. -->

## Architecture

Beszel consists of two main components: the **hub** and the **agent**.

- **Hub**: A web application built on [PocketBase](https://pocketbase.io/) that provides a dashboard for viewing and managing connected systems.
- **Agent**: Runs on each system you want to monitor and communicates system metrics to the hub.

## Getting started

The [quick start guide](https://beszel.dev/guide/getting-started) and other documentation is available on our website, [beszel.dev](https://beszel.dev). You'll be up and running in a few minutes.

## Screenshots

![Dashboard](https://beszel.dev/image/dashboard.png)
![System page](https://beszel.dev/image/system-full.png)
![Notification Settings](https://beszel.dev/image/settings-notifications.png)

## Supported metrics

- **CPU usage** - Host system and Docker / Podman containers.
- **Memory usage** - Host system and containers. Includes swap and ZFS ARC.
- **Disk usage** - Host system. Supports multiple partitions and devices.
- **Disk I/O** - Host system. Supports multiple partitions and devices.
- **Network usage** - Host system and containers.
- **Load average** - Host system.
- **Temperature** - Host system sensors.
- **GPU usage / power draw** - Nvidia, AMD, and Intel.
- **Battery** - Host system battery charge.
- **Containers** - Status and metrics of all running Docker / Podman containers.
- **S.M.A.R.T.** - Host system disk health.

## Help and discussion

Please search existing issues and discussions before opening a new one. I try my best to respond, but may not always have time to do so.

#### Bug reports and feature requests

Bug reports and feature requests can be posted on [GitHub issues](https://github.com/henrygd/beszel/issues).

#### Support and general discussion

Support requests and general discussion can be posted on [GitHub discussions](https://github.com/henrygd/beszel/discussions) or the community-run [Matrix room](https://matrix.to/#/#beszel:matrix.org): `#beszel:matrix.org`.

## License

Beszel is licensed under the MIT License. See the [LICENSE](LICENSE) file for more details.
