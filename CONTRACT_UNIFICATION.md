# REST Contract Unification: niova-mdsvc ↔ mdsvc-tidb

Goal: converge the two control-plane REST contracts onto a single request/response
shape so a data-path client can talk to either service unchanged.

This document is the field-level reference for that work:

1. [Endpoint support matrix](#1-endpoint-support-matrix)
2. [Shared endpoints — request fields + response payloads + inconsistencies](#2-shared-endpoints)
3. [niova-mdsvc-only endpoints](#3-niova-mdsvc-only-endpoints)
4. [mdsvc-tidb-only endpoints](#4-mdsvc-tidb-only-endpoints)
5. [Cross-cutting inconsistencies (summary)](#5-cross-cutting-inconsistencies-summary)
6. [Proposed response-envelope designs](#6-proposed-response-envelope-designs)
7. [Proposed HTTP status-code designs](#7-proposed-http-status-code-designs)

Legend for request transport: **P** = URL path segment, **Q** = query param,
**B** = JSON body field, **H** = HTTP header.

Top-level shapes:

- **niova-mdsvc** — **IMPLEMENTED**: the unified envelope
  `APIResponse[T]{ success: bool, error?: string, payload?: T }` (Design A, §6).
  HTTP-code policy (Design 1, §7) is live: **data endpoints never emit an HTTP
  error code** — a method error is `HTTP 200 {success:false, error}` — while the
  **user/auth endpoints keep real HTTP codes** (401/400/409/404). Writes require
  the `X-RNCUI` header.
- **mdsvc-tidb** — **not yet ported**: every response still embeds
  `BaseResponse{ status: "success"|"failure", ncp_status_code: "NCP_*", message? }`
  plus inline fields; domain errors map to HTTP 4xx/5xx + an `ncp_status_code`.

> **Implementation status (niova):** the envelope, the HTTP-code policy, and the
> payload-shape alignments below (vdev `name`/`failure_domain`/`pfs_id` echoes,
> user `account_status`, snake_case `mount_vdev`, and `GET /api/resource`'s generic
> `resources` array) are all done. Still open: porting the whole envelope to
> mdsvc-tidb.

---

## 1. Endpoint support matrix

| Endpoint | Action | niova | tidb |
|---|---|:--:|:--:|
| `POST /api/vdev` | Create vdev + allocate chunks | ✅ | ✅ |
| `GET /api/vdev` | Get vdev metadata | ✅ | ✅ |
| `DELETE /api/vdev/{id}` | Delete vdev | ✅ | ✅ |
| `GET /api/nisd` | NISD connection info | ✅ | ✅ |
| `GET /api/chunk` | Placement of one chunk | ✅ | ✅ |
| `GET /api/resource` | List/get infra entity by type | ✅ | ✅ |
| `PUT /api/resource` | Upsert one infra entity | ✅ | ✅ |
| `GET /api/infra` | Full infra tree | ✅ | ✅ |
| `POST /api/pfs` | Create PFS | ✅ | ✅ |
| `GET /api/pfs` | Get/list PFS | ✅ | ✅ |
| `POST /users/login` | Login (JWT) | ✅ | ✅ |
| `POST /api/users` | Create user | ✅ | ✅ |
| `GET /api/users` | List users | ✅ | ✅ |
| `GET /api/users/{id}` | Get user | ✅ | ✅ |
| `PUT /api/users/{id}` | Update user | ✅ | ✅ |
| `POST /api/mount_vdev` | Mount vdev, mint access token | ✅ | ❌ |
| `POST /api/snap` | Create snapshot | ✅ | ❌ |
| `POST /api/nisd_args` | Set NISD launch args (singleton) | ✅ | ❌ |
| `GET /api/nisd_args` | Get NISD launch args | ✅ | ❌ |
| `POST /users/admin` | Bootstrap singleton admin | ✅ | ❌ |
| `POST /api/infra` | Atomic create-infra | ❌ | ✅ |
| `GET /api/chunks` | Paginated chunk map | ❌ (DTO exists, route deferred) | ✅ |
| `POST /api/chunk/reassign` | Reassign failed replica | ❌ | ✅ |
| `POST /api/reset` | Wipe CP state | ❌ | ✅ |
| `GET/POST/DELETE /api/authz/rbac…` | RBAC policy admin | ❌ | ✅ |
| `GET/POST/DELETE /api/authz/abac…` | ABAC policy admin | ❌ | ✅ |

**15 shared**, **5 niova-only**, **7 tidb-only** (counting authz as 2).

---

## 2. Shared endpoints

For each: request fields (with transport) and response payload on both sides.
⚠️ marks an inconsistency to resolve during unification.

### `POST /api/vdev` — create vdev + allocate chunk map

| Field | Transport | niova | tidb |
|---|:--:|:--:|:--:|
| `size_bytes` | B | ✅ | ✅ |
| `num_replicas` | B | ✅ | ✅ |
| `failure_domain` | B | ✅ | ✅ |
| `entity_ids[]` | B | ✅ (⚠️ only `[0]` applied) | ✅ |
| `pfs` | B | ✅ | ✅ |
| `name` | B | ✅ (niova extension) | ✅ (via `BaseRequest`) |
| `X-RNCUI` | H | ✅ required | ❌ n/a |

Response payload (`payload` under the envelope on niova):

| niova | tidb |
|---|---|
| `vdev_id`, `name`, `num_chunks`, `failure_domain`, `pfs_id` | `…BaseResponse`, `vdev_id`, `name`, `num_chunks`, `failure_domain`, `pfs_id` |

✅ **Resolved:** niova now echoes `name`, `failure_domain` (normalized level), and
`pfs_id` (populated when the PFS was supplied as a UUID). Field sets match.

### `GET /api/vdev` — vdev metadata

| Field | Transport | niova | tidb |
|---|:--:|:--:|:--:|
| `id` | Q | ✅ (⚠️ resolves UUID **or name**) | ✅ (UUID) |

Response payload:

| niova | tidb |
|---|---|
| `id`, `name`, `size`, `num_chunks`, `num_replicas`, `failure_domain` | `…BaseResponse`, `id`, `name`, `size`, `num_chunks`, `num_replicas`, `failure_domain` |

✅ **Resolved:** niova now returns `name` + `failure_domain`. Field sets match.
Note niova still accepts a name in `?id=` (UUID or name); tidb accepts UUID only —
a superset, not a conflict.

### `DELETE /api/vdev/{id}` — delete vdev

| Field | Transport | niova | tidb |
|---|:--:|:--:|:--:|
| `id` | P | ✅ | ✅ |
| `X-RNCUI` | H | ✅ required | ❌ |

Response:

| niova | tidb |
|---|---|
| `WriteResponse{ success, id?, message?, error? }` | `BaseResponse` |

### `GET /api/nisd` — NISD connection info

| Field | Transport | niova | tidb |
|---|:--:|:--:|:--:|
| `id` | Q | ✅ | ✅ |

Response (fields align):

| niova | tidb |
|---|---|
| `success`, `id`, `peer_port`, `net_info[{ip_addr,port}]`, `error?` | `…BaseResponse`, `id`, `peer_port`, `net_info[{ip_addr,port}]` |

✅ Payload fields identical apart from the envelope.

### `GET /api/chunk` — placement of one chunk

| Field | Transport | niova | tidb |
|---|:--:|:--:|:--:|
| `vdev_id` | Q | ✅ | ✅ |
| `chunk_idx` | Q | ✅ | ✅ |

Response (fields align):

| niova | tidb |
|---|---|
| `success`, `vdev_id`, `chunk_idx`, `nisd_ids[]`, `error?` | `…BaseResponse`, `vdev_id`, `chunk_idx`, `nisd_ids[]` |

✅ `nisd_ids` is one per replica in replica-idx order on both.

### `GET /api/resource` — list/get infra entities by type

| Field | Transport | niova | tidb |
|---|:--:|:--:|:--:|
| `type` | Q | ✅ | ✅ |
| `id` | Q | ✅ (single-fetch) | ⚠️ not supported |

Response payload:

| niova | tidb |
|---|---|
| `type`, **generic** `resources` (array of flat entity objects) | `…BaseResponse`, `type`, **generic** `resources` (array) |

✅ **Resolved:** niova now returns a single generic `resources` array (matching
TiDB), not typed per-type slices; all elements are of the requested `type`. niova
keeps two non-conflicting extras: the **`partition`** type (tidb lacks it) and
single-fetch via `?id=` — both carried in the same generic array.

### `PUT /api/resource` — upsert one entity

| Field | Transport | niova | tidb |
|---|:--:|:--:|:--:|
| `type` | Q | ✅ | ✅ |
| entity object | B | ✅ | ✅ |
| `X-RNCUI` | H | ✅ required | ❌ |

Supported types — niova: `pdu`/`rack`/`hypervisor`/`device`/`nisd`/**`partition`**;
tidb: `pdu`/`rack`/`hypervisor`/`device`/`nisd`.

Response:

| niova | tidb |
|---|---|
| `WriteResponse{ success, id, … }` | `BaseResponse` |

⚠️ niova returns the upserted `id`; tidb returns only status.

### `GET /api/infra` — full PDU→Rack→Hypervisor→Device→NISD tree

No request params.

Response:

| niova | tidb |
|---|---|
| `success`, `infra{ pdus[ …nested… ] }`, `error?` | `…BaseResponse`, `infra{ pdus[ …nested… ] }` |

✅ Same nested shape (niova assembles it proxy-side from `GET_ALL_RESOURCES`).

### `POST /api/pfs` — create PFS

| Field | Transport | niova | tidb |
|---|:--:|:--:|:--:|
| `name` | B | ✅ | ✅ |
| `id` | B | ✅ (optional; server mints if empty) | ⚠️ not accepted |
| `offset` | B | ✅ | ⚠️ not accepted |
| `vdev_ids[]` | B | ✅ | ⚠️ not accepted |
| `X-RNCUI` | H | ✅ required | ❌ |

Response:

| niova | tidb |
|---|---|
| `WriteResponse{ success, id, … }` | `…BaseResponse`, `pfs_id`, `name` |

⚠️ **Request asymmetry:** niova accepts the full `PFS` object; tidb accepts `name`
only. ⚠️ **Response key:** niova returns `id`, tidb returns `pfs_id`.

### `GET /api/pfs` — get/list PFS

| Field | Transport | niova | tidb |
|---|:--:|:--:|:--:|
| `id` | Q | ✅ (optional) | ✅ |

Response:

| niova | tidb |
|---|---|
| `success`, **`pfs[ {id,name,offset,vdev_ids[]} ]`** (list), `error?` | `…BaseResponse`, `id`, `name`, `offset`, `vdev_ids[]` (single object) |

⚠️ **Cardinality mismatch:** niova always returns a list; tidb returns a single
object. Unify on list (single `?id=` → 1-element list).

### `POST /users/login` — authenticate, return JWT

| Field | Transport | niova | tidb |
|---|:--:|:--:|:--:|
| `username` | B | ✅ | ✅ |
| `password` | B | ✅ (⚠️ carries the niova **secret key**) | ✅ (password) |

Response:

| niova | tidb |
|---|---|
| `success`, `access_token`, `token_type`, `expires_in`, `user_id`, `username`, `role`, `is_admin`, `error?` | `…BaseResponse`, `access_token`, `expires_in` |

⚠️ niova returns a much richer login payload (`token_type`, `user_id`, `username`,
`role`, `is_admin`); tidb returns `access_token` + `expires_in` only.

### `POST /api/users` — create user

| Field | Transport | niova | tidb |
|---|:--:|:--:|:--:|
| `username` | B | ✅ | ✅ |
| `role` | B | ✅ | ✅ |
| `password` | B | ⚠️ not used (secret key is generated) | ✅ required |
| `X-RNCUI` | H | ✅ required | ❌ |

Response:

| niova payload | tidb |
|---|---|
| `id`, `username`, `role`, `account_status`, `secret_key` (returned once) | `…BaseResponse` + `UserData{id,username,role,account_status,created_at}` |

✅ **Status key resolved:** niova now uses `account_status`. ⚠️ **User model:** niova
generates and returns a `secret_key` once (no password input); tidb takes a
`password`. ⚠️ tidb adds `created_at`; niova has no `secret_key` analog.

### `GET /api/users` — list users

| Field | Transport | niova | tidb |
|---|:--:|:--:|:--:|
| `username` | Q | ✅ (filter; niova extension) | ⚠️ not supported |

Response:

| niova | tidb |
|---|---|
| `users[ {id,username,role,account_status} ]` (payload) | `…BaseResponse`, `users[ {id,username,role,account_status,created_at} ]` |

✅ `account_status` key now matches. ⚠️ tidb adds `created_at`.

### `GET /api/users/{id}` — get user

| Field | Transport | niova | tidb |
|---|:--:|:--:|:--:|
| `id` | P | ✅ | ✅ |

Response:

| niova | tidb |
|---|---|
| `id`, `username`, `role`, `account_status`, `secret_key?` (payload) | `…BaseResponse` + `UserData{…,account_status,created_at}` |

⚠️ niova returns the decrypted `secret_key` on a single-entity fetch; tidb never
returns a secret. `account_status` key now matches.

### `PUT /api/users/{id}` — update user

| Field | Transport | niova | tidb |
|---|:--:|:--:|:--:|
| `id` | P | ✅ | ✅ |
| `username` | B | ✅ | ⚠️ not supported |
| `new_secret_key` | B | ✅ (admin only) | ⚠️ n/a |
| `role` | B | ⚠️ not supported | ✅ |
| `status` | B | ⚠️ not supported | ✅ |
| `new_password` | B | ⚠️ n/a | ✅ |
| `X-RNCUI` | H | ✅ required | ❌ |

Response:

| niova | tidb |
|---|---|
| `id`, `username`, `role`, `account_status`, `secret_key?` (if key changed) (payload) | `…BaseResponse` + `UserData` |

⚠️ **Divergent update semantics:** niova updates `username` + (admin) `secret_key`;
tidb updates `role`/`status`/`password`. These are almost disjoint — the deepest
model difference in the contract.

---

## 3. niova-mdsvc-only endpoints

| Endpoint | Request | Response | Notes |
|---|---|---|---|
| `POST /api/mount_vdev` | `vdev_id` (B) + `X-RNCUI` (H) | `MountVdevPayload`: `id`, `name`, `size`, `num_chunks`, `num_replica`, `mount_counter`, `last_updated_lts`, `pfs_id`, `access_token` | ✅ **now flat snake_case** (was PascalCase `VdevCfg`) |
| `POST /api/snap` | `vdev_id`, `snap_name`, `chunk_seq[]?` (B) + `X-RNCUI` (H) | `snap_name` (payload) | — |
| `POST /api/nisd_args` | `defrag`, `allow_defrag_mcib_cache`, `mbc_cnt`, `merge_h_cnt`, `mcib_read_cache`, `s3`, `dsync` (B) + `X-RNCUI` (H) | `WritePayload{id?}` | Singleton record |
| `GET /api/nisd_args` | — | `nisd_args{…}` (payload) | — |
| `POST /users/admin` | `username`, `secret_key?` (B) + `X-RNCUI` (H) | `id`, `username`, `role`, `account_status`, `secret_key` (payload) | Unauthenticated bootstrap; ⚠️ returns admin secret key |

---

## 4. mdsvc-tidb-only endpoints

| Endpoint | Request | Response | niova status |
|---|---|---|---|
| `POST /api/infra` | `pdus[ …nested… ]` (B) | `BaseResponse` | No atomic server op (would need proxy fan-out over `PUT_*`) |
| `GET /api/chunks` | `vdev_id`, `start_chunk_idx`, `limit` (Q) | `…BaseResponse`, `vdev_id`, `start_chunk_idx`, `limit`, `next_start_chunk_idx`, `has_more`, `total_chunks`, `chunks[]` | DTO exists (`GetChunksResponse`); **route deferred** |
| `POST /api/chunk/reassign` | `vdev_id`, `chunk_idx`, `failed_replica_idx`, `vdev_config_num`, `snapshot_seqno` (B) | `…BaseResponse`, `nisd_id`, `vdev_config_num`, `snapshot_seqno`, `applied` | No niova server op |
| `POST /api/reset` | — | `AdminResetResponse{success, message?, error?, deleted_* counts}` (⚠️ **not** `BaseResponse`) | No niova server op |
| `GET /api/authz/rbac` | — | `…BaseResponse`, `policies[{resource,action,role}]` | Not built (niova RBAC is server-side) |
| `POST /api/authz/rbac` | `resource`, `action`, `role` (B) | `AuthzResponse{…BaseResponse}` | Not built |
| `DELETE /api/authz/rbac/{resource}/{action}/{role}` | resource, action, role (P) | `AuthzResponse` | Not built |
| `GET /api/authz/abac` | — | `…BaseResponse`, `policies[{resource,action}]` | Not built |
| `POST /api/authz/abac` | `resource`, `action` (B) | `AuthzResponse` | Not built |
| `DELETE /api/authz/abac/{resource}/{action}` | resource, action (P) | `AuthzResponse` | Not built |

⚠️ Even within tidb the envelope isn't uniform: `AdminResetResponse` uses
`{success, error}` (like niova) rather than `BaseResponse` — a pre-existing tidb
inconsistency the unification should also fix.

---

## 5. Cross-cutting inconsistencies (summary)

Ranked by blast radius (✅ = resolved on the niova side; the tidb port is still
pending for the shared-envelope items):

1. ✅ **Response envelope.** niova now uses `APIResponse[T]{success,error,payload}`
   (§6, Design A). tidb still on `BaseResponse` — resolves fully once tidb ports.
2. ✅ **HTTP status semantics.** niova implements the 200-for-method-outcomes policy
   (§7, Design 1) on data endpoints; user/auth keep real codes. tidb pending.
3. **User model.** Secret-key (niova) vs password + role/status (tidb). Update
   bodies are nearly disjoint (`username`/`new_secret_key` vs `role`/`status`/`new_password`).
   *(Unchanged — a genuine model difference, not a wire mismatch.)*
4. ✅ **`status` vs `account_status`** — niova now emits `account_status`.
5. ✅ **Collection vs single.** `GET /api/resource` now returns a generic
   `resources` array (matching tidb). `GET /api/pfs` is a list on niova (a superset;
   tidb should adopt the list) — no niova change needed.
6. **Write-response `id`.** niova returns the mutated id (`WritePayload.id`); tidb
   returns status only, and uses `pfs_id` where niova uses `id`.
7. **`X-RNCUI` header.** Required on every niova write (PumiceDB dedup); unknown to
   tidb. Intentional niova extension — request *body* shapes stay identical.
8. ✅ **Casing.** `mount_vdev` is now flat snake_case (`MountVdevPayload`); the whole
   REST surface is snake_case.
9. ✅ **Richer echoes.** niova vdev create/get now echo `name`/`failure_domain`
   (+`pfs_id` on create); login already echoes `token_type`/`user_id`/`role`/`is_admin`.
10. **Intra-tidb drift.** `AdminResetResponse` doesn't use `BaseResponse` (tidb-side).

---

## 6. Proposed response-envelope designs

All three keep `success` + an error message; they differ in where the
method-specific data goes and whether a machine-readable code is retained.

### Design A — nested `payload` envelope (✅ implemented on niova)

```go
type APIResponse[T any] struct {
    Success bool   `json:"success"`
    Error   string `json:"error,omitempty"`   // human-readable; empty on success
    Payload *T     `json:"payload,omitempty"`  // method-specific; nil on error
}
```

```jsonc
// success
{ "success": true, "payload": { "vdev_id": "…", "num_chunks": 8 } }
// failure
{ "success": false, "error": "vdev not found: …" }
```

- **Pros:** clean separation of envelope vs data; one wrapper for every endpoint;
  generics give type-safe encode/decode; trivial to add fields to a payload without
  touching the envelope.
- **Cons:** clients two-step decode when they don't know the payload type ahead of
  time (use `APIResponse[json.RawMessage]`); a structural change for **both** repos
  (tidb loses `BaseResponse`).

### Design B — flat envelope, inline fields (extend niova's current shape)

```go
type Envelope struct {
    Success   bool   `json:"success"`
    Error     string `json:"error,omitempty"`
    ErrorCode string `json:"error_code,omitempty"` // optional NCP_* taxonomy
    // …method fields inlined per endpoint (embed the payload struct)…
}
```

```jsonc
{ "success": true, "vdev_id": "…", "num_chunks": 8 }
```

- **Pros:** smallest change for niova (already flat); no nesting; one-step decode.
- **Cons:** every endpoint redefines the envelope by embedding; field-name
  collisions possible; harder to write one generic client decoder; drifts back
  toward per-endpoint bespoke structs (the current problem).

### Design C — retain `BaseResponse`, add `success` alias

```go
type BaseResponse struct {
    Success       bool   `json:"success"`
    Status        string `json:"status"`           // "success"|"failure" (kept for tidb clients)
    NCPStatusCode string `json:"ncp_status_code"`   // NCP_* taxonomy
    Message       string `json:"message,omitempty"` // == error text
}
```

- **Pros:** smallest change for tidb; preserves the `NCP_*` taxonomy and existing
  tidb clients; niova just adds the fields.
- **Cons:** redundant `success`/`status` and `message`/`error`; keeps tidb's
  verbosity; doesn't give niova a `payload` slot; least "unified".

**Recommendation:** **Design A**, optionally carrying the `error_code` field from
Design B so the tidb `NCP_*` taxonomy survives as a machine-readable code while
`error` stays human-readable. That satisfies "success + error + payload" and loses
nothing tidb has today.

---

## 7. Proposed HTTP status-code designs

The stated target: **method (business) errors → HTTP 200 with `success:false`**;
**login → real codes** when credentials are rejected. Three ways to draw the line.

### Design 1 — outcome-in-body (✅ implemented on niova)

- **200** for every method outcome — success *and* domain failure (not found,
  conflict, insufficient capacity, invalid domain input, snapshot exists, …). The
  `success` flag carries the verdict.
- **Real HTTP codes** only for things that aren't a method result:
  - **Login:** 400 (missing fields) / 401 (bad credentials).
  - **Authn/authz** on other endpoints: 401 (no/invalid token) / 403 (RBAC deny).
  - **Protocol:** 400 (malformed JSON, bad UUID in path, missing `X-RNCUI`) / 405
    (wrong method).

| Category | HTTP | Body |
|---|:--:|---|
| Method success | 200 | `success:true`, payload |
| Method/domain error | 200 | `success:false`, `error` |
| Login: bad credentials | 401 | `success:false`, `error` |
| Login: missing fields | 400 | `success:false`, `error` |
| Auth token invalid / RBAC deny | 401 / 403 | `success:false`, `error` |
| Malformed request / bad method | 400 / 405 | `success:false`, `error` |

- **Pros:** exactly the requested behavior; clients branch on `success`, not status;
  proxy stops guessing status from error strings (`statusForCPError` shrinks to
  authn/authz only). **Cons:** non-idiomatic REST; intermediaries (LBs, caches)
  can't act on status; must decide the authn/protocol boundary (below).

### Design 2 — status-code-native (what both do today)

Map every error category to a real HTTP code (404/409/400/403/500…), envelope
mirrors it. **Pros:** idiomatic; observable at the HTTP layer. **Cons:** rejected by
the requirement; keeps the fragile string→status mapping; two error signals
(`success` and status) to keep consistent.

### Design 3 — 200-always (strict reading)

Everything, *including* login-auth and protocol errors, returns 200; the envelope
`success`/`error_code` is the sole signal. **Pros:** dead-simple rule, one code
path. **Cons:** contradicts "use proper codes for login"; hides auth failures from
infra; unusual enough to surprise clients/tooling.

**Recommendation:** **Design 1.** Open sub-decisions:

1. **Authn/authz boundary** — treat *all* authentication/authorization failures as
   real codes (401/403), generalizing the login carve-out? (Recommended: yes.)
2. **Protocol errors** — keep 4xx for malformed body / bad path param / missing
   `X-RNCUI`, since the method never ran? (Recommended: yes.)
3. **Success code** — standardize all success to **200** (drop the current 201 on
   create)?

---

*Companion doc: `API_COMPARISON.md` (higher-level endpoint comparison). This file is
the field-level unification reference.*
