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

**Setting it up?** → **[INSTALL.md](INSTALL.md)** — exact commands, expected output, and
troubleshooting. Handing it to an AI agent? The whole prompt is *"Set up Relayent. Follow
INSTALL.md exactly."* — the guide starts by making the agent ask you for the details and
approvals it needs.

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

The relay is **deploy-anywhere**, not localhost-only: a laptop, a VPS, a container, or next to
your app. It only needs to be reachable by both the consuming app and the bridge, which are
decoupled from its location purely by `RELAYENT_RELAY_URL`. The quick start below uses
`localhost` for convenience; in production put the relay behind TLS. The bridge still needs no
inbound connectivity wherever the relay lives — it always dials out.

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
| `cursor` | ✅ | `cursor-agent -p --output-format json --mode ask --trust`, prompt as an argument |
| `gemini` | 🔜 stub | wire the Gemini CLI headless mode |

`cursor` runs in `--mode ask` (read-only Q&A) so a generation job can never edit files or run
shell commands. `--trust` is required for headless runs. The CLI has no schema flag, so the
adapter asks for JSON in-prompt and retries once to repair malformed output.

A backend is only usable when its CLI is **installed**, an adapter **supports** it, and both
hold — reported per backend as `installed` / `supported` / `ready` by
`GET /v1/bridge/capabilities`. A CLI on `PATH` alone does not make a backend `ready`.

Add a backend: implement `adapters.Adapter` in `bridge/adapters/`, register it in
`bridge/registry.go`. One file + one line.

## Install the bridge

On the machine whose AI subscription you want to use (needs a CLI installed **and signed in**):

```bash
./install.sh              # from a checkout — builds from source, pairs interactively
```

It installs to `~/.local/bin`, never uses `sudo`, and never writes outside `$HOME`. Then:

```bash
relayent-bridge setup      # pair with a relay (verifies before saving)
relayent-bridge install    # run at login, in the background
relayent-bridge monitor    # live status + logs in the terminal
relayent-bridge doctor     # diagnose anything that isn't working
```

Configuration works like `aws configure` — a wizard for first-time setup, then individual
settings without re-running it:

```bash
relayent-bridge config list                 # every setting, its value, and where it came from
relayent-bridge config set workspace ~/code # change one value (validated before it's saved)
relayent-bridge config unset workspace      # revert to the default
```

Values live in `~/.relayent/config.env` (owner-only, `0600`); environment variables override
the file, and `config list` shows which source is winning. The pairing key is masked — only
its fingerprint is shown, which matches the relay's status page.

Jobs run in a dedicated empty folder (`~/.relayent/workspace`) — never your personal files.
Point `workspace` somewhere else only if you deliberately want jobs to see it.

## Integrating an app

Everything an app needs is on the `/v1` API — no CLI, no SDK required. Three calls:

```bash
# 1. Fail fast: is anyone home? (Relayent never falls back to a paid API — that is the point)
curl -s $RELAY/v1/bridge/online -H "Authorization: Bearer $KEY"
# -> {"online":true}

# 2. What can it run, and with which models?
curl -s $RELAY/v1/bridge/capabilities -H "Authorization: Bearer $KEY"
# -> backends[] with ready, models[], default_model, models_probed

# 3. Run a job
ID=$(curl -s -XPOST $RELAY/v1/jobs -H "Authorization: Bearer $KEY" \
  -d '{"backend":"cursor","model":"auto","prompt":"...","json_schema":{...}}' | jq -r .job_id)
curl -s "$RELAY/v1/jobs/$ID?wait=1" -H "Authorization: Bearer $KEY"    # long-polls
curl -s -X DELETE "$RELAY/v1/jobs/$ID" -H "Authorization: Bearer $KEY"  # changed your mind
```

**Discovering models.** `models[]` lists what you can pass as `model`; `default_model` is what
runs if you omit it. **`models_probed` matters**: `true` means the list came from the CLI and is
accurate for that install; `false` means it is a static declaration that may drift — treat it as
advisory, not exhaustive. An empty list does not mean models are unsupported, only
undiscoverable (the CLI has no enumerate command) — a name you know still works.

**Cancelling.** `DELETE /v1/jobs/{id}` returns `was_status`, and it is the honest bit:
`pending` means the job was still queued and the work is genuinely prevented; `running` means a
bridge already claimed it — **the CLI cannot be stopped**, because the relay has no channel to
an outbound-only bridge, so the quota is already spent and all you have done is stop waiting.

The [OpenAPI contract](openapi.yaml) is the source of truth. `clients/python/` is optional
convenience, not a dependency — any HTTP client works.

## Checking status

Two views, both live.

**The relay's web dashboard — open the relay's URL in a browser:**

```
https://your-relay.example.com        (or http://localhost:8787 for a local relay)
```

It asks for your pairing key, then shows relay health and uptime, whether a bridge is online,
pending jobs, and each backend's readiness. It auto-refreshes every 5s. A **Security** card
grades the deployment honestly — if it says *"NO — traffic is in the clear"*, the relay is
network-reachable without TLS and you should fix that before using it. The key stays in the
page and is only ever sent to the relay's own `/v1` API.

