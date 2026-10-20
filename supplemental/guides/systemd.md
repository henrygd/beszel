# Installing as a Linux systemd service

This is useful if you want to run the hub or agent in the background continuously, including after a reboot.

## Install script (recommended)

There are two scripts, one for the hub and one for the agent. You can run either one, or both.

The install script creates a dedicated user for the service (`beszel`), downloads the latest release, and installs the service.

If you need to edit the service -- for instance, to change an environment variable -- you can edit the file(s) in `/etc/systemd/system/`. Then reload the systemd daemon and restart the service.

> [!NOTE]
> You need system administrator privileges to run the install script. If you encounter a problem, please [open an issue](https://github.com/henrygd/beszel/issues/new).

### Hub

Download the script:

```bash
curl -sL https://raw.githubusercontent.com/henrygd/beszel/main/supplemental/scripts/install-hub.sh -o install-hub.sh && chmod +x install-hub.sh
```

#### Install

You may specify a port number with the `-p` flag. The default port is `8090`.

```bash
./install-hub.sh
```

#### Uninstall

```bash
./install-hub.sh -u
```

#### Update

```bash
sudo /opt/beszel/beszel update && sudo systemctl restart beszel-hub
```

### Agent

Download the script:

```bash
curl -sL https://raw.githubusercontent.com/henrygd/beszel/main/supplemental/scripts/install-agent.sh -o install-agent.sh && chmod +x install-agent.sh
```

#### Install

You may optionally include the SSH key and port as arguments. Run `./install-agent.sh -h` for more info.

If specifying your key with `-k`, please make sure to enclose it in quotes.

```bash
./install-agent.sh
```

#### Uninstall

```bash
./install-agent.sh -u
```

#### Update

```bash
sudo /opt/beszel-agent/beszel-agent update && sudo systemctl restart beszel-agent
```

## Manual install

### Hub

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

### Agent

1. Create the system service at `/etc/systemd/system/beszel-agent.service`

```bash
[Unit]
Description=Beszel Agent Service
After=network.target

[Service]
# update the values in curly braces below (remove the braces)
Environment="PORT={PASTE_YOUR_PORT_HERE}"
Environment="KEY={PASTE_YOUR_KEY_HERE}"
# Environment="EXTRA_FILESYSTEMS={sdb}"
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

## NUT UPS/PDU monitoring

The agent can collect UPS and PDU data from Network UPS Tools (NUT). Install
the `upsc` client on the monitored system. The agent uses the standard text
output, including on older clients such as NUT 2.8.0 and 2.8.1; JSON support
is not required. Add the optional settings below to the agent service, then
reload systemd and restart the agent.

```ini
# Defaults to localhost when omitted.
Environment="NUT_SERVER=localhost"
# A Go duration; for example, refresh every 60 seconds.
Environment="NUT_INTERVAL=60s"
# Optional comma-separated UPS names. When omitted, the agent discovers them.
# Environment="NUT_DEVICES=ups1,ups2"
# Optional NUT nutauth.conf file, readable by the agent service user.
# Environment="NUT_AUTHCONF_FILE=/etc/nut/nutauth.conf"
```

`NUT_AUTHCONF_FILE` is passed to `upsc -A`, so it must use NUT's
`nutauth.conf` format and should be protected with mode 0600. It is not an
`upscmd.conf` file.

The optional auth file requires a client that supports `-A`; this is checked
separately at startup. Leave it unset for ordinary read-only queries on older
clients. The NUT server can run on another host.

Power uses `ups.realpower` or `output.realpower` in watts. If neither is
available, the agent estimates watts from `ups.load * ups.realpower.nominal / 100`
and labels the result as estimated. Accuracy depends on how the driver reports
load. `ups.power` is apparent power (VA), and `output.power` has inconsistent
units across drivers, so neither is used as watts. Missing power is displayed as N/A; an actual zero is displayed
as 0 W. Legacy samples without a power source are omitted from power charts.

Notifications include a return to ONLINE from a known unhealthy state. Initial
observations, unknown states, and unchanged states do not produce recovery
notifications. Detection follows `NUT_INTERVAL` (default: one hour); choose a
shorter interval such as 60s to detect shorter outages. These notifications do
not provide outage counts or durations.
