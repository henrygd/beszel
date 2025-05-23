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
- **Simple**: Easy setup, no need for public internet exposure.
- **Docker stats**: Tracks CPU, memory, and network usage history for each container.
- **Alerts**: Configurable alerts for CPU, memory, disk, bandwidth, temperature, and status.
- **Multi-user**: Users manage their own systems. Admins can share systems across users.
- **OAuth / OIDC**: Supports many OAuth2 providers. Password auth can be disabled.
- **Automatic backups**: Save and restore data from disk or S3-compatible storage.
- **REST API**: Use or update your data in your own scripts and applications.

## Architecture

Beszel consists of two main components: the **hub** and the **agent**.

- **Hub**: A web application built on [PocketBase](https://pocketbase.io/) that provides a dashboard for viewing and managing connected systems.
- **Agent**: Runs on each system you want to monitor, creating a minimal SSH server to communicate system metrics to the hub.

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
- **Temperature** - Host system sensors.
- **GPU usage / temperature / power draw** - Nvidia and AMD only. Must use binary agent.

## Discussion and reporting issues

- Report issues on the [GitHub issues page](https://github.com/henrygd/beszel/issues)
- Contact developers on [the GitHub discussions page](https://github.com/henrygd/beszel/discussions) or the community in the Matrix room **#beszel:matrix.org**

## Support



## License

Beszel is licensed under the MIT License. See the [LICENSE](LICENSE) file for more details.