The page shows your key's **fingerprint** (an 8-char hash, never the key). It matches what
`relayent-bridge config list` prints — that is how you confirm both sides hold the same key.

**The bridge's terminal dashboard — on the machine running the bridge:**

```bash
relayent-bridge monitor      # Ctrl-C to quit
```

Connection state, bridge polling status, backend readiness, and a live tail of recent jobs,
refreshed every 2s. Colour is disabled automatically when piped. It never prints the key, so
it is safe to screenshot when asking for help.

**Everything else:**

```bash
relayent-bridge status                     # is the login service running?
relayent-bridge doctor                     # diagnose config, connectivity, backends
tail -f ~/.relayent/logs/bridge.err.log    # raw job activity
```

⚠️ Job activity is in `bridge.err.log`, **not** `bridge.out.log` — Go's logger writes to
stderr. `bridge.out.log` stays empty; it is not a sign anything is broken.

From any machine with the key, the same data is available over the API:

```bash
curl -s $RELAY/v1/status              -H "Authorization: Bearer $KEY"   # health, tls, key fingerprint
curl -s $RELAY/v1/bridge/online       -H "Authorization: Bearer $KEY"   # {"online":true}
curl -s $RELAY/v1/bridge/capabilities -H "Authorization: Bearer $KEY"   # which backends are ready
```

Service logs go to `~/.relayent/logs/` and are rotated at 5 MiB, keeping 3 generations —
neither launchd nor systemd rotates a service's log, so the bridge does it itself.

## Deploy the relay

The relay is deploy-anywhere. For a public one, use the bundled stack — Caddy fetches and
renews a free Let's Encrypt certificate automatically:

```bash
cd deploy/
cp .env.example .env       # set RELAYENT_DOMAIN, RELAYENT_PAIRING_KEY, ACME_EMAIL
docker compose run --rm relay keygen   # generate a strong key
docker compose up -d
```

The relay container is never published to the host — only Caddy's `443` is exposed, so TLS
cannot be bypassed.

## Multi-user (multi-tenant)

By default one shared pairing key means one shared subscription — whoever holds the key spends
that one account's quota. For **many users, each on their own subscription, isolated from one
another**, run the relay multi-tenant: set `RELAYENT_DATA_DIR` (a persistent volume) and,
normally, OIDC. Then:

- **Humans sign in at `/login`** — "Sign in with Google" (or your OIDC provider), or a bootstrap
  admin token. The **first person to sign in becomes the admin**; the rest are regular users.
  After login you're routed by role: admins to the **`/admin` console**, users to their own
  status page.
- **The `/admin` console** manages users (create, promote/demote, disable, delete), enrolls
  bridges, issues and revokes app credentials, and shows per-user activity and an audit log —
  **never any prompt or result content**.
- **Each user runs their own bridge**, enrolled with a one-time token, bound to only their jobs.
- **Apps** authenticate with an issued **app credential** and name `target_user` per job; a job
  for `alice` runs only on alice's bridge/subscription.

Enterprise SSO (Azure AD, Okta, any OIDC issuer) is a change of issuer URL — same protocol. The
relay stores **no passwords** (OIDC) and only **hashed** machine credentials plus a non-secret
user directory. Full setup is in [INSTALL.md](INSTALL.md#multi-user-multi-tenant-mode); the
changed threat model is in [SECURITY.md](SECURITY.md#multi-tenant-model-the-changed-threat-model).

## Security

**The pairing key is the only thing between the internet and your CLI subscription.** Anyone
holding it can send prompts to your machine and spend your quota. Treat it like a password.

- A network-reachable relay **refuses to start** without a key of ≥24 chars — generate one with
  `relayent-relay keygen`, don't invent one.
- Rotate without downtime: `relayent-relay rotate` prints the two-phase procedure. Multiple
  keys are valid at once (`RELAYENT_PAIRING_KEY=new,old`) so bridges migrate before you drop
  the old one. Bring your own key — `keygen` is only a convenience.
- The bridge dials **out** only: no ports open on the user's machine, no tunnel, no inbound.
- No credentials pass through Relayent — the CLI uses its own subscription session.
- Keys are never logged; an 8-char fingerprint is shown instead. Failed auth is rate-limited
  and key comparison is constant-time.
- The bridge refuses a remote `http://` relay outright — the key would cross the wire in
  cleartext.

**Read [SECURITY.md](SECURITY.md) before deploying a public relay.** It documents the full
threat model, including — importantly — [what Relayent does *not*
protect against](SECURITY.md#what-relayent-does-not-protect-against).

## Documentation

| Document | What it covers |
|---|---|
| **[API.md](API.md)** | Integrating an app: every call, what the numbers mean, the traps, a runnable client, a verification checklist, and the admin / multi-tenant surface |
| **[INSTALL.md](INSTALL.md)** | Setup, start to finish: bridge, relay (localhost / private / public+TLS), verification, configuration, rotation, troubleshooting |
| **[SECURITY.md](SECURITY.md)** | Threat model, the deploy guide, and **what Relayent does not protect against** |
| **[AGENTS.md](AGENTS.md)** | Conventions and security invariants for AI agents working on this codebase |
| **[openapi.yaml](openapi.yaml)** | The `/v1` contract — the only integration surface |

## License

MIT — see [LICENSE](LICENSE).
