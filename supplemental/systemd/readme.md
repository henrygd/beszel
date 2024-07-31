## Run the hub as a system service (Linux)

This runs the hub in the background continuously using systemd.

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

## Run the agent as a system service (Linux)

This runs the agent in the background continuously using systemd.

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
