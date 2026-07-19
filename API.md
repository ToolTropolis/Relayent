# Relayent API Reference

The `/v1` HTTP API. [`openapi.yaml`](openapi.yaml) is the machine-readable contract; this is
the human reference.

- **Base URL:** your relay's URL (e.g. `https://relay.example.com`)
- **Auth:** `Authorization: Bearer <pairing-key>` on every request except `/v1/health`
- **Format:** JSON request and response bodies
- **Client:** any HTTP library. `clients/python/` is optional.

New to Relayent? Read [Concepts](#concepts) first. Integrating? See the
[integration guide](#integration-guide) and [checklist](#checklist).

---

## Endpoints

| Method | Path | Purpose | Caller |
|---|---|---|---|
| `GET` | `/v1/health` | Liveness check | anyone |
| `GET` | `/v1/status` | Relay health and posture | app |
| `GET` | `/v1/bridge/online` | Is a bridge available? | app |
| `GET` | `/v1/bridge/capabilities` | Available backends and models | app |
| `POST` | `/v1/jobs` | Enqueue a job | app |
| `GET` | `/v1/jobs/{id}` | Fetch a job's result | app |
| `DELETE` | `/v1/jobs/{id}` | Cancel a job | app |
| `GET` | `/v1/jobs/next` | Claim the next job | bridge only |
| `POST` | `/v1/jobs/{id}/result` | Post a result | bridge only |

Apps use the first seven. The last two belong to the bridge daemon.

---

## Reference

### GET /v1/health

Liveness check. No authentication.

**Request**
```
GET /v1/health
```

**Response** `200`
```json
{"status": "ok"}
```

---

### GET /v1/status

Relay health and security posture for the caller's key.

**Request**
```
GET /v1/status
Authorization: Bearer <key>
```

**Response** `200`
```json
{
  "status": "ok",
  "version": "1.0.0",
  "uptime_seconds": 71,
  "bridge_online": false,
  "pending_jobs": 0,
  "require_pairing": true,
  "key_fingerprint": "fOw2tuTQ",
  "tls": true,
  "network_reachable": true
}
```

| Field | Type | Description |
|---|---|---|
| `tls` | boolean | Whether the request arrived encrypted. `false` over HTTPS = misconfigured proxy. |
| `key_fingerprint` | string | 8-char hash of your key. Never the key itself. |
| `bridge_online` | boolean | Is a bridge polling for your key? |
| `pending_jobs` | integer | Jobs queued for your key. |

**Errors:** `401`

---

### GET /v1/bridge/online

Whether a bridge is currently available for your key. Call before enqueuing.

**Request**
```
GET /v1/bridge/online
Authorization: Bearer <key>
```

**Response** `200`
```json
{"online": true}
```

`false` means no bridge has polled in the last 40 seconds.

**Errors:** `401`

---

### GET /v1/bridge/capabilities

Which backends and models the caller's bridge can run.

**Request**
```
GET /v1/bridge/capabilities
Authorization: Bearer <key>
```

**Response** `200`
```json
{
  "online": true,
  "reported_at": "2026-07-16T22:33:24Z",
  "capabilities": {
    "version": "1.0.0",
    "hostname": "someones-macbook.local",
    "backends": [
      {"name": "cursor", "installed": true, "supported": true, "ready": true,
       "models": ["auto", "gpt-5.3-codex"], "default_model": "auto", "models_probed": true},
      {"name": "gemini", "installed": false, "supported": false, "ready": false}
    ]
  }
}
```

| Field | Type | Description |
|---|---|---|
| `installed` | boolean | The CLI exists on the bridge host. |
| `supported` | boolean | Relayent has a working adapter. |
| `ready` | boolean | Both true — jobs will run. **Branch on this one.** |
| `models` | string[] | Values accepted as `model`. Empty = undiscoverable, not unsupported. |
| `default_model` | string | Used when `model` is omitted. |
| `models_probed` | boolean | `true` = list came from the CLI (accurate). `false` = a static hint that may drift. |

`online` is `false` and `backends` empty when no bridge has reported recently.

**Errors:** `401`

---

### POST /v1/jobs

Enqueue a job. Returns immediately; the result is fetched separately.

**Request**
```
POST /v1/jobs
Authorization: Bearer <key>
Content-Type: application/json
```

| Parameter | Type | Required | Description |
|---|---|---|---|
| `backend` | string | yes | `claude` \| `codex` \| `cursor` \| `gemini`. Use one reporting `ready: true`. |
| `prompt` | string | yes | The content prompt. |
| `model` | string | no | A value from `models[]`. Omit for the backend's default. |
| `system` | string | no | System instruction. |
| `json_schema` | object | no | JSON Schema. When set, the response is a parsed `json` object (best-effort). |

```json
{
  "backend": "cursor",
  "model": "auto",
  "prompt": "Describe the city of Paris.",
  "json_schema": {
    "type": "object",
    "properties": {"city": {"type": "string"}, "country": {"type": "string"}},
    "required": ["city", "country"]
  }
}
```

**Response** `202`
```json
{"job_id": "019cbc555e23931529d46dc568cbc5bd"}
```

**Errors:** `400` (missing field, or body > 1 MiB), `401`, `429` (rate limited)

---

### GET /v1/jobs/{id}

Fetch a job's status and result.

**Request**
```
GET /v1/jobs/{id}?wait=1
Authorization: Bearer <key>
```

| Parameter | In | Description |
|---|---|---|
| `id` | path | The `job_id` from `POST /v1/jobs`. |
| `wait` | query | `1` long-polls up to 90s until the result is ready. Omit to return immediately. |

**Response** `200`
```json
{"id": "019cbc...", "status": "done", "json": {"city": "Paris", "country": "France"}}
```

| `status` | Meaning | Read |
|---|---|---|
| `pending` | Queued or running (or `wait=1` timed out after 90s — call again) | — |
| `done` | Succeeded | `json` (if a schema was sent) or `text` |
| `error` | Failed | `error` |

**Errors:** `401`, `404` (unknown job for this key)

---

### DELETE /v1/jobs/{id}

Cancel a job. Unblocks anyone waiting in `GET /v1/jobs/{id}?wait=1`.

**Request**
```
DELETE /v1/jobs/{id}
Authorization: Bearer <key>
```

**Response** `200`
```json
{"id": "019cbc...", "cancelled": true, "was_status": "pending",
 "detail": "job was still queued and has been removed; no bridge will run it"}
```

| `was_status` | Meaning |
|---|---|
| `pending` | Was still queued. Work prevented. |
| `running` | A bridge already claimed it. The CLI cannot be stopped; quota is already spent. |
| `done` / `error` | Already finished. `cancelled` is `false`. |

**Errors:** `401`, `404`

---

## Admin & multi-tenant

Everything above is the single integration surface and works in **both** deployment models.
When the relay runs **multi-tenant** (`RELAYENT_DATA_DIR` + usually OIDC), it adds per-user
identity, isolation, and an operator surface. As an app integrator you need only one extra
thing: authenticate with an **app credential** and name the **`target_user`** on each
`POST /v1/jobs` (see below). The rest of this section is for the **operator** running the relay.

### For apps: routing to a specific user

An app credential is issued by an admin and looks like `"<id>.<secret>"`. Use it exactly like a
pairing key (`Authorization: Bearer <id>.<secret>`), and add `target_user` — the user whose
bridge/subscription should run the job:

```json
{"backend": "cursor", "prompt": "…", "target_user": "alice"}
```

The job runs on **alice's** subscription. A different `target_user` routes to that user instead.
Omitting it with an app credential is a `400`. (A bridge or legacy pairing-key caller routes to
itself and ignores `target_user`.)

**Reading back is also per-user.** A job lives in the target user's namespace, so an app must
pass the **same `target_user`** as a query parameter on the read-side calls too — otherwise the
lookup runs under the app's own identity and 404s:

```
GET    /v1/jobs/{id}?wait=1&target_user=alice
DELETE /v1/jobs/{id}?target_user=alice
GET    /v1/bridge/online?target_user=alice
```

For a bridge or legacy caller these are self-scoped and the parameter is optional (naming a
different user is rejected).

### Signing in (operators)

Humans use the **`/login`** page — an HTML page, not a JSON endpoint. It offers OIDC sign-in
("Sign in with Google", or your configured provider) and a bootstrap-admin-token field. On
success the browser is routed **by role**: an admin lands on the **`/admin`** console, a regular
user on **`/`** (their own status page). The **first user ever to sign in becomes the admin**;
everyone after is a regular user until an admin promotes them.

### Admin API (`/v1/admin/*`)

Every route requires the admin scope (an OIDC admin session, or the bootstrap `RELAYENT_ADMIN_TOKEN`
as a bearer). None of these ever return prompt or result content.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/v1/admin/users` | List users + per-user activity (no content) |
| `POST` | `/v1/admin/users` | Pre-provision a user |
| `POST` | `/v1/admin/users/{sub}/role` | Grant/revoke admin — `{"role":"admin"\|"user"}` |
| `POST` | `/v1/admin/users/{sub}/disabled` | Disable/re-enable (`?disabled=true\|false`) |
| `DELETE` | `/v1/admin/users/{sub}` | Delete a user record |
| `POST` | `/v1/admin/enroll-tokens` | Mint a one-time bridge-enrollment token |
| `GET` | `/v1/admin/app-creds` | List app credentials (no secrets) |
| `POST` | `/v1/admin/app-creds` | Issue an app credential (secret shown once) |
| `POST` | `/v1/admin/app-creds/{id}/revoke` | Revoke an app credential |
| `GET` | `/v1/admin/audit` | Recent activity — who/when/backend/status/bytes, no content |
| `GET` | `/v1/admin/config` | Effective relay config — no secret values |
| `GET` | `/v1/admin/backends` | List backends and whether each is enabled |
| `POST` | `/v1/admin/backends/{name}` | Enable/disable a backend — `{"enabled":true\|false}` |

An admin cannot demote or delete **themselves** (last-admin lockout guard). `GET /v1/admin/config`
returns only non-secret values plus booleans for whether the pairing key / admin token are set;
config itself is env/compose-driven (change `.env`, then `docker compose up -d`).

**Backend exposure policy.** An admin can disable a backend globally. A disabled backend is
**omitted** from `GET /v1/bridge/capabilities` and **refused** at `POST /v1/jobs` with `403`, for
every caller — so an integrating app should treat "not present in capabilities" as "not available",
and be ready for a `403` if it enqueues a backend that was disabled after discovery. This is how a
public surface (e.g. the demo) is kept to a safe subset like `cursor` only.

Bridges obtain their own credential by redeeming an enrollment token at **`POST /v1/enroll`** — see
[INSTALL.md](INSTALL.md#multi-user-multi-tenant-mode). Full request/response detail for every
endpoint is in [`openapi.yaml`](openapi.yaml).

---

## Concepts

### Architecture

Your app hands a **job** to a **relay**. A **bridge** on a user's machine long-polls the relay,
runs the job on the user's logged-in CLI subscription, and posts the result back.

```
Your app ──POST /v1/jobs──▶ Relay ◀──poll── Bridge ──▶ claude / codex / cursor-agent
         ◀─GET /v1/jobs/{id}─┘            (a laptop)      (their subscription)
```

- Jobs are **asynchronous**: enqueue, then fetch.
- The bridge **may be offline**. Check `/v1/bridge/online` first.
- Relayent **never falls back to a paid API**. Your app decides what to do when offline.

### Authentication

Every request except `/v1/health` requires a bearer token:

```
Authorization: Bearer <token>
```

What the token is depends on the deployment model:

- **Single-key:** one shared **pairing key**. Possession is authorization; there is no per-user
  identity and no selective revocation.
- **Multi-tenant:** an **app credential** (`<id>.<secret>`, issued by an admin, names `target_user`
  per job) or a **bridge credential** (obtained via enrollment). Humans use OIDC sessions from
  `/login`, or the bootstrap admin token. Credentials are scoped and individually revocable, and
  every job is isolated to one user. See [Admin & multi-tenant](#admin--multi-tenant).

In all cases the token is a secret: never log it, embed it in a URL, or commit it. See
[SECURITY.md](SECURITY.md).

### Status codes

| Code | Meaning |
|---|---|
| `200` | Success. **A failed job also returns `200`** with `status: "error"` — check the body. |
| `202` | Job accepted (enqueued, not yet run). |
| `400` | Malformed request, missing field, or body > 1 MiB. |
| `401` | Missing or wrong key (indistinguishable, by design). |
| `404` | Unknown job for this key. |
| `429` | Rate limited. Back off. |

Error bodies use one envelope:
```json
{"error": "backend and prompt are required"}
```

### Rate limits and timeouts

| Limit | Value |
|---|---|
| Job TTL | 10 minutes (uncollected results are discarded) |
| Result long-poll (`wait=1`) | 90 seconds per call, then returns `pending` |
| Bridge presence window | 40 seconds |
| Enqueue rate | 1/sec sustained, burst 30, per key |
| Failed-auth rate | 1/5s, burst 8, per IP |
| Request body | 1 MiB |

### Structured output

`json_schema` requests a parsed object instead of prose. It is **best-effort**: the adapters
instruct JSON in-prompt and repair once, but a model may still return prose. Handle both:

```python
if "json" in result: use(result["json"])
else:                 fallback(result["text"])
```

### Reverse proxies

`GET /v1/jobs/{id}?wait=1` holds the connection up to 90 seconds. A proxy with a shorter read
timeout returns `502`/`504` mid-job, which looks like a bridge failure. Set
`proxy_read_timeout` to at least `120s`. This is the most common integration error.

---

## Integration guide

### Before you write code

| Establish | Notes |
|---|---|
| Relay URL | No default. `https://` required off-localhost. |
| Pairing key | A credential. Read it from config/env; never hard-code or commit it. |
| Backend | Pick one reporting `ready: true` from `/v1/bridge/capabilities`. |
| **Offline policy** | When no bridge is online, does the feature degrade, queue, error, or fall back to a paid API? **A product decision — confirm with a human.** |

### Where it goes

Relayent is a provider, not a framework. If the app has one AI-dispatch function, this is one
branch there. If provider logic is duplicated across call sites, it is one branch per site —
grep for the existing provider names to find them.

### Reference client

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
        r = requests.get(f"{self.url}/v1/bridge/capabilities", headers=self.h, timeout=10)
        r.raise_for_status()
        caps = r.json().get("capabilities") or {}
        return {b["name"]: b.get("models", [])
                for b in (caps.get("backends") or []) if b["ready"]}

    def run(self, backend, prompt, model=None, system=None, json_schema=None, timeout=300):
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

        import time
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            r = requests.get(f"{self.url}/v1/jobs/{job_id}?wait=1",
                             headers=self.h, timeout=120)   # must exceed the server's 90s
            r.raise_for_status()
            res = r.json()
            if res["status"] == "done":
                return res.get("json", res.get("text"))
            if res["status"] == "error":
                raise RuntimeError(f"Job failed: {res.get('error')}")

        requests.delete(f"{self.url}/v1/jobs/{job_id}", headers=self.h, timeout=10)
        raise TimeoutError(f"Job {job_id} exceeded {timeout}s; cancelled.")
```

---

## Checklist

- [ ] Key stored as a secret — not in code, a URL, or logs
- [ ] `/v1/bridge/online` checked before enqueuing, with a defined offline policy
- [ ] Backend chosen on `ready`, not `installed`
- [ ] `?wait=1` used instead of a polling loop
- [ ] HTTP client timeout > 90s on the result fetch
- [ ] Reverse proxy set to `proxy_read_timeout 120s` or more
- [ ] Both `json` and `text` handled (schemas are best-effort)
- [ ] `status: "error"` checked on `200` responses
- [ ] `429` backed off, not retried tightly
- [ ] Results fetched within the 10-minute TTL

---

## See also

- [`openapi.yaml`](openapi.yaml) — machine-readable contract; generate clients from it
- [SECURITY.md](SECURITY.md) — threat model and limitations
- [INSTALL.md](INSTALL.md) — standing up a relay and bridge
