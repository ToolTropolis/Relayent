# Relayent API Guide

How to integrate an app with Relayent, and why each call exists.

[`openapi.yaml`](openapi.yaml) is the machine-readable contract — generate a client from it,
paste it into Swagger/Postman. **This document is the part a spec cannot give you**: what to
call, in what order, what the numbers mean, and which mistakes are worth avoiding.

You need three things: an **HTTP client**, a **relay URL**, and a **pairing key**. No SDK. The
Python client in `clients/python/` is convenience, not a dependency.

---

## If you were handed this to do an integration

**This document is sufficient on its own.** Read it, then integrate. But the two things it
cannot know are yours to establish first:

> **STOP and get these before writing code:**
>
> | Need | Why you cannot guess it |
> |---|---|
> | **The relay URL** | There is no default. `https://` is required off-localhost. |
> | **The pairing key** | A credential. It is not in any repo, and inventing one wastes an afternoon on 401s. |
> | **Which backend** (`claude`/`codex`/`cursor`) | Ask, or read it from `capabilities` and pick a `ready` one. |
> | **The offline policy** | When no bridge is online, does the feature degrade, queue, error, or fall back to a paid API? **This is a product decision, not a technical one — ask a human.** Relayent will never fall back silently; your code decides. |
>
> **Never print the pairing key** into a transcript, log, commit, or PR. It spends real money.
> **Never commit it.** Read it from config/env at runtime like any other secret.

**Where the integration goes.** Relayent is a provider, not a framework. If the app has one
`call_ai()` function, this is a single branch there — an afternoon. If its provider logic is
copy-pasted across N call sites (common), it is N branches, and finding them is most of the
work. Grep for the existing provider names first.

**Definition of done** — an integration is not finished until:
1. A real job returns a real result from the app's own code path.
2. The offline case does what the human said it should (test it: stop the bridge).
3. A schema-shaped call is handled for **both** `json` and `text` (schemas are best-effort).
4. Nothing logs the key.

**Report evidence, not conclusions.** "Integrated successfully" is not a result. Paste the
actual job result your app produced.

---

## What you are integrating with

Your app does not talk to an AI CLI. It hands a **job** to a **relay**, and a **bridge** on
someone's machine picks it up and runs it on *their* logged-in subscription.

```
Your app ──POST /v1/jobs──▶ Relay ◀──long-poll── Bridge ──▶ claude / codex / cursor-agent
         ◀─GET /v1/jobs/{id}─┘                (a laptop)      (their subscription)
```

Three consequences shape every design decision below:

1. **The bridge may not be there.** Someone's laptop is asleep. Jobs are not durable in any
   meaningful sense — check first, fail fast.
2. **Jobs are asynchronous.** You enqueue, then fetch. There is no synchronous call.
3. **Relayent never falls back to a paid API.** That is the product guarantee. When no bridge
   is online, your app decides what happens — that decision is yours, not ours.

---

## The five calls you will actually use

| Call | When |
|---|---|
| `GET /v1/bridge/online` | Before enqueuing. Fail fast if nobody is home. |
| `GET /v1/bridge/capabilities` | To discover which backends and models are usable. |
| `POST /v1/jobs` | Run something. |
| `GET /v1/jobs/{id}?wait=1` | Get the result (blocks until ready). |
| `DELETE /v1/jobs/{id}` | You no longer want it. |

The rest of the API (`/v1/jobs/next`, `/v1/jobs/{id}/result`) belongs to the **bridge**. Your
app never calls those.

---

## 1. Authentication

Every request except `/v1/health` needs the pairing key:

```
Authorization: Bearer <pairing-key>
```

**The key is a bearer token: possession is authorization.** Anyone holding it can send prompts
to the paired machine and spend that person's subscription. There is no per-user identity and
no selective revocation — see [SECURITY.md](SECURITY.md).

