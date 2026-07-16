# Relayent — Session Handoff

**Date:** 2026-07-16
**Context:** Built from scratch this session. Continue from "Next steps" below.

---

## What Relayent is

**Use the AI subscription you already pay for — from anywhere.**

Relayent lets any app (even one deployed on a remote server) route its AI calls to a
**locally-running CLI subscription** on a user's machine — Claude Code, Codex, Cursor,
(Gemini pending) — instead of a paid API key.

**Origin:** EngageHub's AI features need paid API keys. Users already paying for a Claude
Code / Codex / Cursor subscription shouldn't pay twice. The blocker: a public server can't
reach a CLI on a laptop behind NAT. Solution: the laptop **dials out** to a relay and pulls jobs.

> The Specter project (`../Specter`) was the inspiration, but its checkout still uses the paid
> `@anthropic-ai/sdk` — the bridge mechanism did **not** exist. This is all new work.

```
 ┌──────────────────────┐      ┌───────────────────┐      ┌────────────────────────┐
 │ Your app (anywhere)  │─POST▶│  Relayent Relay   │◀poll─│ Relayent Bridge (Mac)  │
 │  enqueue AI job      │      │  job broker /v1   │      │  runs claude -p / codex │
 │  GET result ────────▶│◀────▶│                   │◀post─│  (your subscription)    │
 └──────────────────────┘      └───────────────────┘      └────────────────────────┘
```

---

## Locked-in decisions (do not re-litigate)

| Decision | Choice | Why |
|---|---|---|
| **Name** | Relayent (relay + agent) | — |
| **Stack** | **All Go** (relay + bridge) | Single static binaries, cross-compile, great at subprocess+HTTP |
| **Repo** | **Standalone** at `ToolTropolis/Relayent/`, own git history | Must be reusable beyond EngageHub |
| **Integration surface** | **`/v1` HTTP API only** (OpenAPI = source of truth) | Consumer language is irrelevant; clients are optional sugar |
| **Connection** | Bridge **dials out** (no inbound ports, no tunnel) | Works behind NAT/firewall; zero attack surface on the laptop |
| **Deployment** | Relay deployable **anywhere** (not localhost-only) | Decoupled purely by `RELAYENT_RELAY_URL` |
| **Offline behavior** | **Fail fast** — never silently fall back to a paid API | Guarantees only the subscription is billed |
| **EngageHub routing** | New selectable provider `relayent` | Additive; existing provider ladders untouched |

---

## Status: what's DONE and VERIFIED

### ✅ Committed — `b472e84` "feat: Relayent MVP"
- **Relay** (`relay/`, Go): `/v1/jobs` (enqueue), `/v1/jobs/next` (long-poll claim),
  `/v1/jobs/{id}/result`, `/v1/jobs/{id}` (fetch, `?wait=1` blocks), `/v1/health`,
  `/v1/bridge/online`. Bearer pairing-key auth, per-key job scoping, in-memory queue + TTL
  janitor. Unit tests (`relay/queue_test.go`) + distroless `Dockerfile`.
- **Bridge** (`bridge/`, Go): dial-out long-poll loop, pluggable adapter registry, per-job
  timeout, graceful shutdown. **No credential storage** — shells out to the already-authenticated CLI.
- **`openapi.yaml`** — the versioned, language-neutral contract.
- **`clients/python/relayent_client.py`** — reference client (`run()`, `BridgeOfflineError`).
- README, Makefile (incl. `cross` targets), LICENSE (MIT), .gitignore.

**Verified live:** enqueue → bridge claim → `claude -p` on the **subscription**
(`ANTHROPIC_API_KEY`/`OPENAI_API_KEY` unset) → structured JSON
`{"city":"Paris","country":"France","is_capital":true}` in ~5.7s. Reliable across 3
consecutive runs. Fail-fast confirmed: bridge killed → `online:false` → client raises
`BridgeOfflineError`, no API fallback. `go test ./...` passes.

