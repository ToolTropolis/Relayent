# Installing Relayent

A complete setup guide. Every step states what to run, what you should see, and what to do
when you see something else.

**Handing this to an AI agent?** This is the whole prompt:

> *Set up Relayent. Follow INSTALL.md exactly.*

The document is self-sufficient: [Step 0](#step-0--ask-before-you-touch-anything) makes the
agent interview you for the server details, credentials and approvals it needs before running
anything. You don't need to anticipate them — it will ask. Add details up front only if you
want to skip the questions.

There is no separate machine version of this guide. The commands, expected outputs, and
failure modes are the same for a person and an agent; what an agent additionally needs is
[Step 0](#step-0--ask-before-you-touch-anything) and the
[ground rules](#ground-rules-agents), both below.

---

# STEP 0 — Ask before you touch anything

**This applies to whoever is doing the setup, including an AI agent given nothing more than
"set up Relayent, follow INSTALL.md".** Everything this job needs is gathered here, so the
instruction really can be that short.

> **AGENT: STOP. Do not run any command yet.**
> Work through the interview below and **ask the operator every question you cannot answer
> from your instructions**. Ask them in one batch, not one at a time. Only then continue.
> **Never guess.** A guessed domain, an assumed-open port, or an invented email produces a
> deploy that fails in ways that still look fine in a log. Asking costs one message; guessing
> costs an afternoon.

## The interview

**Q1 — What are we installing?** (Both is common: the bridge on a laptop, the relay on a server.)

- [ ] **Bridge** — on the machine with the AI subscription → [Part 1](#part-1-install-the-bridge)
- [ ] **Relay** — the job broker → Q2
- [ ] Both

**Q2 — Where will the relay run, and who reaches it?** This picks the scenario. If the
operator is unsure, recommend 2A for trying it out, 2B for real use.

| Answer | Scenario | Needs approval? |
|---|---|---|
| "Same machine as my app" / "just trying it" | [2A localhost](#2a-localhost-the-default) | No — run it |
| "My VPN / Tailscale / private network" | [2B private](#2b-private-network-recommended-for-real-use) | No — run it |
| "On the internet" / "my web server" / a public domain | [2C public + TLS](#2c-public-relay-with-tls) | **YES — see Q3** |
| "…but nginx/Apache already uses 80/443 there" | [2D behind an existing proxy](#2d-behind-an-existing-reverse-proxy-nginxapache-already-on-80443) | **YES — see Q3** |

**Q3 — Only if the answer to Q2 was public (2C). Ask all of these:**

| Ask | Why it matters | Do not proceed without it |
|---|---|---|
| "Do you authorise exposing this relay publicly?" | It can spend the subscription of every paired bridge | An explicit yes |
| "What domain?" | Certificates are issued for a name | The real name — never a placeholder |
| "Does its DNS A/AAAA record already point at this host?" | ACME validates by connecting back | Verify with the preflight; don't take "yes" on faith |
| "Are ports 80 and 443 open to the internet?" | 80 is required for the ACME challenge — HTTPS alone fails | A yes, then confirm in the preflight |
| "Email for Let's Encrypt expiry notices?" | Optional but recommended | Ask; proceed without if declined |
| "Existing pairing key, or generate one?" | Some operators bring their own | Default to `keygen` |
| "Is anything already serving 80/443 on that host?" | Decides 2C (bundled Caddy) vs 2D (behind your proxy). Getting this wrong means a port clash or an outage | Check yourself: `sudo ss -ltnp \| grep -E ':(80\|443)\s'` |

**Q4 — Only if installing a bridge:**

| Ask | Why |
|---|---|
| "Which machine, and is an AI CLI installed **and signed in** there?" | Relayent reuses the CLI's session; logged-out = no jobs |
| "Which relay URL and pairing key should it pair with?" | If you just deployed the relay, use those |
| "Should jobs see a project directory, or the default empty sandbox?" | Default is the sandbox — recommend keeping it |

**Q5 — Anything that needs a decision you cannot make:** the repo is private (does the agent
have credentials?), a firewall change, `sudo`, or an unread `curl … | sh`. **Ask.**

## Then say what you are about to do

Before the first command, tell the operator in two lines: the scenario, and anything you will
need approval for. Then proceed.

---

## Ground rules (agents)

**Autonomy.** Run [Part 1](#part-1-install-the-bridge), [2A](#2a-localhost-the-default),
[2B](#2b-private-network-recommended-for-real-use) and [Part 3](#part-3-verify-end-to-end)
freely once Step 0 is answered. **Stop and get explicit approval before:** a public deploy
([2C](#2c-public-relay-with-tls)), piping an unread `curl` into a shell,
`RELAYENT_ALLOW_INSECURE=1`, opening a firewall port, or anything needing `sudo`.

**Never print a pairing key** into a transcript, log, issue, PR, or commit — it spends real
money. Use the 8-char fingerprint. Never commit `.env` or `~/.relayent/config.env`.

**Report evidence, not conclusions.** "Deployed successfully" is not a result. Paste real
command output. If a check fails, report it and stop — never pair a bridge to a relay that
failed its checks.

**Do not work around a refusal.** This software fails closed on purpose. A relay refusing to
start, or a bridge refusing an `http://` URL, is a control working. Fix the cause; never
silence it with `RELAYENT_ALLOW_INSECURE`.

**What is proven and what is not** — calibrate your confidence:

| Section | Status |
|---|---|
| Part 1 (bridge), 2A (localhost), Part 3 (verification) | ✅ Executed command-by-command; the outputs below are what they really print |
| 2B (private network) | ⚠️ Same binaries as 2A, but the network path is untested |
| 2C (public + TLS, bundled Caddy) | ⚠️ Compose config validated; the bundled Caddy stack itself has not been run end-to-end |
| **2D (behind an existing nginx)** | ✅ **This is the path that was proven**: cert issued, relay served over HTTPS behind a real nginx, `"tls":true` confirmed, a real job returned structured JSON, and a 90s long-poll survived the proxy |

**Two things 2C's live run turned up**, worth expecting:
- **Let's Encrypt's 50-certs-per-168h limit is per _registered domain_ and is shared.** On a
  free subdomain host (FreeDNS, DuckDNS…) other people's certs count against yours. The first
  attempt failed on exactly this; the window rolled 5 minutes later. Wait it out — never loop.
- **A cloud host's internal DNS resolver can lag well behind public ones.** On Oracle Cloud the
  record was live at every authoritative nameserver and at Google ~11 minutes before the
  server itself could resolve it. `certbot` runs on the server, so the server's view is the
  one that matters. Check with `getent hosts <domain>`, not `dig` from your laptop.

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

Status page: <http://localhost:8787> (enter the key). Every relay serves this at its own root
URL — see [Checking status](#checking-status).

### 2B: Private network (recommended for real use)

A relay on Tailscale/WireGuard, or bound to a private interface, gives you remote access with
no public surface. Use a real key — the relay will refuse to start without one:

```bash
./bin/relayent-relay keygen        # 43-char, 256-bit key
RELAYENT_PAIRING_KEY=<key> RELAYENT_LISTEN=:8787 ./bin/relayent-relay
```

Then pair the bridge against the private address. Note plain `http://` to a non-loopback host
is refused by the bridge — terminate TLS or keep the relay on loopback.

### Getting a domain and pointing it at your host

Skip this if you already have a domain whose DNS you control. Everything in 2C needs a name
that resolves to your relay host — a certificate is issued *for a name*, and ACME validates by
connecting back to it.

**A free subdomain is fine for this.** Two common options:

| Provider | Notes |
|---|---|
| [FreeDNS (afraid.org)](https://freedns.afraid.org) | Free subdomains on shared domains. What this guide was tested against. |
| [DuckDNS](https://www.duckdns.org) | Free `*.duckdns.org` subdomains, simpler UI. |

⚠️ **Shared-domain caveat, and it is not theoretical.** On FreeDNS/DuckDNS your subdomain lives
under someone else's registered domain (`ignorelist.com`, `duckdns.org`). Let's Encrypt's
**50-certificates-per-168-hours limit applies to the registered domain**, so *other people's*
certificates count against your quota. We hit exactly this: `too many certificates (50)
already issued for "…" in the last 168h0m0s`. It cleared in 5 minutes because the window
rolls continuously — but if you are unlucky it can be hours. **Never retry in a loop.** For
anything you depend on, use a domain you own.

**Create the record:**

```
Type:  A
Name:  relayent          (i.e. relayent.yourdomain.com)
Value: <your host's public IP>      # curl -s ifconfig.me   on the host
TTL:   default / 300
```

On FreeDNS: *Subdomains → Add*. Enter **just the subdomain part** in the Subdomain field —
entering the full name creates `relayent.yourdomain.com.yourdomain.com`.

**Then wait, and verify from the right place:**

```bash
# On YOUR machine — is the record live at all?
dig +short relayent.yourdomain.com

# On THE SERVER — this is the one that matters. certbot runs there.
ssh you@your-host 'getent hosts relayent.yourdomain.com'
```

⚠️ **The server's view is what counts, and it can lag badly.** Cloud hosts use their provider's
internal resolver, whose cache is independent of Google/Cloudflare and cannot be flushed by
you. On Oracle Cloud we watched the record go live at all four authoritative nameservers and
at Google a full **~11 minutes** before the server itself could resolve it. `resolvectl
flush-caches` did not help. **Do not run certbot until `getent hosts` on the server returns
your IP** — otherwise validation fails while your site is down, for nothing.

### 2C: Public relay with TLS

> ⚠️ **This exposes a service that can spend your CLI subscription.**
> **Agents: get explicit human approval before this step** (or confirm your instructions
> already authorise it). Read [SECURITY.md](SECURITY.md) first.
>
> ✅ **This has been run end-to-end against a real domain** — cert issued, HTTPS serving,
> `"tls":true` confirmed, a real job returned structured JSON, and a 90s long-poll survived
> the proxy. The two things that bit us are called out in the coverage table above (shared
> rate limits, and a cloud resolver lagging public DNS). Still verify every step: your DNS,
> ports and proxy are not ours.

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

### 2D: Behind an existing reverse proxy (nginx/Apache already on 80/443)

**Use this when something already serves ports 80/443 on the host** — an app, a site, another
service. 2C's bundled Caddy stack needs those ports, so it does not apply. This is the common
case on any server that already does something, and it is what this guide was tested against.

**There are no alternate ports.** Let's Encrypt's HTTP-01 challenge only validates on **port
80** and TLS-ALPN-01 only on **443**. You cannot move the challenge to 8443. So the relay goes
*behind* the proxy you already have, on a hostname of its own.

```
                    ┌──────────────────────────────┐
  Internet ──443──▶ │ your existing nginx          │
                    │  ├ app.example.com  → app    │   (untouched)
                    │  └ relayent.example.com ─────┼──▶ relayent-relay:8787
                    └──────────────────────────────┘        (no host ports)
```

#### 1. Check what you're working with

```bash
sudo ss -ltnp | grep -E ':(80|443)\s'      # what holds the ports?
which certbot || echo "certbot not installed"
sudo ls /etc/letsencrypt/renewal/           # existing certs — and how they renew
sudo grep authenticator /etc/letsencrypt/renewal/*.conf
```

That last one matters: **match the method already in use** rather than introducing a second
one. `authenticator = standalone` means certs were issued with the proxy stopped;
`authenticator = webroot` means it stayed up.

#### 2. Get the certificate — BEFORE touching any config

⚠️ **Ordering is not advice, it is load-bearing.** nginx does **not** skip a `server` block
whose `ssl_certificate` is unreadable — **it refuses to start at all**, taking every other site
on that host down with it. Deploy the vhost before the cert exists and you have an outage.

```bash
# 1. The server must resolve the name (see "Getting a domain", above)
getent hosts relayent.example.com          # must return this host's IP

# 2. standalone needs port 80, so the proxy stops briefly (~10s)
docker compose stop nginx                  # or: sudo systemctl stop nginx
sudo certbot certonly --standalone -d relayent.example.com
docker compose start nginx

# 3. Confirm before going further
sudo ls /etc/letsencrypt/live/relayent.example.com/fullchain.pem
```

If your proxy is already set up for **webroot** renewals, prefer that — no downtime:
```bash
sudo certbot certonly --webroot -w /var/www/certbot -d relayent.example.com
```

#### 3. Run the relay with no published ports

The relay must be reachable **only** by the proxy. Publishing it to the host would let callers
bypass TLS entirely by hitting `http://host:8787`.

```yaml
# docker-compose.yml — alongside your existing services
services:
  relayent-relay:
    image: relayent-relay:latest
    container_name: relayent-relay
    environment:
      RELAYENT_LISTEN: ':8787'
      RELAYENT_PAIRING_KEY: ${RELAYENT_PAIRING_KEY:-}
      RELAYENT_TRUST_PROXY: '1'      # your proxy terminates TLS — see below
    networks: [your-existing-network]   # MUST match the proxy's network
    security_opt: [no-new-privileges:true]
    read_only: true
    cap_drop: [ALL]
    restart: always
```

Build the image on the host (Relayent is a separate repo, so there is no local build context):

```bash
git clone https://github.com/ToolTropolis/Relayent.git /tmp/relayent
docker build -f /tmp/relayent/relay/Dockerfile -t relayent-relay:latest /tmp/relayent
docker run --rm relayent-relay:latest keygen >> /dev/null   # then put the key in .env
```

⚠️ **Do not use `${RELAYENT_PAIRING_KEY:?err}`.** Compose interpolates every service's
variables *before* applying profiles or starting anything, so a required-variable guard aborts
the **entire stack** — including your existing app — on any host where the key is unset. The
relay already refuses to start without a strong key when network-reachable; that is the control
you want. (We shipped this bug and caught it in testing.)

#### 4. Add the vhost

```nginx
server {
    listen 80;
    server_name relayent.example.com;
    location /.well-known/acme-challenge/ { root /var/www/certbot; }   # for webroot renewals
    location / { return 301 https://$host$request_uri; }
}

server {
    listen 443 ssl;
    server_name relayent.example.com;

    ssl_certificate     /etc/letsencrypt/live/relayent.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/relayent.example.com/privkey.pem;
    # Match the TLS policy of your other vhosts.

    location / {
        proxy_pass         http://relayent-relay:8787;
        proxy_http_version 1.1;
        proxy_set_header   Host              $host;
        proxy_set_header   X-Real-IP         $remote_addr;
        proxy_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;   # REQUIRED — see below

        proxy_hide_header  Content-Security-Policy;     # the relay serves its own, with a nonce

        proxy_read_timeout 120s;   # LOAD-BEARING — see below
        proxy_send_timeout 120s;
        proxy_buffering    off;
    }
}
```

**Three settings that are not decoration:**

| Setting | Why |
|---|---|
| `proxy_read_timeout 120s` | The `/v1` API **long-polls**: a result fetch blocks up to 90s, a bridge poll ~25s. nginx defaults to **60s**, which severs jobs mid-flight and surfaces as *"the bridge is broken"* rather than a proxy timeout. Verified: a real 90s long-poll returns 200 with this set. |
| `X-Forwarded-Proto` + `RELAYENT_TRUST_PROXY=1` | How the relay knows it is behind TLS. Without both, `/v1/status` reports `"tls":false` and the status page tells your users their traffic is unencrypted — while it actually is not. |
| **No `limit_conn`** | If your other vhosts cap connections per IP, do **not** copy that here. A bridge holds one long-poll open continuously and a consumer may hold another; a cap severs them. The relay does its own per-key and per-IP rate limiting. |

#### 5. Test the config before loading it

```bash
docker compose exec nginx nginx -t     # or: sudo nginx -t
```

**Only reload if that passes.** Use `reload`, never `restart` — reload keeps existing
connections and, crucially, will not take the site down on a bad config:

```bash
docker compose exec nginx nginx -s reload
```

#### 6. Verify — all must pass

```bash
DOMAIN=relayent.example.com; KEY=<your key>

curl -s https://$DOMAIN/v1/health                                        # {"status":"ok"}
curl -s https://$DOMAIN/v1/status -H "Authorization: Bearer $KEY" | grep -o '"tls":[a-z]*'
#   PASS: "tls":true    FAIL: "tls":false -> X-Forwarded-Proto / TRUST_PROXY not wired
curl -s -o /dev/null -w '%{http_code}\n' https://$DOMAIN/v1/status      # 401 (no key)
curl -sI http://$DOMAIN | head -1                                        # 301/308 -> https
curl -s -o /dev/null -w '%{http_code}\n' https://your-existing-app.example.com   # 200 — unaffected!

# The long-poll — with NO bridge running, this must block ~90s and return 200, not 502/504:
time curl -s -o /dev/null -w '%{http_code} after %{time_total}s\n' \
  "https://$DOMAIN/v1/jobs/$(curl -s -XPOST https://$DOMAIN/v1/jobs -H "Authorization: Bearer $KEY" \
     -d '{"backend":"cursor","prompt":"x"}' | sed -n 's/.*"job_id":"\([^"]*\)".*/\1/p')?wait=1" \
  -H "Authorization: Bearer $KEY"
```

#### Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| **Everything on the host is down** after a reload | The vhost references a cert that does not exist | Restore the old config, reload, get the cert first |
| `502 Bad Gateway` | The relay is not on the proxy's docker network, or not running | `docker network inspect <net>`; check `proxy_pass` resolves the container name |
| `"tls":false` over real HTTPS | `X-Forwarded-Proto` missing, or `RELAYENT_TRUST_PROXY` unset | Set both |
| `504` after ~60s on a job | `proxy_read_timeout` too low | Raise to 120s |
| Whole stack refuses to start | `${VAR:?err}` on an unset variable | Use `${VAR:-}` |
| `too many certificates … in the last 168h` | Shared registered domain (FreeDNS etc.) | Wait for the window. **Never loop.** |

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

## Checking status

Once it is running, two live views:

**The relay's web dashboard** — open the relay's own URL in a browser and enter your pairing
key. Works for any relay, not just localhost:

```
https://relay.example.com          # or http://localhost:8787
```

Shows relay health and uptime, whether a bridge is online, pending jobs, and each backend's
readiness — auto-refreshing every 5s. Its **Security** card grades the deployment: if it reads
*"NO — traffic is in the clear"*, the relay is network-reachable without TLS. Fix that before
using it for anything real. The key stays in the page and is only ever sent to the relay's
own `/v1` API.

The page shows your key's 8-char **fingerprint**, never the key. It matches
`relayent-bridge config list` — that is how you confirm both sides hold the same key.

**The bridge's terminal dashboard** — on the machine running the bridge:

```bash
relayent-bridge monitor      # Ctrl-C to quit
```

Connection, polling state, backends, and a live tail of recent jobs, refreshed every 2s.
Colour turns off automatically when piped. It never prints the key, so it is safe to
screenshot.

**Logs:**

```bash
tail -f ~/.relayent/logs/bridge.err.log     # job activity
journalctl --user -u relayent-bridge -f     # Linux
```

⚠️ Activity is in `bridge.err.log`, **not** `bridge.out.log` — Go's logger writes to stderr.
An empty `bridge.out.log` is normal, not a fault.

**Over the API**, from anywhere with the key:

```bash
curl -s $RELAY/v1/status              -H "Authorization: Bearer $KEY"   # health, tls, fingerprint
curl -s $RELAY/v1/bridge/online       -H "Authorization: Bearer $KEY"   # {"online":true}
curl -s $RELAY/v1/bridge/capabilities -H "Authorization: Bearer $KEY"   # ready backends
```

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
