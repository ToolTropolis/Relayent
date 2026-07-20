# Relayent Demo

A single public chat page backed by Relayent. The model dropdown is populated from the relay's
capabilities API; each message runs as a Relayent job on a user's local CLI subscription. There is
**no API key behind it** — that's the point.

It's a thin proxy: the browser talks only to the demo server's `/api/*`, which forwards to the
relay with the app credential attached **server-side**. The credential and relay URL never reach
the browser.

## What a visitor can use — you decide, centrally

The demo only offers the backends the **relay** is configured to expose. Control that in the admin
console under **Backends** (or `POST /v1/admin/backends/{name}`). For a public demo, keep paid
backends (`claude`, `codex`) **disabled** so visitors can only spend what you're happy to — e.g.
`cursor`. A disabled backend is hidden from the dropdown *and* refused at enqueue, so it can't be
reached by editing requests either.

## Run it

```bash
# 1. Issue an app credential (admin console -> App credentials, or the API):
#    curl -sXPOST $RELAY/v1/admin/app-creds -H "Authorization: Bearer $ADMIN" -d '{"app_id":"demo"}'
# 2. Configure:
cp .env.example .env        # set DEMO_APP_CREDENTIAL and DEMO_TARGET_USER
# 3. Build (from the repo root — shared go.mod) and run:
docker build -f demo/Dockerfile -t relayent-demo:latest .
docker compose -f demo/docker-compose.yml up -d
```

Point your reverse proxy at `http://relayent-demo:8080` (the demo joins the shared
`relayent-network`; the proxy's service must too). It exposes no host port.

## Configuration

| Variable | Required | Meaning |
|---|---|---|
| `DEMO_RELAY_URL` | yes | Relay base URL (container name on the shared network) |
| `DEMO_APP_CREDENTIAL` | yes | App credential `<id>.<secret>` (server-side only) |
| `DEMO_TARGET_USER` | yes | Whose bridge/subscription runs the jobs; must have a bridge online |
| `DEMO_DEFAULT_BACKEND` | no | Pre-selected backend (default `cursor`) |
| `DEMO_TITLE` | no | Page title/heading |
| `DEMO_TRUST_PROXY` | no | `1` when behind a trusted proxy, so the visitor IP is read from `X-Forwarded-For` (analytics) |

## Visitor analytics (admin-only)

Each page view is reported to the relay as a **content-free** hit: the demo parses a coarse
device/browser/OS family from the User-Agent, the referrer **host** (never the full URL), and any
`utm_*` params, and sends them with the visitor IP. The relay turns the IP into a country (offline
GeoIP) plus a daily-rotating hash and **discards it** — no raw IP or URL is ever stored. Obvious
bots are skipped. The report is fire-and-forget: it never blocks or breaks the chat page.

The admin sees the rollup — visits, unique visitors, per-day trend, and top countries / referrers /
campaigns / devices / browsers / OSes — in the relay admin console under **Demo visitors**. There
are no per-visitor rows, by design (same privacy posture as the audit log).

To enable it:

1. Issue the demo's app credential **with the `demo-stats` scope** (see `.env.example`).
2. For accurate countries, point the relay at a GeoLite2 database via `RELAYENT_GEOIP_DB`. Without
   it, everything still works and countries show as `??`.

## Offline behavior

If no bridge is online for the target user, the header shows **no bridge online** and Send is
disabled. If a backend is disabled or a job fails, the relay's error is shown in the chat. The demo
never falls back to a paid API.