### ✅ Built & verified, **NOT YET COMMITTED** — the status interface
- **`GET /v1/status`** → `{status, version, uptime_seconds, bridge_online, pending_jobs, require_pairing}`
- **`GET /v1/bridge/capabilities`** → `{online, reported_at, capabilities:{version, hostname, backends[]}}`
- **`POST /v1/bridge/capabilities`** → bridge self-reports (relay can't see the user's machine)
- **`GET /`** → self-contained HTML status page (`relay/statuspage.go`). Asks for the pairing
  key in-browser, calls the same `/v1` API, auto-refreshes 5s, dark/light aware.
  **Browser-verified:** live data, **zero JS errors**, correct values, 404s unknown paths.
- **Honest capability model** — `BackendInfo{Installed, Supported, Ready}` distinguishes
  *CLI present* vs *adapter implemented* vs *actually usable*. Bridge reports capabilities on
  startup + every 60s (`capabilitiesLoop`).
- Error messages match the model (`unavailableReason` in `bridge/main.go`):
  - stub w/ CLI present → `backend "cursor" is not supported yet by this bridge (adapter is a stub)`
  - stub w/o CLI → `... not supported yet and its CLI is not installed on this machine`

Verified output before the Cursor work:
```json
{"online":true,"capabilities":{"hostname":"Navjyots-MacBook-Pro.local","backends":[
  {"name":"claude","installed":true,"supported":true,"ready":true},
  {"name":"codex","installed":true,"supported":true,"ready":true},
  {"name":"cursor","installed":true,"supported":false,"ready":false},
  {"name":"gemini","installed":false,"supported":false,"ready":false}]}}
```

### ✅ Committed — `945765a` "feat: status interface + real Cursor adapter"
The status interface (above) and the Cursor adapter are now built, verified and committed.
`go build` / `go vet` / `go test ./...` all pass — the adapter compiled as written, no fixups
were needed. The Cursor stub is gone from `stubs.go` (Gemini stub remains).

**Cursor verified live:** `{"backend":"cursor"}` + `json_schema` → clean structured JSON in
~5s, on the subscription with `ANTHROPIC_API_KEY`/`OPENAI_API_KEY` unset. 3/3 clean across two
different schemas; bridge log confirms `backend=cursor`. `/v1/bridge/capabilities` now reports
cursor `installed:true, supported:true, ready:true`.

Docs updated in the same commit: README backends table + deploy-anywhere note; `openapi.yaml`
gained `/v1/status`, `/v1/bridge/capabilities` and the four new schemas — each field diffed
against the live API responses (only `model` is spec-only, correctly, since it is `omitempty`).

**Cursor research (all confirmed live):**
- `cursor-agent` is at `~/.local/bin/cursor-agent` and **authenticated**:
  `cursor-agent status` → `✓ Logged in as navjyotnishant@gmail.com` (subscription, no API key)
- Headless works and returns **the same envelope shape as Claude**:
  ```bash
  cursor-agent -p --output-format json --mode ask --trust "Reply with only the word PONG"
  # -> {"type":"result","subtype":"success","is_error":false,"result":"PONG",...}   (~2.6s)
  ```
- **`--trust` is REQUIRED** headlessly (else it errors "Workspace Trust Required").
- **`--mode ask`** chosen deliberately: read-only Q&A, so generation jobs can never edit files
  or run shell commands. Prompt is an **argument**, not stdin (unlike claude/codex).
- No `--json-schema` flag → adapter instructs JSON in-prompt + does one JSON-repair retry.

---

## Security hardening + installer (done, pushed)

Commits `1b619ad`, `3ee8c5a`, `80013a9`, `4af9b5c`, `21ff148`.

**Relay auth fails closed.** A network-reachable relay **refuses to start** without a
pairing key of ≥24 chars (`validateKeySetPolicy`); loopback still allows weak/absent keys for
dev. `RELAYENT_ALLOW_INSECURE=1` is the explicit opt-out. Plus: constant-time comparison,
per-IP limits on failed auth (8 burst → 429), per-key limits on enqueue, explicit server
timeouts, security headers, and keys never logged (8-char SHA-256 fingerprint instead).

**Key rotation + BYOK.** `RELAYENT_PAIRING_KEY` takes a comma-separated list — first primary,
rest retiring — so keys rotate with no downtime. `relayent-relay rotate` prints the two-phase
procedure; `keygen` emits a 256-bit key. `/v1/status` exposes `key_fingerprint`,
`key_retiring`, `rotation_active`, `tls`, `network_reachable`.

