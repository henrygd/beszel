# Package update apply (opt-in)

Upstream Beszel already detects pending package updates. This helper set enables **applying** them from the hub UI or automatically on a schedule.

## Security model

1. Keep the agent as a non-root user (`User=beszel`).
2. Install `beszel-pkg-apply` as the only privileged entrypoint.
3. Sudoers allows **only** that helper with fixed argv `security` or `all` (no raw `apt-get`/`dnf` from the agent).
4. The hub never accepts package names from the browser; the agent applies from its local pending cache.
5. Package names piped to the helper are validated (`[A-Za-z0-9._+:-]+`).

## Agent environment

| Variable | Purpose |
|----------|---------|
| `PACKAGE_UPDATES_APPLY=true` | Allow hub-triggered Apply buttons and auto-apply configuration |
| `PACKAGE_UPDATES_AUTO_APPLY=security\|all` | Optional automatic apply after checks (or set from the hub UI) |
| `PACKAGE_UPDATES_AUTO_APPLY_INTERVAL=24h` | Minimum time between automatic applies |

Manual Apply is fire-and-forget; the package list refreshes when the agent reports new counts. The hub UI **Auto-apply** dropdown (`all` / `security` / `off`) writes a per-host preference under the agent data directory.

## Install (systemd example)

```bash
install -d -m 0755 /usr/local/lib/beszel
install -m 0755 beszel-pkg-apply /usr/local/lib/beszel/beszel-pkg-apply
install -m 0440 beszel-agent-apply.sudoers /etc/sudoers.d/beszel-agent-apply
mkdir -p /etc/systemd/system/beszel-agent.service.d
install -m 0644 beszel-agent-apply.service.d.conf /etc/systemd/system/beszel-agent.service.d/apply.conf
systemctl daemon-reload
systemctl restart beszel-agent
```

Adjust the drop-in for your distro and sandboxing needs. Settings that imply `NoNewPrivileges` must allow passwordless `sudo` for the helper only.
