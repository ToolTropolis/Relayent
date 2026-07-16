# Installing Relayent

A complete setup guide. Every step states what to run, what you should see, and what to do
when you see something else.

This document is written to be followed exactly — by a person, or by an AI agent given the
task "set up Relayent". There is no separate machine version: the commands, expected outputs,
and failure modes below are the same either way.

> **Agents:** you may run every command in [Part 1](#part-1-install-the-bridge) and
> [Part 2A](#2a-localhost-the-default) without asking. **Stop and ask a human before**:
> exposing a relay to the internet ([2C](#2c-public-relay-with-tls)), running `curl … | sh`
> from an unread source, or setting `RELAYENT_ALLOW_INSECURE=1`. Never print a pairing key
> into a transcript, log, issue, or commit — use `config list`, which masks it.

---

## What you are setting up

Two components, and it matters which goes where:

| Component | Runs where | Purpose |
|---|---|---|
| **Bridge** | The machine with your AI CLI subscription (your laptop) | Dials **out** to the relay, runs jobs on your logged-in CLI |
| **Relay** | Anywhere both your app and your bridge can reach | Job broker exposing the `/v1` HTTP API |

The bridge never accepts inbound connections — no ports are opened on your machine. The relay
is the only thing anything connects *to*.

**The pairing key is a shared secret between them.** Anyone holding it can send prompts to
your machine and spend your subscription. Treat it like a password throughout.

---

## Prerequisites

**On the bridge machine**, at least one AI CLI installed **and signed in**:

```bash
claude --version          # Claude Code   — https://claude.com/claude-code
codex --version           # Codex         — https://developers.openai.com/codex
cursor-agent status       # Cursor        — https://cursor.com/cli
```

At least one must succeed. For Cursor you should see `✓ Logged in as <you>`. **Signed in
matters more than installed**: Relayent reuses the CLI's own session and never handles
credentials, so an installed-but-logged-out CLI cannot run jobs.

**To build from source** (recommended): Go 1.22+ (`go version`). On macOS: `brew install go`.

---

## Part 1: Install the bridge

On the machine whose subscription you want to use.

### 1.1 Install

```bash
git clone https://github.com/ToolTropolis/Relayent.git
cd Relayent
./install.sh
```

`install.sh` builds from source when Go is present, installs to `~/.local/bin`, never uses
`sudo`, and never writes outside `$HOME`. It then launches the pairing wizard.

**Expected:** a platform line, `✓ Built …`, a list of detected CLIs, then the wizard.

**If `~/.local/bin` is not on your PATH**, the installer says so. Add it:

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc
```

> A published release will support `curl -fsSL … | sh`. Piping a remote script into a shell
> executes whatever that URL returns — read it first, here or anywhere else.

### 1.2 Pair with a relay

If you don't have a relay yet, do [Part 2](#part-2-set-up-a-relay) first and come back — the
wizard verifies the relay before saving, so it needs one running.

```bash
relayent-bridge setup
```

It asks for the **relay URL** and the **pairing key**, then verifies both.

**Expected:**

```
  ✓ Paired with relay (version 1.0.0)
  ✓ Saved /Users/you/.relayent/config.env (owner-only, 0600)
  ✓ Key fingerprint: YI2iV3WH  (matches the relay's status page)

  Backends on this machine:
    ✓ claude   ready
    ✓ cursor   ready
    · gemini   CLI not installed
```

Nothing is saved unless verification succeeds.

| You see | Meaning | Fix |
|---|---|---|
| `refusing http:// for a remote relay` | The key would cross the network in cleartext | Use `https://`. Fix the relay's TLS ([2C](#2c-public-relay-with-tls)) — do not work around this |
| `the relay rejected this pairing key (401)` | Key mismatch | Compare with the relay's `RELAYENT_PAIRING_KEY` |
| `cannot reach the relay` | Not running, or DNS/firewall | Check the relay is up and the URL is right |
| No backend shows `ready` | CLI missing or logged out | Revisit [Prerequisites](#prerequisites) |

### 1.3 Run it at login

```bash
relayent-bridge install
```

Registers a per-user service (launchd on macOS, systemd `--user` on Linux): starts at login,
restarts on failure, **never runs as root**.

**Expected:** `✓ Installed as a login service (launchd)` plus the plist and log paths.

**Verify:**

```bash
relayent-bridge status     # -> ✓ launchd state: running
relayent-bridge monitor    # live dashboard; Ctrl-C to exit
```

In `monitor`, look for **Polling ● online** and at least one **✓ ready** backend.

---

## Part 2: Set up a relay

Pick **one** based on where your app runs. If unsure, start with 2A.

### 2A: Localhost (the default)

For an app on the same machine. **No exposure, nothing to secure.**

```bash
make all
RELAYENT_PAIRING_KEY=devkey RELAYENT_LISTEN=127.0.0.1:8787 ./bin/relayent-relay
```

**Expected:** `listening on 127.0.0.1:8787 (loopback-only), pairing key fingerprint=…`

The word **loopback-only** is the important part — it is unreachable from the network, which
is why a weak key like `devkey` is tolerated here and nowhere else.

**Verify:** `curl -s localhost:8787/v1/health` → `{"status":"ok"}`

Status page: <http://localhost:8787> (enter the key).

### 2B: Private network (recommended for real use)

A relay on Tailscale/WireGuard, or bound to a private interface, gives you remote access with
no public surface. Use a real key — the relay will refuse to start without one:

```bash
./bin/relayent-relay keygen        # 43-char, 256-bit key
RELAYENT_PAIRING_KEY=<key> RELAYENT_LISTEN=:8787 ./bin/relayent-relay
```

Then pair the bridge against the private address. Note plain `http://` to a non-loopback host
is refused by the bridge — terminate TLS or keep the relay on loopback.

### 2C: Public relay with TLS

> ⚠️ **This exposes a service that can spend your CLI subscription. Agents: get explicit
> human approval before this step.** Read [SECURITY.md](SECURITY.md) first.

Requirements: a domain whose A/AAAA record points at the host, ports **80 and 443**
reachable (80 is required for the ACME challenge), Docker with Compose.

```bash
cd deploy/
cp .env.example .env
docker compose run --rm relay keygen        # generate a strong key
```

Put the key and your domain in `.env`:

```bash
RELAYENT_DOMAIN=relay.example.com
ACME_EMAIL=you@example.com
RELAYENT_PAIRING_KEY=<the key you just generated>
```

`.env` is gitignored. Never commit it.

```bash
docker compose up -d
docker compose logs -f caddy      # watch the certificate get issued
```

**Expected:** Caddy obtains a Let's Encrypt certificate automatically (free, auto-renewing).
The relay container is **not** published to the host — only Caddy's 443 is exposed, so TLS
cannot be bypassed.

**Verify — all four must pass:**

```bash
curl -s https://relay.example.com/v1/health
# -> {"status":"ok"}

curl -s https://relay.example.com/v1/status -H "Authorization: Bearer $KEY" | grep '"tls":true'
# -> must contain "tls":true

curl -s -o /dev/null -w '%{http_code}\n' https://relay.example.com/v1/status
# -> 401  (no key = refused)

curl -sI http://relay.example.com | head -1
# -> 301/308 redirect to HTTPS
```

Then open `https://relay.example.com` and check the **Security** card reads *"served over TLS
with a pairing key enforced"*. If it says **"NO — traffic is in the clear"**, stop and fix TLS
before pairing anything.

| Problem | Cause | Fix |
|---|---|---|
| `refusing to start: RELAYENT_PAIRING_KEY is not set` | Working as designed — a public relay must have a key | `docker compose run --rm relay keygen`, put it in `.env` |
| `…is too short (N chars, need >= 24)` | Weak key | Use `keygen`. A guessable key is no key |
| Certificate never issues | Port 80 blocked, or DNS not propagated | Open 80/443; `dig relay.example.com` |
| `"tls":false` behind HTTPS | `RELAYENT_TRUST_PROXY` not set | The bundled compose sets it; only set it when you run the proxy |

---

## Part 3: Verify end-to-end

The only test that matters — a real job on your real subscription:

```bash
KEY=<your pairing key>
RELAY=http://localhost:8787          # or https://relay.example.com

# 1. A bridge must be polling. If false, nothing else will work.
curl -s $RELAY/v1/bridge/online -H "Authorization: Bearer $KEY"
# -> {"online":true}

# 2. Which backends can actually run? Trust "ready", not "installed".
curl -s $RELAY/v1/bridge/capabilities -H "Authorization: Bearer $KEY"

# 3. Run a job (use a backend showing ready:true).
ID=$(curl -s -XPOST $RELAY/v1/jobs -H "Authorization: Bearer $KEY" \
  -d '{"backend":"claude","prompt":"Reply with JSON about the number 7.",
       "json_schema":{"type":"object","properties":{"value":{"type":"integer"},
       "is_prime":{"type":"boolean"}},"required":["value","is_prime"]}}' \
  | sed -n 's/.*"job_id":"\([^"]*\)".*/\1/p')

# 4. Wait for the result (long-polls).
curl -s "$RELAY/v1/jobs/$ID?wait=1" -H "Authorization: Bearer $KEY"
# -> {"id":"...","status":"done","json":{"is_prime":true,"value":7}}
```

`"status":"done"` with a `json` object means the whole chain works: app → relay → bridge →
your CLI subscription → back.

| Result | Meaning | Fix |
|---|---|---|
| `{"online":false}` | No bridge polling with this key | `relayent-bridge status`; check both sides use the same key |
| `not supported yet by this bridge` | Backend is a stub (`gemini`) | Use one showing `ready:true` |
| `status:"error"` naming a CLI | The CLI failed — usually logged out | Run it manually, e.g. `cursor-agent status` |
| Nothing after ~90s | Bridge claimed it but the CLI hung | `relayent-bridge monitor` and watch the log panel |

---

## Configuring

Like `aws configure`: a wizard first, individual settings after.

```bash
relayent-bridge config list                    # every setting + which source wins
relayent-bridge config set workspace ~/code    # change one value
relayent-bridge config unset workspace         # back to the default
relayent-bridge config get pairing-key         # masked; --reveal to print in full
```

| Setting | Default | Notes |
|---|---|---|
| `relay-url` | — | `https://` required off-localhost |
| `pairing-key` | — | Secret. Masked in all output |
| `workspace` | `~/.relayent/workspace` | **Where jobs run.** An empty sandbox by default |
| `poll-wait` | `25` | Seconds per long-poll |
| `job-timeout` | `180` | Max seconds per CLI invocation |

Values live in `~/.relayent/config.env` (`0600`). **Environment variables override the file** —
`config list` shows which source is in effect, which is the answer when a change "does
nothing". Restart after changes:

```bash
relayent-bridge uninstall && relayent-bridge install
```

### The workspace

Jobs run in `~/.relayent/workspace`, an **empty** directory — not your home folder. This is
deliberate: jobs are read-only Q&A and need none of your files, and a CLI launched from `$HOME`
makes macOS attribute its file access to the bridge, which is what triggers
Desktop/Documents/Downloads permission prompts.

Point it elsewhere only if you specifically want jobs to see a project:

```bash
relayent-bridge config set workspace ~/code/my-project
```

That grants read access to that directory. Don't set it to `$HOME`.

---

## Rotating the pairing key

```bash
RELAYENT_PAIRING_KEY=$CURRENT ./bin/relayent-relay rotate
```

It prints a new key and the two-phase procedure. The short version — the relay accepts several
keys at once, so rotation costs no downtime:

1. Restart the relay with `RELAYENT_PAIRING_KEY=new,old` (both work).
2. Move each bridge to the new key: `relayent-bridge config set pairing-key <new>`, restart.
   Watch `key_retiring` on `/v1/status`; when no bridge reports `true`, continue.
3. Restart the relay with only the new key.

**If a key may be compromised, skip the overlap** and cut straight to the new key. That
disconnects every bridge at once, which is the right trade.

---

## Uninstalling

```bash
relayent-bridge uninstall            # stop and remove the service
rm ~/.relayent/config.env            # remove the pairing key from this machine
rm -rf ~/.relayent                   # remove everything (config, logs, workspace)
```

`uninstall` deliberately leaves your pairing in place; the second command is what removes the
credential.

---

## Troubleshooting

**Start here:**

```bash
relayent-bridge doctor
```

It checks config, file permissions, relay reachability, and per-backend readiness in one pass.

| Symptom | Likely cause | Fix |
|---|---|---|
| `no relay configured` | Never paired | `relayent-bridge setup` |
| Service installed but `offline` | Crashing at startup | `cat ~/.relayent/logs/bridge.err.log` |
| Backends ready in the terminal, not under the service | launchd's minimal PATH hides the CLIs | The plist pins PATH; reinstall with `uninstall && install` |
| macOS asks for Desktop/Documents access | Workspace widened to `$HOME` | `relayent-bridge config unset workspace` |
| `429 too many failed attempts` | Rate limiter saw bad keys | Wait ~1 min; fix the key |
| Jobs slow/timing out | CLI itself is slow | Raise `job-timeout`; try the CLI by hand |
| Logs growing | They shouldn't — rotated at 5 MiB × 3 | Check `~/.relayent/logs/` |

**Logs:**

```bash
tail -f ~/.relayent/logs/bridge.err.log     # activity lands here (Go logs to stderr)
journalctl --user -u relayent-bridge -f     # Linux
```

---

## Runbook (copy-paste)

Fastest path to a working localhost setup:

```bash
# Build
git clone https://github.com/ToolTropolis/Relayent.git && cd Relayent && make all

# Relay (terminal 1)
RELAYENT_PAIRING_KEY=devkey RELAYENT_LISTEN=127.0.0.1:8787 ./bin/relayent-relay

# Bridge (terminal 2)
./install.sh          # answer: http://localhost:8787  /  devkey
relayent-bridge install

# Verify (terminal 3)
curl -s localhost:8787/v1/bridge/online -H 'Authorization: Bearer devkey'
# -> {"online":true}
```

For anything beyond localhost, use a `keygen` key and read [SECURITY.md](SECURITY.md).

---

## Where to go next

- [README.md](README.md) — what Relayent is, the API, integrating an app
- [SECURITY.md](SECURITY.md) — threat model, and **what Relayent does not protect against**
- [openapi.yaml](openapi.yaml) — the `/v1` contract, the only integration surface
