# Relayent Security

Relayent connects an app you run to a CLI subscription on someone's laptop. That is
a genuinely sensitive thing to do, so this document states plainly what it protects,
what it does not, and what you must get right yourself.

If you only read one section, read [The one thing that matters](#the-one-thing-that-matters).

---

## The one thing that matters

**The pairing key is the only thing standing between the internet and your CLI
subscription.** Anyone who has it can send prompts to your machine and spend your
quota. It is a password. Treat it like one.

- Never commit it, paste it into a ticket, or send it over chat.
- Never serve a relay over plain `http://` off localhost — the key crosses the wire
  in cleartext on every request.
- Rotate it if you even suspect exposure (`relayent-relay rotate`).

Everything else in this document is detail around that sentence.

---

## Architecture, and why it is shaped this way

```
  Your app  ──POST──▶  Relay (public)  ◀──poll──  Bridge (your laptop)
                                                     │
                                                     └─▶ claude / codex / cursor-agent
```

The **bridge dials out**. Nothing listens on the user's machine — no ports are
opened, no tunnel is created, no inbound connection is ever accepted. A laptop
behind NAT needs no firewall changes, and there is no network attack surface on it
to speak of. The relay is the only internet-facing component, which is why all the
hardening lives there.

**No credentials pass through Relayent.** The bridge shells out to a CLI that is
already logged in. Relayent never sees, stores, or transmits your Claude/OpenAI/
Cursor credentials — it cannot, because it never has them.

---

## What Relayent protects against

| Threat | Control |
|---|---|
| Anyone using your relay | Bearer pairing key required on every `/v1` endpoint |
| A publicly deployed relay left open by accident | Relay **refuses to start** when network-reachable without a key of ≥24 chars |
| Guessing the key | Failed auth rate-limited per IP (8 burst, then 429); 401s never reveal whether the key was missing vs wrong |
| Learning the key from response timing | Constant-time comparison; the multi-key check does not short-circuit |
| Key theft in transit | Bridge refuses a remote `http://` relay; deploy guide uses automatic TLS |
| Burning your subscription quota | Enqueues rate-limited per key (1/s sustained, 30 burst) |
| Cross-tenant access | Jobs are namespaced by key — one key's bridge never sees another's jobs |
| Key exposure via logs | Keys are never logged; an 8-char SHA-256 fingerprint is shown instead. The Caddy config strips `Authorization` |
| Jobs reading your personal files | Jobs run in an empty sandbox (`~/.relayent/workspace`), never `$HOME` |
| Jobs editing files or running shell | Cursor uses `--mode ask` (read-only Q&A) |
| Config file theft by another local user | `~/.relayent/config.env` written atomically at `0600` |
| Slowloris / connection exhaustion | Explicit server read/write/idle timeouts |
| Header spoofing to bypass limits | `X-Forwarded-For`/`Proto` honoured only when `RELAYENT_TRUST_PROXY=1` |
| XSS on the status page | Bridge-reported values are validated server-side against a known set, rendered via `textContent` (never `innerHTML`), and the page's script runs under a per-request CSP nonce — `script-src` never uses `'unsafe-inline'`, which would also authorise injected `onerror=` handlers |
| Other web attacks | CSP forbids all external loads; `nosniff`, `DENY` framing, `no-referrer` |
| Rotation downtime | Multiple keys valid at once; old keys flagged `key_retiring` |

---

## What Relayent does NOT protect against

This section is the point of the document. Every one of these is real.

**Anyone with the pairing key is you.** There are no per-user identities, roles, or
audit trails. The key is a bearer token: possession is authorization. If you hand it
to five people, you have five people spending your subscription with no way to tell
them apart or revoke one of them. Rotation is all-or-nothing.

**A malicious relay sees every prompt.** The bridge sends prompts to whatever relay
it is paired with and runs whatever prompts come back. Only pair with a relay you
control or trust. Relayent cannot protect you from a relay operator who is hostile
or compromised.

**Prompt content is not encrypted end-to-end.** TLS protects it in transit, but the
relay process sees prompts and results in plaintext in memory. There is no E2E
encryption between app and bridge. Do not send secrets you would not want the relay
operator to read.

**Jobs consume your real quota.** A flood of jobs within the rate limit will still
exhaust your subscription. The limiter bounds the rate, not the total.

**The relay's queue is in-memory and unauthenticated at rest.** Anyone who can read
the relay process's memory, or exec into its container, can read pending prompts.
Job data lives up to 10 minutes (TTL).

**Prompt injection is not addressed.** If your app builds prompts from untrusted
input, that is your boundary to defend. Relayent passes prompts through verbatim.

**The CLIs are trusted completely.** Relayent shells out to `claude`, `codex`, and
`cursor-agent`. If one is compromised or malicious, Relayent gives it a network-
driven way to be invoked. `--mode ask` limits Cursor to read-only, but a CLI that
ignored its own flags would not be stopped by Relayent.

**Home is readable by the CLIs.** Jobs run in a sandbox, but the CLI processes can
still *read* `$HOME` — they must, to load their own auth sessions from `~/.claude`,
`~/.codex`, `~/.cursor`. The sandbox controls where jobs run and write, not
everything the CLI could theoretically read. On Linux the systemd unit narrows
writes via `ProtectSystem=strict`; macOS has no equivalent applied here.

**No supply-chain verification.** Releases are not signed and there are no published
checksums yet. `install.sh` builds from source when it can, precisely because that is
the path you can actually verify.

**The relay is not multi-tenant-safe in the strong sense.** Namespacing by key stops
accidental crossover, not a determined attacker who has a key.

---

## Deploying a public relay

If you put a relay on the internet, do these four things.

**1. Use a strong key.** Generate one; do not invent one.

```bash
relayent-relay keygen        # 256 bits, base64url
```

The relay refuses to start network-reachable without a key of ≥24 chars. That check
is a floor, not a strategy — use `keygen`.

**2. Use TLS. Always.** Free and automatic via Let's Encrypt:

```bash
cd deploy/
cp .env.example .env         # set RELAYENT_DOMAIN, RELAYENT_PAIRING_KEY, ACME_EMAIL
docker compose up -d
```

Caddy obtains and renews the certificate. The relay container is never published to
the host — only Caddy's 443 is exposed, so TLS cannot be bypassed. Ports 80 and 443
must be reachable for the ACME challenge.

**3. Keep the key out of your shell history and your repo.** Put it in `.env`
(gitignored) or your platform's secret store. `docker compose run --rm relay keygen`
prints one without it ever touching a config file you might commit.

**4. Rotate on a schedule, and immediately on suspicion.**

```bash
RELAYENT_PAIRING_KEY=$CURRENT relayent-relay rotate
```

This prints the two-phase procedure: run the relay with `new,old` so both work,
migrate bridges (watch `key_retiring` on `/v1/status`), then drop the old key. If a
key is compromised, skip the overlap and cut over at once — disconnecting every
bridge is the right trade there.

### Reducing exposure further

- **Don't expose it at all.** A relay on Tailscale/WireGuard, or bound to
  `127.0.0.1` beside your app, gives you everything with no public surface. This is
  the best option if you don't need internet reach.
- **Restrict by IP** at your firewall or reverse proxy if your consumers are fixed.
- **Separate keys per consumer** if you run several relays; a key cannot be revoked
  individually within one relay.

---

## Running a bridge safely

- **Only pair with a relay you trust.** It can send your machine any prompt.
- **The setup wizard refuses remote `http://`** — this is not a warning you should
  work around. Fix the relay's TLS instead.
- **Keep `~/.relayent/config.env` at `0600`.** `relayent-bridge doctor` checks this.
- **Jobs run in `~/.relayent/workspace`.** Leave it that way unless you specifically
  want jobs to see a project directory; `RELAYENT_WORKSPACE` widens it, and doing so
  grants read access to whatever you point it at.
- **The bridge runs as you, never root.** It needs no privileges beyond your own CLI
  sessions. Nothing in Relayent asks for sudo.
- **Stop it any time:** `relayent-bridge uninstall`. To remove the pairing entirely,
  delete `~/.relayent/config.env`.

---

## Reporting a vulnerability

Open a private security advisory on the GitHub repository, or email the maintainer.
Please do not open a public issue for a security bug. Include reproduction steps and
what an attacker gains — that is what determines priority.

---

## Security checklist

Before pointing a public relay at real users:

- [ ] Key generated with `keygen`, not invented
- [ ] Key stored in a secret store or gitignored `.env` — never committed
- [ ] TLS terminating in front of the relay; `/v1/status` reports `"tls": true`
- [ ] Relay container not published directly to the host
- [ ] `RELAYENT_TRUST_PROXY=1` set only because you run the proxy
- [ ] Rotation procedure rehearsed once before you need it under pressure
- [ ] Every bridge paired over `https://`
- [ ] You have read [What Relayent does NOT protect against](#what-relayent-does-not-protect-against) and accept those limits
