# Relayent

**Use the AI subscription you already pay for — from anywhere.**

Relayent lets any app (even one deployed on a remote server) route its AI calls to a
**locally-running CLI subscription** on a user's machine — Claude Code, Codex, Gemini, or
Cursor — instead of a paid API key. No inbound ports, no tunnels: the user's machine dials
**out** to a relay and pulls jobs.

```
 ┌──────────────────────┐      ┌───────────────────┐      ┌────────────────────────┐
 │ Your app (anywhere)  │─POST▶│  Relayent Relay   │◀poll─│ Relayent Bridge (Mac)  │
 │  enqueue AI job      │      │  job broker /v1   │      │  runs claude -p / codex │
 │  GET result ────────▶│◀────▶│                   │◀post─│  (your subscription)    │
 └──────────────────────┘      └───────────────────┘      └────────────────────────┘
```

## Why

If you (or your users) already pay for Claude Code / Codex / Gemini / Cursor, you shouldn't
have to buy API tokens to use those same models inside another app. Relayent reuses the CLI's
own authenticated session — it never stores or handles credentials.

## Components

| Component | What | Runs where |
|---|---|---|
| **Relay** (`relay/`, Go) | A small job broker exposing the `/v1` HTTP API. | A server both sides can reach. |
| **Bridge** (`bridge/`, Go) | A daemon that dials out to the relay, runs the local CLI headless, returns results. | The user's own machine. |
| **API** (`openapi.yaml`) | The versioned `/v1` contract — the **only** integration surface. | — |
| **Client** (`clients/python/`) | Optional Python convenience over the API. | Inside the consuming app. |

## Quick start (local)

```bash
# 1. Build
make all                          # -> bin/relayent-relay, bin/relayent-bridge

# 2. Run the relay
RELAYENT_PAIRING_KEY=devkey RELAYENT_LISTEN=:8787 ./bin/relayent-relay

# 3. Run the bridge on your machine (uses your logged-in CLI subscription)
RELAYENT_RELAY_URL=http://localhost:8787 RELAYENT_PAIRING_KEY=devkey ./bin/relayent-bridge

# 4. Enqueue a job from any app (raw HTTP)
curl -s -XPOST localhost:8787/v1/jobs -H 'Authorization: Bearer devkey' \
  -d '{"backend":"claude","prompt":"Return {\"ok\":true} as JSON",
       "json_schema":{"type":"object","properties":{"ok":{"type":"boolean"}}}}'
# -> {"job_id":"..."}

curl -s 'localhost:8787/v1/jobs/<id>?wait=1' -H 'Authorization: Bearer devkey'
# -> {"status":"done","json":{"ok":true}}
```

## Integrating a new app

You only depend on the `/v1` HTTP API (see [`openapi.yaml`](openapi.yaml)). Two options:

**Raw HTTP** — POST `/v1/jobs`, poll `GET /v1/jobs/{id}?wait=1`. Works in any language.

**Python reference client:**
```python
from relayent_client import RelayentClient, BridgeOfflineError

client = RelayentClient("https://relay.example.com", pairing_key="...")
try:
    data = client.run(backend="claude", prompt="...", json_schema={...})
except BridgeOfflineError:
    ...  # fail fast — do NOT silently fall back to a paid API unless you mean to
```

## Configuration

**Relay** (env): `RELAYENT_LISTEN` (default `:8787`), `RELAYENT_PAIRING_KEY` (if set, only
this key is accepted; otherwise any non-empty key gets its own isolated namespace).

**Bridge** (env): `RELAYENT_RELAY_URL` (required), `RELAYENT_PAIRING_KEY` (required),
`RELAYENT_POLL_WAIT` (s), `RELAYENT_JOB_TIMEOUT` (s), and per-backend binary overrides
`RELAYENT_CLAUDE_BIN` / `RELAYENT_CODEX_BIN` / `RELAYENT_GEMINI_BIN` / `RELAYENT_CURSOR_BIN`.

## Backends

| Backend | Status | How it runs |
|---|---|---|
| `claude` | ✅ | `claude -p --output-format json [--json-schema] [--model]`, prompt on stdin |
| `codex` | ✅ | `codex exec -`, prompt on stdin |
| `gemini` | 🔜 stub | wire the Gemini CLI headless mode |
| `cursor` | 🔜 stub | wire the Cursor agent headless mode |

Add a backend: implement `adapters.Adapter` in `bridge/adapters/`, register it in
`bridge/registry.go`. One file + one line.

## Security notes

- Every request needs a bearer **pairing key**; jobs are scoped to it, so one user's jobs are
  only ever claimed by their own bridge.
- The bridge makes **outbound** connections only — nothing listens on the user's machine.
- No credentials pass through Relayent — the CLI uses its own subscription session.
- Run the relay behind TLS in production and treat the pairing key as a secret.

## License

MIT — see [LICENSE](LICENSE).