**Bridge installs like a normal app.** `install.sh` (builds from source, never sudo, never
writes outside `$HOME`) → `relayent-bridge setup|install|uninstall|status|doctor`. Setup
verifies the relay before saving to `~/.relayent/config.env` (0600). `install` registers a
launchd agent (macOS) / systemd --user unit (Linux) — starts at login, restarts on failure,
never root. The plist pins PATH; without it launchd's minimal PATH hides the CLIs.

**Jobs run in a sandbox, not `$HOME`.** ⚠️ *This was a real bug:* no adapter set `cmd.Dir`,
and the plist pinned `WorkingDirectory` to `$HOME`, so CLIs inherited it and macOS attributed
their file access to the bridge → prompts for Desktop/Documents/Downloads. Jobs now run in
`~/.relayent/workspace` (`Request.WorkDir`, threaded through every adapter).
`RELAYENT_WORKSPACE` overrides. Home stays *readable* on purpose — the CLIs must load their
own sessions from `~/.claude`, `~/.codex`, `~/.cursor`.

**Stored XSS fixed (HIGH).** ⚠️ Found by `/security-review`, confirmed exploitable: a payload
in `BackendInfo.Name` (via `POST /v1/bridge/capabilities`) reached the status page's
`innerHTML` and read the operator's pairing key out of the DOM. Fixed in three layers —
`createElement`/`textContent` at the sink, `sanitizeCapabilities()` filtering to the known
backend set at the source, and a per-request CSP nonce replacing `script-src 'unsafe-inline'`
(which had *authorised* the injected `onerror=`). Don't remove any one layer assuming another
holds.

