# Multi-tenant build — checkpoints & rollback

This is the largest change to Relayent to date (see
`~/.claude/plans/are-we-talking-about-abstract-bunny.md`). It is built in phases, each an
atomic, individually revertible checkpoint. **This file is the rollback ledger** — do not skip
the "Reverting costs" column, because some phases add state (a DB, a volume) that a code revert
alone does not undo.

**Branch:** `feature/multi-tenant-principal` — `main` stays production-safe throughout.

## How to revert

```bash
# Inspect the checkpoints:
git tag -n5 | grep phase

# Roll the working branch back to the end of a phase (discards later phases):
git reset --hard phase-N-<name>

# Or just look at one without moving:
git checkout phase-N-<name>
```

⚠️ **A git revert restores code, not data.** Where a phase adds durable state, its row below
says exactly what else to clean up. Reverting the code without cleaning the state can leave the
relay reading a schema its code no longer understands.

## Checkpoints

| Phase | Tag | Adds | Reverting costs / cleanup |
|---|---|---|---|
| 1 | `phase-1-principal` | `Principal` type; Queue re-keyed `key`→`userID`; auth middleware → `*Principal`. Pure refactor. | **Nothing to clean up.** Zero behaviour change, no state, no new deps. `git reset --hard main` fully undoes it. |
| 2 | `phase-2-store` ✅ | **bbolt** control-plane store (`go.etcd.io/bbolt` — NOT sqlite; see note); opt-in via `RELAYENT_DATA_DIR`. | Code revert is safe (`git reset --hard phase-1-principal`). **Also** `go mod tidy` to drop bbolt, and delete `$RELAYENT_DATA_DIR` / the `relay_data` volume. Nothing in production reads it (nil store in legacy mode), so no data-loss risk. |
| 3 | `phase-3-oidc` ✅ | OIDC login (`go-oidc` + `oauth2`), tamper-evident session cookie, first-login-is-admin bootstrap. | Code revert safe (`git reset --hard phase-2-store`). `go mod tidy` drops go-oidc/oauth2. Remove `RELAYENT_OIDC_*` env. No schema change — reuses phase 2's users bucket. |
| 4 | `phase-4-machine-auth` ✅ | Hashed bridge + app credentials (`<id>.<secret>`); one-time enrollment tokens. | Code revert safe (`git reset --hard phase-3-oidc`). Orphaned `app_creds`/`bridge_bindings` buckets are harmless, or clear the data dir. No dep change. |
| 5 | `phase-5-enroll` ✅ | `POST /v1/enroll` (one-time token → bridge credential); `setup` accepts a token or a key. | Code revert safe (`git reset --hard phase-4-machine-auth`). Already-issued bridge credentials stop being accepted; re-run `setup` with the legacy key. |
| 6 | `phase-6-target-user` ✅ | `EnqueueRequest.TargetUser` + `routeTarget` (per-user routing, anti-spoof guard). | Code revert safe (`git reset --hard phase-5-enroll`). Additive field; old clients omit it and self-route. |
| 7 | `phase-7-admin` ✅ | `/v1/admin/*` API (users, enroll-tokens, app-creds, activity) + bootstrap admin token. HTML dashboard deferred. | Code revert safe (`git reset --hard phase-6-target-user`). Remove `RELAYENT_ADMIN_TOKEN`. No new persisted state. |
| 8 | `phase-8-audit` *(pending)* | Audit table + no-content boundary. | Code revert safe. `audit` table becomes orphaned; drop it if desired. |
| 9 | `phase-9-compat` *(pending)* | Legacy migration docs; SECURITY.md posture update. | Docs only. Trivially revertible. |

## Note: SQLite → bbolt (phase 2)

The plan named `modernc.org/sqlite` (pure-Go SQLite). It was tried and abandoned: its pure-Go
libc emulation blocked indefinitely in the build/test environment (a 1-second test took 10
minutes wall-clock). Switched to `go.etcd.io/bbolt` — also pure Go, no cgo/libc, battle-tested
(etcd's storage engine), and the same tests run in ~1s. The store interface and every security
property (hashes only, no content at rest) are unchanged; only the engine differs.

## Invariant held at every checkpoint

The **legacy single-key path keeps working** at every phase, so the live production deployment
(`relayent.ignorelist.com`, single `RELAYENT_PAIRING_KEY`) is never broken by an in-progress
phase. Multi-tenant features are additive and opt-in until phase 9's migration.

## Full abandonment

If the whole line of work is dropped: `main` (`e049f6c` at branch start) is the untouched,
production-shipping state. Deleting the `feature/multi-tenant-principal` branch and any
`relay_data` volume returns everything to pre-build.