- `401` — missing *or* wrong key. The API deliberately does not tell you which.
- `429` — too many failed attempts from your IP (see [Limits](#limits-and-timeouts)).

Store it like a password. Never log it, never put it in a URL, never commit it.

---

## 2. Fail fast — is a bridge online?

```bash
curl -s $RELAY/v1/bridge/online -H "Authorization: Bearer $KEY"
# {"online": true}
```

**Call this before enqueuing.** `online: false` means no bridge has polled in the last **40
seconds** — a laptop closed, a bridge stopped. Enqueuing anyway means the job sits until it
expires and your user waits for nothing.

This is the hook for your fallback policy. Relayent will not silently bill a paid API on your
behalf; if that is what you want when offline, your code does it, deliberately.

```python
if not client.bridge_online():
    raise BridgeOffline("No machine is available to run this right now.")
```

---

## 3. Discover what is runnable

```bash
curl -s $RELAY/v1/bridge/capabilities -H "Authorization: Bearer $KEY"
```

```json
{
  "online": true,
  "reported_at": "2026-07-16T22:33:24Z",
  "capabilities": {
    "version": "1.0.0",
    "hostname": "someones-macbook.local",
    "backends": [
      {"name":"claude","installed":true,"supported":true,"ready":true,
       "models":["fable","opus","sonnet","haiku"],"models_probed":false},
      {"name":"cursor","installed":true,"supported":true,"ready":true,
       "models":["auto","gpt-5.3-codex","sonnet-4-thinking"],
       "default_model":"auto","models_probed":true},
      {"name":"gemini","installed":false,"supported":false,"ready":false}
    ]
  }
}
```

### Only trust `ready`

Three flags, and the distinction is not pedantry — it comes from a real bug where a CLI was
installed but its adapter was still a stub, and the UI cheerfully said "installed: yes" for
something that could not run:

| Flag | Means |
|---|---|
| `installed` | The CLI exists on that machine |
| `supported` | Relayent has a working adapter for it |
| **`ready`** | **Both. Jobs will actually run.** ← the only one to branch on |

### `models_probed` is the important one

| | Meaning |
|---|---|
| `models_probed: true` | The list came **from the CLI itself**. Accurate for that install. (`cursor-agent --list-models`.) |
| `models_probed: false` | A **static declaration** in the adapter. A hint that may drift as the CLI changes. |
| `models: []` | The CLI cannot enumerate its models. **This does not mean models are unsupported** — pass a name you know and it works. |

`default_model` is what runs when you omit `model`. Omitting it is usually right; the CLI's
default is normally what the user wants.

**Cache this.** It changes when someone installs a CLI, not per request.

---

## 4. Run a job

```bash
curl -s -XPOST $RELAY/v1/jobs -H "Authorization: Bearer $KEY" -d '{
  "backend": "cursor",
  "model": "auto",
  "system": "You are a terse assistant.",
  "prompt": "Describe the city of Paris.",
  "json_schema": {
    "type": "object",
    "properties": {"city": {"type":"string"}, "country": {"type":"string"}},
    "required": ["city","country"]
  }
}'
# 202 -> {"job_id": "019cbc555e23931529d46dc568cbc5bd"}
```

| Field | Required | Notes |
|---|---|---|
| `backend` | yes | Use one reporting `ready: true` |
| `prompt` | yes | The content prompt |
| `model` | no | From `models[]`. Omit for the backend's default |
| `system` | no | System instruction |
| `json_schema` | no | See below — this is the feature worth using |

`202 Accepted` means queued, **not** run. The job id is how you collect it.

### Structured output

Pass `json_schema` and you get a parsed object back instead of prose. This is the difference
between an integration you can rely on and one that regex-parses English.

It is **best-effort by design**, and honest about it. The CLIs are inconsistent — Claude's
`--json-schema` alone did not reliably shape output across versions — so every adapter also
instructs JSON in-prompt, echoes the schema, and does **one repair retry** on malformed output.
In practice that is reliable; it is not a guarantee. **Always check which field came back:**

```python
r = fetch(job_id)
if "json" in r:   data = r["json"]        # structured, as asked
elif "text" in r: fallback(r["text"])     # the model returned prose anyway
```

---

## 5. Get the result

```bash
curl -s "$RELAY/v1/jobs/$ID?wait=1" -H "Authorization: Bearer $KEY"
```

**`?wait=1` long-polls** — the connection blocks up to **90 seconds** until a result arrives.
That is what you want: no polling loop, no wasted requests, result the instant it exists.

```json
{"id":"019cbc...","status":"done","json":{"city":"Paris","country":"France"}}
```

| `status` | Meaning |
|---|---|
| `pending` | Still queued or running. With `wait=1`, you waited 90s and it is still not done — call again |
| `done` | Success. Read `json` (if you sent a schema) or `text` |
| `error` | Failed. Read `error` |

⚠️ **If your relay is behind a reverse proxy, its read timeout must exceed 90s.** nginx
defaults to **60s** and will return `502`/`504` mid-job — which looks exactly like the bridge
failing, and sends you debugging the wrong component. Set `proxy_read_timeout 120s`. This is
the single most common integration failure.

Without `wait=1` the call returns immediately with whatever the current status is — useful for
a status widget, wasteful as a polling loop.

---

## 6. Cancel a job

```bash
curl -s -X DELETE "$RELAY/v1/jobs/$ID" -H "Authorization: Bearer $KEY"
```

```json
{"id":"019cbc...","cancelled":true,"was_status":"pending",
 "detail":"job was still queued and has been removed; no bridge will run it"}
```

**`was_status` is the honest part.** Read it:

| `was_status` | What actually happened |
|---|---|
| `pending` | It was still queued. **Work genuinely prevented.** |
| `running` | A bridge already claimed it. **The CLI cannot be stopped** — the relay has no channel to an outbound-only bridge. You stop waiting; the quota is already spent. |
| `done` / `error` | Already finished. `cancelled: false` — nothing to cancel. |

Cancelling unblocks anyone waiting in `?wait=1`. `404` means unknown job — or another tenant's,
which reports identically on purpose.

---

## Limits and timeouts

Real numbers, from the code:

| Limit | Value | What it means for you |
|---|---|---|
| **Job TTL** | **10 minutes** | A result not collected within 10 min of enqueue is discarded. Fetch it. |
| Result long-poll | 90s | `?wait=1` blocks this long, then returns `pending`. Call again. |
| Bridge presence window | 40s | `online: false` = no poll in 40s |
| **Enqueue rate** | **1/sec sustained, burst 30** | Per pairing key. Exceed it → `429` |
| Failed auth | 1/5s, burst 8 | Per IP. Exceed it → `429` |
| Request body | 1 MiB | Big prompts → `400` |

**On `429`**: back off. The enqueue limit is per *key*, so it protects a real person's
subscription quota from your app. It is not a suggestion.

**On the 10-minute TTL**: it is why `bridge/online` matters. A job enqueued to an offline
bridge is dead in 10 minutes and nobody is told.

---

## Errors

Uniform envelope:

```json
{"error": "backend and prompt are required"}
```

| Code | Meaning | Do |
|---|---|---|
| `400` | Malformed body, missing field, body >1 MiB | Fix the request |
| `401` | Missing or wrong key | Check the key. Do not retry in a loop |
| `404` | Unknown job (or another tenant's) | Check the id |
| `429` | Rate limited | Back off |

**Job failures are not HTTP errors.** A job that ran and failed returns `200` with
`status: "error"` — the *request* succeeded. Check `status`, not just the HTTP code:

```json
{"id":"...","status":"error","error":"backend \"gemini\" is not supported yet by this bridge (adapter is a stub)"}
```

---

## A complete integration

```python
import requests

class Relayent:
    def __init__(self, relay_url, pairing_key):
        self.url = relay_url.rstrip("/")
        self.h = {"Authorization": f"Bearer {pairing_key}"}

    def online(self) -> bool:
        r = requests.get(f"{self.url}/v1/bridge/online", headers=self.h, timeout=10)
        r.raise_for_status()
        return r.json()["online"]

    def ready_backends(self) -> dict:
        """{name: [models]} for backends that can actually run. Cache this."""
        r = requests.get(f"{self.url}/v1/bridge/capabilities", headers=self.h, timeout=10)
        r.raise_for_status()
        caps = r.json().get("capabilities") or {}
        return {b["name"]: b.get("models", [])
                for b in (caps.get("backends") or []) if b["ready"]}

    def run(self, backend, prompt, model=None, system=None, json_schema=None, timeout=300):
        # Fail fast rather than enqueue into the void — the job would just expire.
        if not self.online():
            raise RuntimeError("No bridge is online; refusing to enqueue.")

        body = {"backend": backend, "prompt": prompt}
        for k, v in (("model", model), ("system", system), ("json_schema", json_schema)):
            if v:
                body[k] = v

        r = requests.post(f"{self.url}/v1/jobs", headers=self.h, json=body, timeout=15)
        if r.status_code == 429:
            raise RuntimeError("Rate limited — back off.")
        r.raise_for_status()
        job_id = r.json()["job_id"]

        # Long-poll. Each call blocks up to 90s server-side; loop until our own deadline.
        import time
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            r = requests.get(f"{self.url}/v1/jobs/{job_id}?wait=1",
                             headers=self.h, timeout=120)   # MUST exceed the server's 90s
            r.raise_for_status()
            res = r.json()
            if res["status"] == "done":
                return res.get("json", res.get("text"))     # schema honoured? or prose?
            if res["status"] == "error":
                raise RuntimeError(f"Job failed: {res.get('error')}")

        # Do not leave it queued — it would run and burn quota nobody is waiting for.
        requests.delete(f"{self.url}/v1/jobs/{job_id}", headers=self.h, timeout=10)
        raise TimeoutError(f"Job {job_id} exceeded {timeout}s; cancelled.")
```

---

## Verifying your integration

Do not report success without these. Each is one command; each catches a real failure.

```bash
RELAY=https://relay.example.com; KEY=<your key>

# 1. Auth works, and the relay is what you think it is.
curl -s $RELAY/v1/status -H "Authorization: Bearer $KEY"
#    -> "tls":true (unless localhost). "tls":false over https = misconfigured proxy.

# 2. A bridge is actually there.
curl -s $RELAY/v1/bridge/online -H "Authorization: Bearer $KEY"     # {"online":true}

# 3. Your backend is READY (not merely installed).
curl -s $RELAY/v1/bridge/capabilities -H "Authorization: Bearer $KEY"

# 4. A real job, end to end, from YOUR app's code path — not curl.
#    Paste the actual result.

# 5. The offline path. Stop the bridge, then run your feature:
#    relayent-bridge uninstall      (on the bridge machine)
#    Your app must do what the human specified — not hang, not silently bill an API.

# 6. The long-poll survives your proxy, if you have one. With no bridge running:
time curl -s -o /dev/null -w '%{http_code} after %{time_total}s\n' \
  "$RELAY/v1/jobs/$(curl -s -XPOST $RELAY/v1/jobs -H "Authorization: Bearer $KEY" \
     -d '{"backend":"cursor","prompt":"x"}' | sed -n 's/.*"job_id":"\([^"]*\)".*/\1/p')?wait=1" \
  -H "Authorization: Bearer $KEY"
#    PASS: 200 after ~90s.   FAIL: 502/504 after ~60s -> raise proxy_read_timeout to 120s.
```

**These numbers are measured, not estimated.** The long-poll returns `200 after 90.147s`
against a live relay behind nginx; `online` flips to false within 40s of a bridge stopping; a
failed job returns HTTP `200` with `status:"error"`.

## Integration checklist

- [ ] Key stored as a secret — not in code, not in a URL, not in logs
- [ ] `bridge/online` checked before enqueuing, with a deliberate offline policy
- [ ] Backends chosen on **`ready`**, never `installed`
- [ ] `?wait=1` used instead of a polling loop
- [ ] Client HTTP timeout **>90s** on the result fetch
- [ ] Any reverse proxy set to `proxy_read_timeout 120s` or more
- [ ] Both `json` and `text` handled — schemas are best-effort
- [ ] `status: "error"` checked on `200` responses
- [ ] `429` backed off, not retried tightly
- [ ] Results fetched inside the 10-minute TTL
- [ ] Abandoned jobs cancelled rather than left to run

---

## See also

- [`openapi.yaml`](openapi.yaml) — the contract. Generate clients from this.
- [SECURITY.md](SECURITY.md) — the threat model, and **what Relayent does not protect against**.
- [INSTALL.md](INSTALL.md) — standing up a relay and a bridge.
- [README.md](README.md) — what Relayent is and why.
