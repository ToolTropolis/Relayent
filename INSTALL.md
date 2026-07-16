# Installing Relayent

A complete setup guide. Every step states what to run, what you should see, and what to do
when you see something else.

This document is written to be followed exactly — by a person, or by an AI agent given the
task "set up Relayent". There is no separate machine version: the commands, expected outputs,
and failure modes below are the same either way.

---

## If you are an AI agent

Read this section first. It is the contract for doing this job.

**Autonomy.** Run anything in [Part 1](#part-1-install-the-bridge),
[Part 2A](#2a-localhost-the-default) and [Part 3](#part-3-verify-end-to-end) without asking.
**Stop and get explicit human approval before:** exposing a relay to the internet
([2C](#2c-public-relay-with-tls)), piping an unread `curl` into a shell, setting
`RELAYENT_ALLOW_INSECURE=1`, or opening a firewall port. If your instructions already
authorise a public deploy, treat that as the approval and say so in your report.

**Never print a pairing key** into a transcript, log, issue, PR, or commit. It spends real
money. `relayent-bridge config list` masks it; the 8-char fingerprint identifies it safely.
Never commit `.env` or `~/.relayent/config.env`.

**Information you need before starting.** Ask for anything missing rather than inventing it —
a guessed domain or an assumed open port produces a broken deploy that looks fine in a log:

| Needed for | Ask for |
|---|---|
| Any relay | Which scenario: localhost (2A), private network (2B), or public+TLS (2C) |
| 2C only | The domain name, and confirmation its A/AAAA record already points at this host |
| 2C only | Confirmation that ports 80 and 443 are reachable from the internet |
| 2C only | An email for Let's Encrypt expiry notices |
| 2C only | Explicit authorisation to expose the service publicly |
| Bridge | Which machine runs it, and that an AI CLI there is installed **and signed in** |

**Report evidence, not conclusions.** "Deployed successfully" is not a result. Paste the actual
command output for each verification step. If a check fails, report the failure and stop —
do not proceed to pair a bridge against a relay that failed its checks.

**Do not work around a refusal.** This software fails closed on purpose. If the relay refuses
to start, or the bridge refuses an `http://` URL, that is a control doing its job. Fix the
underlying problem; never reach for `RELAYENT_ALLOW_INSECURE` to silence it.

**What is proven and what is not** — calibrate your confidence:

| Section | Status |
|---|---|
| Part 1 (bridge), 2A (localhost), Part 3 (verification) | ✅ Executed command-by-command; output below is what they actually print |
| 2B (private network) | ⚠️ Same binaries as 2A, but the network path is untested |
| **2C (public + TLS)** | ⚠️ **Never run end-to-end.** Certificate issuance needs a real domain and public DNS. Compose config is validated; TLS is not. Expect friction and verify hard |

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

**On the relay host** (only if it is a different machine — a server, VPS, or container host):

```bash
git --version                 # to clone
docker compose version        # v2.x — required for the public+TLS deploy (2C)
go version                    # only if building the relay directly instead of via Docker
```

The relay host needs **no** AI CLI and no subscription — it only brokers jobs. Do not install
Claude/Codex/Cursor there.

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

> ⚠️ **This exposes a service that can spend your CLI subscription.**
> **Agents: get explicit human approval before this step** (or confirm your instructions
> already authorise it). Read [SECURITY.md](SECURITY.md) first.
>
> ⚠️ **This section has never been run end-to-end.** Let's Encrypt issuance requires a real
> domain and public DNS, which cannot be tested locally. The compose config is validated and
> the relay's own guards are tested; the certificate path is not. Verify every step rather
> than assuming it worked.

**You need, before starting:**

| # | Requirement | Why |
|---|---|---|
| 1 | A domain whose A/AAAA record already points at this host | ACME validates by connecting back to it |
| 2 | Ports **80 and 443** reachable from the internet | 80 is required for the HTTP-01 challenge — HTTPS alone is not enough |
| 3 | Docker with Compose v2 | The stack is containerised |
| 4 | An email for expiry notices | Optional but recommended |

**Preflight — run these first. They take seconds and catch the two most common failures:**

```bash
# 1. Does the domain actually resolve to THIS host?
dig +short relay.example.com          # -> must equal this host's public IP
curl -s ifconfig.me                   # -> this host's public IP

# 2. Is port 80 free? Caddy cannot bind it if something else holds it.
sudo lsof -i :80 -i :443 || echo "  ports free"

# 3. Is Docker Compose v2 present?
docker compose version                # -> Docker Compose version v2.x
```

If the `dig` result does not match the host's IP, **stop** — the certificate will never issue.
DNS changes take time to propagate; wait rather than retrying `up -d` in a loop, since repeated
failures count against Let's Encrypt rate limits.

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

**Expected in the Caddy log:** `certificate obtained successfully` for your domain. Issuance
usually takes 5–30 seconds. If you see repeated ACME errors, **stop and read them** — do not
loop on `up -d`. Let's Encrypt rate-limits failures, and enough of them will lock you out for
hours. To debug issuance safely, uncomment `acme_ca` (the staging endpoint) in
[deploy/Caddyfile](deploy/Caddyfile): staging issues untrusted certs with far looser limits.
Comment it out and `docker compose restart caddy` once it works.

The relay container is **not** published to the host — only Caddy's 443 is exposed, so TLS
cannot be bypassed.

#### Verification — all five must pass

Do not report success without running these. **Paste the actual output**; a claim of success
without this evidence is not a result. If any check fails, stop and fix it before pairing a
bridge — a relay that fails these is either unreachable or unsafe.

```bash
DOMAIN=relay.example.com
KEY=<your pairing key>

# 1. Reachable over TLS at all.
curl -s https://$DOMAIN/v1/health
# PASS: {"status":"ok"}

# 2. The relay KNOWS it is behind TLS (proves RELAYENT_TRUST_PROXY + Caddy headers).
curl -s https://$DOMAIN/v1/status -H "Authorization: Bearer $KEY" | grep -o '"tls":[a-z]*'
# PASS: "tls":true      FAIL: "tls":false -> the status page will warn users

# 3. Auth is enforced — no key is refused.
curl -s -o /dev/null -w '%{http_code}\n' https://$DOMAIN/v1/status
# PASS: 401

# 4. A wrong key is refused (not just a missing one).
curl -s -o /dev/null -w '%{http_code}\n' https://$DOMAIN/v1/status -H 'Authorization: Bearer wrong'
# PASS: 401

# 5. Plaintext redirects to HTTPS.
curl -sI http://$DOMAIN | head -1
# PASS: HTTP/1.1 308 Permanent Redirect (301 is also fine)
```

Then open `https://$DOMAIN` and check the **Security** card reads *"served over TLS with a
pairing key enforced"*. If it says **"NO — traffic is in the clear"**, stop and fix TLS before
pairing anything.

**Long-poll check (do this too).** ⚠️ Untested through a real proxy. The API long-polls for up
to 90s, and a proxy timeout that is too low breaks jobs in a way that looks like the bridge
failing:

```bash
time curl -s "https://$DOMAIN/v1/jobs/nonexistent?wait=1" -H "Authorization: Bearer $KEY"
# PASS: returns 404 promptly, or blocks ~90s then responds — either is fine.
# FAIL: a 502/504 from Caddy after ~30-60s means the proxy is cutting long-polls short.
#       Raise response_header_timeout in deploy/Caddyfile.
```

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

## Reporting the outcome (agents)

When you finish, report in this shape. The point is that a reviewer can tell what actually
happened without re-running anything — and can see what you did *not* verify.

```markdown
## Relayent setup — <localhost | private | public+TLS>

**Result:** working / partly working / failed

**What I did**
- Relay: <scenario>, <where it runs>
- Bridge: <machine>, backends ready: <list>
- Pairing key: fingerprint <8 chars> — generated with keygen / supplied by operator
  (never paste the key itself)

**Verification** — actual output:
$ curl -s https://relay.example.com/v1/health
{"status":"ok"}

$ curl -s .../v1/status ... | grep -o '"tls":[a-z]*'
"tls":true

<...every check, with its real output...>

**End-to-end job:** <the real job result JSON, proving the whole chain>

**Not verified / needs attention**
- <anything you could not check, and why>
- <any warning you saw but did not resolve>
```

Rules for the report:

- **Paste real output.** Never invent, summarise, or predict a command's result.
- **State failures plainly.** A partly-working deploy reported as working is worse than a
  failure reported honestly — someone will build on it.
- **Say what you skipped.** "I could not confirm the long-poll behaviour" is a useful sentence.
- **Never include the pairing key.** Fingerprint only.

---

## Where to go next

- [README.md](README.md) — what Relayent is, the API, integrating an app
- [SECURITY.md](SECURITY.md) — threat model, and **what Relayent does not protect against**
- [openapi.yaml](openapi.yaml) — the `/v1` contract, the only integration surface
