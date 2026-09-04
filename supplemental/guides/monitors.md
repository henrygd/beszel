# Uptime monitors

Beszel can check external services from the hub, so you can retire a separate
uptime tool for the common cases: **HTTP(S)** (+ keyword search), **TLS
certificate expiry**, **DNS records** and **ping (ICMP)**.

## Quick start

1. Open the **Monitors** page from the navbar and click **Add monitor**.
2. Pick a type, enter the target, keep the defaults (check every 60 s,
   10 s timeout, down after 3 consecutive failures).
3. Leave **Send notifications** on to reuse your existing email/webhook
   channels on down/recovery transitions.

Monitors run on the hub (no agent needed). Pause, test, mute or delete them
from their cards; open a monitor for latency graphs and check history.

## Monitor types

### HTTP(S) and keyword

Full URL target (`https://example.com/health`). Accepted status codes default
to `200-299` (Kuma syntax also allows lists and ranges like `200,201,204` or
`200-299,401`). Method, headers, body, basic/bearer auth, redirect following
(max 10, re-validated per hop) and `ignore TLS errors` are configurable.
`keyword` mode additionally requires (or forbids, when inverted) a text
snippet in the first 2 MB of the response body.

HTTPS checks also watch certificate expiry by default (warn below 21 days,
down below 7 days, configurable).

### TLS certificate

`host[:port]` target (default port 443, SNI = host). No HTTP request is made,
so it works for any TLS service. Same 21/7-day warn/critical thresholds.

### DNS

Hostname target, record type (A, AAAA, CNAME, MX, TXT, NS, SOA, SRV, CAA,
PTR), optional custom resolver (`IP` or `IP:port`, UDP or TCP) and optional
expected answer (`contains` or `exact` match). Success = NOERROR with at
least one record (matching when expected).

### Ping

Hostname or IP target, 3 packets of 56 bytes by default. Success = at least
one reply; min/avg/max and loss are recorded. Unprivileged mode is tried
first; in Docker add `--cap-add=NET_RAW` (or set
`net.ipv4.ping_group_range`) or checks report `missing NET_RAW capability`
instead of a silent false down. Where ICMP is filtered, prefer an HTTP
monitor.

## Notifications

Transitions (up → down, down → up, warning in/out) notify through your
existing channels (emails + shoutrrr webhooks) with quiet hours honored.
Steady down states renotify only if **Resend every** is set (minutes,
0 = never). Turn **Send notifications** off to keep history without noise.

## Declarative config

Monitors can be defined in `config.yml` (same file as systems). Entries match
by `(name, target)`, are created or updated at boot, and **never deleted**:
monitors created in the UI survive a sync that does not mention them.

```yaml
monitors:
  - name: homepage
    type: http
    target: https://example.com
    interval: 60
    timeout: 10
    users: [you@example.com]
    config:
      accepted_status_codes: 200-299
  - name: mail DNS
    type: dns
    target: example.com
    users: [you@example.com]
    config:
      qtype: MX
      resolver: 1.1.1.1
      expected_answer: mail.
  - name: gateway
    type: ping
    target: 192.168.1.1
    users: [you@example.com]
```

Secrets (`password`, `token`, sensitive headers) are never exported back into
`config.yml` and must be managed out of band.

## Security notes

By default the hub refuses to check private networks (loopback, RFC 1918,
link-local, cloud metadata). Set `MONITORS_ALLOW_PRIVATE_NETWORK=true` only
for lab use. Every redirect hop is re-validated and resolved IPs are pinned
per attempt (DNS-rebinding resistant). Concurrency is capped at 10
simultaneous checks (`MONITORS_MAX_CONCURRENT`, max 50).

## History and retention

Every attempt is stored (`monitor_checks`, 30-day retention, 500k-row safety
cap) with latency, status code and typed details. The 24h uptime ratio is
recomputed every 10 minutes. Manual **test** runs never pollute history.
