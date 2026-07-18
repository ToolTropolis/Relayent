# AGENTS.md

Conventions for AI agents working on this repository. Read before changing code.

For *using* or *installing* Relayent, read [INSTALL.md](INSTALL.md) instead — it is written to
be followed by agents and humans alike.

---

## What this is

Relayent routes AI jobs from any app to a **CLI subscription on a user's machine** (Claude
Code, Codex, Cursor), so users don't pay twice for models they already have.

- **relay/** — internet-facing HTTP job broker (`/v1`). The only component an attacker reaches.
- **bridge/** — daemon on the user's machine. Dials **out**, runs the local CLI, posts results.
- **internal/api/** — shared wire types. `openapi.yaml` is the contract.

```
App ──POST──▶ Relay ◀──long-poll── Bridge ──▶ claude / codex / cursor-agent
```

## Build and test

```bash
export PATH="/opt/homebrew/bin:$PATH"        # macOS: Go is installed via brew
go build ./... && go vet ./... && go test ./...
gofmt -l relay/ bridge/ internal/            # must print nothing
make all                                      # -> bin/relayent-relay, bin/relayent-bridge
```

Run all four before claiming a change works. `go vet` must be silent.

---

## Security invariants — do not break these

Each one exists because of a specific incident or decision. Changing any requires
understanding why it is there.

**1. The pairing key is the only thing protecting a user's subscription.**
Never log it, print it, or put it in an error message. Show `keyFingerprint()` (8-char SHA-256)
instead. Compare with `subtle.ConstantTimeCompare`, never `==`.

**2. A network-reachable relay must fail closed.**
`validateKeySetPolicy` refuses startup without a ≥24-char key. Do not soften this to a warning;
warnings are ignored exactly when they matter. `RELAYENT_ALLOW_INSECURE=1` is the only bypass.

**3. Untrusted data must never reach the DOM as markup.** ⚠️ *This was a real HIGH vuln.*
`BackendInfo.Name` comes from anyone with a key. It was concatenated into `innerHTML` and could
read the operator's pairing key out of the status page. Three independent layers now stop it —
`textContent`/`createElement` at the sink, `sanitizeCapabilities()` at the source, and a CSP
nonce. **Do not remove any layer assuming another holds.**
Never add `'unsafe-inline'` to `script-src`: it authorises inline `onerror=` handlers, which is
exactly how the payload executed.

**4. Jobs run in a sandbox, never `$HOME`.** ⚠️ *This was a real bug.*
Every adapter must set `cmd.Dir = req.WorkDir`. A CLI launched from `$HOME` makes macOS
attribute its file access to the bridge, prompting the user for Desktop/Documents access.
The launchd plist and systemd unit must point `WorkingDirectory` at the workspace.
Home stays *readable* on purpose — the CLIs load their sessions from `~/.claude`, `~/.codex`,
`~/.cursor`.

**5. Never inject API keys into adapters.**
`cmd.Env = os.Environ()` and nothing more. The entire premise is reusing the CLI's own
subscription session. An `ANTHROPIC_API_KEY` would silently bill the user.

**6. The bridge never opens a port and never runs as root.**
It dials out only. Services are per-user (launchd `gui/<uid>`, systemd `--user`). Nothing
requires `sudo`.

**7. Plaintext to a remote host is refused, not warned about.**
`validateRelayURL` rejects `http://` off-loopback. The key and every prompt would be on the
wire.

**8. `X-Forwarded-For`/`Proto` are trusted only when `RELAYENT_TRUST_PROXY=1`.**
Otherwise any caller forges them to bypass rate limits or fake `tls:true`.

---

**9. Multi-tenant isolation is authorization, and it is tested.** Jobs route by `Principal.UserID`
only. A bridge principal may claim/return only its bound user's jobs; an app principal must name
a `target_user` and cannot self-route; a self-routing principal (bridge/legacy/OIDC) may NOT
enqueue for a different user (the anti-spoof guard in `routeTarget`). Do not weaken these — the
cross-tenant tests in `queue_test.go`/`route_test.go` are the proof.

**10. No prompt/result content is ever at rest.** The control-plane store (`store.go`) holds
identity, hashed credentials, and audit metadata only. `AuditEvent` has byte-LENGTH fields
(`PromptLen`/`ResultLen`), never the bytes. If you add a field to any stored struct, it must not
be able to hold content — a test greps the DB file to prove it.

**11. Credentials are stored hashed, never raw.** OIDC means no password at rest. Machine
credentials are `<id>.<secret>`; only `sha256(secret)` is stored, compared constant-time
(`verifySecret`). The bootstrap admin token is a skeleton key — enforce the ≥24-char floor on a
network-reachable relay.

**12. Admin scope is granted, never assumed; humans sign in on one surface.** OIDC login is at
`/login` (`login.go`); the callback (`oidc.go`) routes by role — admin→`/admin`, user→`/`. The
**first** stored user bootstraps to admin (`CountUsers()==0`); all others need explicit promotion
via `POST /v1/admin/users/{sub}/role`. `UpsertUser` **preserves** an existing role, so a login
can't self-promote — do not change that. An admin can't self-demote/self-delete (`admin.go`
guards). The admin API (`admin.go`) and console (`adminpage.go`) never return content;
`GET /v1/admin/config` returns non-secret config only (booleans for whether secrets are set).

## House style

**Comments explain *why*, never *what*.** The code shows what. A comment earns its place by
recording a constraint, a hard-won gotcha, or a rejected alternative — something the next
reader cannot recover from the code.

```go
// Good — records a constraint you cannot see:
// --json-schema takes an INLINE JSON string, not a file path. Passing a path
// makes the CLI hang forever.

// Bad — restates the code:
// Set the working directory
cmd.Dir = req.WorkDir
```

**File headers.** Every file carries the existing banner (Primary author / Created / Last
updated / Description / AI usage). Match it.

**Errors are actionable.** Say what went wrong *and* what to do:

```go
return fmt.Errorf("refusing to start: RELAYENT_PAIRING_KEY is not set and %s is reachable...\n"+
    "  Generate one:  relayent-relay keygen", listenAddr)
```

**Adding a backend:** implement `adapters.Adapter` in `bridge/adapters/`, add one line to
`NewRegistry()`. Stubs return `Available()=false` and implement `BinPresent()` so the UI can
tell "CLI missing" from "not implemented". Add the name to `knownBackends` in
`relay/security.go` or the relay will drop it. Set `cmd.Dir = req.WorkDir`.

**Wire types:** `internal/api/types.go` and `openapi.yaml` must stay in step, including the
admin/auth surface (`/v1/admin/*`, `/v1/auth/*`) and the `/login` + `/admin` HTML pages. Verify
the spec against live responses rather than assuming — a field diff has caught real drift here.

---

## Verification standards

**Tests passing is not verification.** This session's two worst bugs — the XSS and the empty
monitor log panel — both had green tests. Drive the real thing:

- Relay/bridge changes → run a real job end-to-end, confirm structured JSON returns.
- Status page changes → drive a real browser, confirm **zero console errors**.
- Security changes → attempt the actual attack, confirm it fails.
- Service changes → install it, check `lsof`/`launchctl`, then uninstall.

**Report honestly.** If something is untested, say so. The TLS deploy stack has never been
verified end-to-end (it needs a real domain and public DNS) — that fact is stated in the docs
rather than glossed over. Do the same for anything you cannot test.

## Testing notes

- API keys must be **unset** when testing the bridge, to prove subscription use:
  `env -u ANTHROPIC_API_KEY -u OPENAI_API_KEY ./bin/relayent-bridge`
- macOS has no `timeout`. Use `perl -e 'alarm N; exec @ARGV' …`.
- `relayent-bridge monitor` never exits — don't run it in a blocking pipe.
- Clean up: `pkill -f relayent-relay; pkill -f relayent-bridge;
  launchctl bootout gui/$(id -u)/com.relayent.bridge`
- gopls reports false errors when Relayent isn't the IDE workspace root. `go build` is the
  source of truth.

## Gotchas (hard-won — don't rediscover)

1. `claude --json-schema` takes an **inline JSON string**, not a file path. A path hangs forever.
2. Structured output is unreliable across CLI versions. Every adapter instructs JSON in-prompt,
   echoes the schema, and does one repair retry.
3. `cursor-agent` needs `--trust` headlessly and takes the prompt as an **argument**, not stdin.
   `--mode ask` keeps it read-only — that flag is load-bearing.
4. **"CLI on PATH" ≠ "backend usable."** Hence `Installed`/`Supported`/`Ready`. Only `Ready`
   means jobs will work.
5. Go's `log` writes to **stderr**. Bridge activity lands in `bridge.err.log`, not
   `bridge.out.log`. Reading only stdout left the monitor's log panel silently empty.
6. launchd/systemd do **not** rotate a service's logs. The bridge rotates its own
   (`logrotate.go`). Rotation copies and truncates **in place** — launchd holds the fd, not the
   path, so a rename would send later writes into the archive.
7. launchd gives agents a minimal PATH. The plist pins `~/.local/bin` and `/opt/homebrew/bin`
   or the CLIs vanish.
8. YAML flow mappings in `openapi.yaml` (`{ type: string, description: ... }`) break on an
   unquoted `?` or `,`. Quote those descriptions.
9. System python3 is externally managed (PEP 668). Use a throwaway venv, never
   `--break-system-packages`.

---

## Boundaries

**Ask a human before:**
- Exposing a relay to the internet, or weakening any control in *Security invariants*.
- `git push`, opening a PR, or anything that leaves the machine. The repo is **private**
  (`ToolTropolis/Relayent`) — keep it that way unless told otherwise.
- Deleting a user's `~/.relayent/config.env` (their credential) or `~/.relayent/workspace`.

**Never:**
- Print a pairing key into a transcript, log, commit, or issue. `config list` masks it — use that.
- Commit `.env`, `~/.relayent/config.env`, or anything with key material.
- Add a paid-API fallback. Failing fast when the bridge is offline is the product guarantee:
  only the subscription is ever billed.
