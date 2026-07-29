# Design note: end-to-end encryption of prompts & results

**Status:** proposal / not implemented
**Author:** design sketch
**Scope:** close the one documented trust gap — the relay operator can read prompt and result content while a job is in flight.

## The problem, precisely

Today every hop is TLS-encrypted *in transit*, but the payload is **plaintext inside the relay process**. A job carries the prompt as a plain string, and the bridge returns the result the same way:

- `api.EnqueueRequest.Prompt` / `.System` — the app's prompt, in the clear ([internal/api/types.go](../../internal/api/types.go))
- `api.Job.Prompt` / `.System` — the same fields the bridge claims from `GET /v1/jobs/next`
- `api.ResultRequest.Text` / `.JSON` — the bridge's answer, in the clear on the way back

The relay holds these in memory in [`jobEntry`](../../relay/queue.go) for the life of the job (plus a TTL grace window). So the operator of the relay process *could* read them. `SECURITY.md` states this honestly: **isolation here is authorization, not encryption.**

## Why the fix is viable

The important observation is that **the relay never needs to read payload content** — it only routes. Everything it does is keyed on `userID` and job `id`:

- `Enqueue(userID, id, job)` stores the job and wakes that user's bridge.
- `ClaimNext(userID, …)` hands the next job to the user's bridge.
- `SetResult(userID, id, res)` / `Fetch(userID, id, …)` move the result back, scoped to the same user.
- `snapshotLocked` copies `Text` / `JSON` / `Error` straight through without inspecting them ([relay/queue.go](../../relay/queue.go)).

Nothing in the relay parses, transforms, filters, or logs `Prompt` / `Text`. The relay is **already a blind router in behaviour** — the payload just happens to be readable. That is exactly the precondition that makes end-to-end encryption a drop-in rather than a rewrite: if the content were ciphertext, every routing operation above would still work unchanged.

## Proposed design: App ⇄ Bridge sealed payloads

The app encrypts the prompt so that **only the target user's bridge** can decrypt it; the bridge encrypts the result so that **only the originating app** can read it. The relay sees ciphertext plus the routing metadata it already needs.

### Keys

- Each **bridge** owns an X25519 keypair. The public key is registered at enrollment (redeeming the one-time token in `POST /v1/enroll`) and republished in `BridgeCapabilities`. The private key never leaves the user's machine — consistent with the dial-out, "nothing to attack on your machine" posture.
- Each **app** (or demo server) owns an X25519 keypair. Its public key is handed to the bridge inside the sealed job so the bridge can encrypt the reply back.
- The relay stores and serves bridge public keys as opaque routing metadata. It holds **no** private keys and cannot decrypt anything.

### Wire changes (additive, versioned)

Add sealed fields alongside the existing plaintext ones so old and new peers interoperate during rollout:

```
EnqueueRequest / Job:
  enc_alg        string   // e.g. "x25519-xchacha20poly1305"; empty = legacy plaintext
  enc_prompt     []byte   // sealed to the bridge's public key
  app_pubkey     []byte   // so the bridge can seal the result back to this app

ResultRequest / JobResult:
  enc_alg        string
  enc_result     []byte   // sealed to app_pubkey
```

When `enc_alg` is set, the plaintext `Prompt`/`System`/`Text`/`JSON` fields are omitted. The relay's `jobEntry`, queue maps, TTL janitor, and per-user scoping are **untouched** — they never looked inside the payload.

### Flow

1. App fetches the target bridge's public key (via a capabilities/directory lookup) and seals the prompt to it.
2. App `POST /v1/jobs` with `enc_prompt` + `app_pubkey`. Relay routes by `userID` as today.
3. Bridge `GET /v1/jobs/next`, decrypts with its private key, runs the CLI, seals the result to `app_pubkey`.
4. Bridge `POST …/result` with `enc_result`. Relay routes it back; app decrypts.

The relay's view is reduced to: *who* (userID), *which job* (id), *which backend*, *status* — never *what*.

## What this closes and what it doesn't

**Closes:** the operator can no longer read prompt or result content. The trust boundary moves from "trust the relay operator" to "trust your own bridge," which the user already runs.

**Still true:** the relay sees routing metadata — userID, job id, backend name, timing, sizes. That is unavoidable for a router and should be stated plainly. Traffic-analysis (sizes/timing) is out of scope.

## Cost / open questions

- **Key distribution.** Works cleanly when app and bridge can exchange/pin public keys out of band (a team setting). Needs a directory endpoint to serve bridge public keys, with a trust decision about who may look them up.
- **The demo playground breaks.** The public demo's *relay-side* server is the "app," so there is no independent client keypair on the browser side; a browser-origin keypair would be a larger piece. Likely the demo stays plaintext-with-caveat, and E2E is a team/self-host feature. **Decision needed.**
- **No content features may exist.** This is only viable because nothing server-side inspects payloads. Any future feature that needs to read content (server-side moderation, transforms) is incompatible and must be weighed against this.
- **Key rotation & revocation** for bridge keypairs — reuse the existing enrollment/rotation machinery.

## Recommendation

Ship it as an **opt-in, additive** capability for self-hosted / team relays, keep the demo on the documented plaintext caveat, and only after this lands change the blog/diagram claim from "the operator could read them" to "the relay routes ciphertext; only your bridge can read the prompt." Until then, the honest caveat stays.