**Docs:** `SECURITY.md` (threat model incl. an explicit "what this does NOT protect against"),
`deploy/` (docker-compose + Caddy, free automatic Let's Encrypt TLS).

## Decisions made this session (do not re-litigate)

| Decision | Choice | Why |
|---|---|---|
| **Desktop GUI** | **No** — CLI + web status page only | A native app means a 2nd language, Apple Developer account (~$99/yr), notarization, and a GUI process that itself trips macOS TCC prompts. All to expose a setting nothing currently reads. Revisit only if jobs become file-aware. |
| **Workspace config** | Env var (`RELAYENT_WORKSPACE`) | Jobs are read-only Q&A and never read user files; the empty sandbox is the correct default, not a limitation. A picker is only useful once file-aware jobs exist. |
| **"More directories"** | Not applicable | A subprocess has exactly one cwd. The choice is *which* directory, not a set of them. |
| **Encryption** | TLS + strong keys, no bespoke crypto | Nothing is stored at rest; the exposure is auth, not cipher strength. Boring standard primitives. |
| **Repo** | Private, `ToolTropolis/Relayent` | Pushed 2026-07-16. |

## Next steps (in order)

1. **Re-run `/security-review`** before any public exposure. It caught a HIGH XSS that a
   hand-written threat model had wrongly claimed was covered — worth repeating.
2. **Verify the TLS deploy on the real server.** ⚠️ **Never tested end-to-end** — Let's Encrypt
   issuance needs a real domain, public DNS, and reachable ports 80/443. Locally verified only:
   compose config parses, and it refuses to start without a key.
3. **Linear follow-ups** — note the status interface, Cursor adapter, and security work on
   ENG-82.
4. **Then:** EngageHub integration (ENG-83) — see below.

### Optional / later
- Real **Gemini** adapter (CLI not installed on this machine).
- Signed/notarized releases + published checksums (`install.sh` currently prefers building
  from source precisely because that is the verifiable path).
- Redis queue backend for a multi-instance relay.
- Streaming protocol extension; MVP is request/response only.

---

## How to run locally

```bash
export PATH="/opt/homebrew/bin:$PATH"
make all                    # -> bin/relayent-relay, bin/relayent-bridge

# Terminal 1 — relay
RELAYENT_PAIRING_KEY=devkey RELAYENT_LISTEN=:8787 ./bin/relayent-relay

# Terminal 2 — bridge (unset API keys to PROVE subscription use)
env -u ANTHROPIC_API_KEY -u OPENAI_API_KEY \
  RELAYENT_RELAY_URL=http://localhost:8787 RELAYENT_PAIRING_KEY=devkey \
  PATH="/opt/homebrew/bin:$HOME/.local/bin:$PATH" ./bin/relayent-bridge

# Terminal 3 — drive it
curl -s localhost:8787/v1/status -H 'Authorization: Bearer devkey'
curl -s localhost:8787/v1/bridge/capabilities -H 'Authorization: Bearer devkey'
ID=$(curl -s -XPOST localhost:8787/v1/jobs -H 'Authorization: Bearer devkey' \
  -d '{"backend":"claude","prompt":"Reply with JSON about the number 5.",
       "json_schema":{"type":"object","properties":{"value":{"type":"integer"}},"required":["value"]}}' \
  | sed -n 's/.*"job_id":"\([^"]*\)".*/\1/p')
curl -s "localhost:8787/v1/jobs/$ID?wait=1" -H 'Authorization: Bearer devkey'
# -> {"status":"done","json":{"value":5}}
```
Status page: open <http://localhost:8787/> and enter `devkey`.

**Env vars** — Relay: `RELAYENT_LISTEN` (default `:8787`), `RELAYENT_PAIRING_KEY` (if set, only
that key is accepted; else any non-empty key gets its own namespace).
Bridge: `RELAYENT_RELAY_URL` + `RELAYENT_PAIRING_KEY` (both required), `RELAYENT_POLL_WAIT`,
`RELAYENT_JOB_TIMEOUT`, `RELAYENT_{CLAUDE,CODEX,GEMINI,CURSOR}_BIN`.

---

## Layout

```
Relayent/
├── internal/api/types.go        # shared /v1 wire types (EnqueueRequest, Job, BackendInfo, StatusResponse…)
├── relay/
│   ├── main.go                  # HTTP handlers + auth middleware + routes + keygen/rotate subcommands
│   ├── security.go              # 🔒 KeySet (rotation), validateKeyPolicy (fail-closed), constant-time
│   │                            #    compare, limiter, sanitizeCapabilities, securityHeaders, usage
│   ├── queue.go                 # per-key in-memory broker: enqueue/claim/result/fetch/caps/TTL
│   ├── statuspage.go            # self-contained HTML dashboard + per-request CSP nonce
│   ├── security_test.go         # key policy, rotation, limiter, XSS sanitiser regressions
│   ├── queue_test.go            # round-trip, key scoping, presence, long-poll blocking
│   └── Dockerfile
├── bridge/
│   ├── main.go                  # subcommands + dial-out poll loop, process(), reportCapabilities
│   ├── config.go                # file+env config, validateRelayURL (no remote http://), workspace
│   ├── setup.go                 # 🆕 setup wizard, doctor, config write (0600), key fingerprint
│   ├── service.go               # 🆕 install/uninstall/status — launchd plist / systemd unit
│   ├── registry.go              # backend registry + Describe() (Installed/Supported/Ready)
│   ├── config_test.go           # relay URL policy, workspace-is-never-$HOME
│   └── adapters/
│       ├── adapter.go           # Adapter interface, Request{...WorkDir} / Result
│       ├── claude.go            # ✅ claude -p --output-format json --json-schema (INLINE)
│       ├── codex.go             # ✅ codex exec -   (prompt on stdin)
│       ├── cursor.go            # ✅ cursor-agent -p --output-format json --mode ask --trust
│       ├── stubs.go             # gemini stub only (Available()=false, BinPresent())
│       └── util.go              # parseJSON / stripFences
├── deploy/                      # 🆕 docker-compose.yml + Caddyfile + .env.example (auto TLS)
├── install.sh                   # 🆕 one-command bridge installer
├── SECURITY.md                  # 🆕 threat model + deploy guide + what it does NOT protect
├── clients/python/relayent_client.py
├── openapi.yaml                 # ✅ full /v1 contract incl. /v1/status + capabilities
├── Makefile  README.md  LICENSE  .gitignore  go.mod   (module: github.com/navjyotnishant/relayent)
```

**Adding a backend:** implement `adapters.Adapter` in `bridge/adapters/`, add one line to
`NewRegistry()` in `bridge/registry.go`. Stub adapters return `Available()=false` and implement
`BinPresent()` so the UI can distinguish "CLI missing" from "not supported yet".

---

## Gotchas discovered (hard-won — don't rediscover)

1. **`claude --json-schema` takes an INLINE JSON string, not a file path.** Passing a temp-file
   path makes the process **hang forever** (cost ~90s of a mystery "pending" job). Fixed.
2. **Structured output is unreliable across CLI versions.** `--json-schema` alone did *not*
   reliably shape `result` (got prose "7 is prime." instead of JSON). Fix that worked: forceful
   in-prompt directive + echo the schema + **one JSON-repair retry**. Now 3/3 clean dicts.
3. **`cursor-agent` needs `--trust`** headlessly, and takes the prompt as an **argument** (not stdin).
4. **Don't equate "CLI on PATH" with "backend usable."** `cursor-agent` existed while the adapter
   was a stub → status page falsely showed "installed: yes". Hence `Installed`/`Supported`/`Ready`.
5. **gopls shows false errors** (`undefined: Queue`, "not in your workspace") because Relayent is
   outside the IDE workspace. `go build` at the repo root is the source of truth. Opening Relayent
   as its own workspace fixes this (and the permission prompts).
6. **macOS has no `timeout`** — use `perl -e 'alarm N; exec @ARGV' ...` when capping a command.
7. **`openapi.yaml` uses YAML flow mappings** (`{ type: string, description: ... }`). An
   unquoted `?` or `,` inside a description breaks the parse — quote any description
   containing them. System python3 has no pyyaml and is externally-managed (PEP 668); use a
   throwaway venv to validate rather than `pip install --break-system-packages`.
8. Relay `Version`/bridge `Version` are `var`s, overridable at link time via ldflags.

---

## EngageHub integration (separate, NOT started) — ENG-83

**Do not couple.** EngageHub consumes Relayent **only** via the `/v1` API / reference client.

- `utils/relayent.py` — thin adapter: read settings → call client → **fail-fast** when offline.
- `AppSettings` columns + idempotent migration in `utils/db_init.py` (`ADD COLUMN IF NOT EXISTS`):
  `relayent_url`, `relayent_pairing_key_enc` (Fernet via `utils/crypto`), `relayent_backend`, `relayent_model`.
- Extend `get_effective_settings` key_map (`utils/settings_helper.py`) so the pairing key is
  user-overridable (each user pairs their own laptop).
- Add a `provider == 'relayent'` branch at **5 dispatch sites** (EngageHub has **no** shared AI
  wrapper — the ladder is copy-pasted):
  1. `routes/strategic_account_plan.py::_call_ai_json` (~L362) — **lights up all 5 AI-populate/suggest features**
  2. `routes/ai_chat.py` — 4 dispatch blocks (~L289, L572, L681, L757)
  3. `routes/admin.py::enrich_account` (~L402)
  4. `services/rfp_service.py::generate_with_provider` (~L521) + `evaluate_question_score_with_provider` (~L579)
  5. `agents/market_research/agents.py::get_llm_response` (~L14) — ⚠️ this site expects the
     **encrypted** settings dict (calls `decrypt_key` itself), unlike all others
- Settings → AI provider UI option in `build/*.html` (relay URL, pairing key, backend, model).
  **No external CDNs** (repo rule).
- Reachability: relay on the EngageHub host → reuse `host.docker.internal:host-gateway` (already
  in `docker-compose.yml` for Ollama); else point `RELAYENT_URL` at the relay's public URL.
- ⚠️ EngageHub work is gated by CLAUDE.md pre-flight before any PRD deploy (Docker build, manual
  test, `/run-tests`, Linear report, explicit approval, `/security-review`).

---

## Linear

- **Project:** [Relayent](https://linear.app/engagehub/project/relayent-f376224ef44d) (under the EngageHub team)
- **[ENG-82](https://linear.app/engagehub/issue/ENG-82)** — Build Relayent MVP → **Done**
  (needs a follow-up note for the status interface + Cursor adapter once committed)
- **[ENG-83](https://linear.app/engagehub/issue/ENG-83)** — Integrate Relayent into EngageHub → **In Progress** (not started)

Plan file: `/Users/navjyotnishant/.claude/plans/we-have-opgion-to-iterative-marshmallow.md`
