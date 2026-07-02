# REST API Comparison: niova-mdsvc vs mdsvc-tidb

This document compares the REST HTTP contracts of the two control-plane
implementations:

- **niova-mdsvc** — the raft/PumiceDB control plane. REST is served by the proxy
  under `/api/…` (plus `/users/login`); additive — the legacy `/func` RPC endpoint
  still serves every op.
- **mdsvc-tidb** — the TiDB-backed control-plane service. REST under `/api/…`
  (plus `/users/login`), JWT auth.

Both use a Go 1.22 `ServeMux` with method+pattern routing, path/query params, and
HTTP status codes. The largest structural difference is the response envelope.

---

## 0. Response envelope (applies to every endpoint)

| | niova-mdsvc | mdsvc-tidb |
|---|---|---|
| Wrapper | `success bool` + `error string` (+ sometimes `message`) | `BaseResponse{ status: "success"/"failure", ncp_status_code: "NCP_OK"/…, message }` (embedded in every response) |
| Error signal | HTTP 4xx/5xx + `{"error": "..."}` | HTTP status + `status:"failure"` + an `ncp_status_code` |
| Write idempotency | **`X-RNCUI` header required** on every write (PumiceDB dedups writes by RNCUI) | none |

`ncp_status_code` values (TiDB): `NCP_OK`, `NCP_FAILURE`, `NCP_INVALID_REQUEST`,
`NCP_NOT_FOUND`, `NCP_FORBIDDEN`, `NCP_CONFLICT`, `NCP_CAPACITY_RACE`,
`NCP_INTERNAL_ERROR`, `NCP_INFRA_ALREADY_EXISTS`, `NCP_UNSUPPORTED_RESOURCE_TYPE`,
`NCP_INSUFFICIENT_CAPACITY`.

---

## 1. Endpoint coverage matrix

| Endpoint | Action | niova | TiDB |
|---|---|:--:|:--:|
| `POST /api/vdev` | Create vdev (+allocate chunks) | ✅ | ✅ |
| `GET /api/vdev` | Get vdev metadata | ✅ | ✅ |
| `DELETE /api/vdev/{id}` | Delete vdev | ✅ | ✅ |
| `GET /api/nisd` | Get NISD connection info | ✅ | ✅ |
| `GET /api/chunk` | Placement of one chunk | ✅ | ✅ |
| `GET /api/resource` | List/get infra entity by type | ✅ | ✅ |
| `PUT /api/resource` | Upsert one infra entity | ✅ | ✅ |
| `GET /api/infra` | Full infra tree | ✅ | ✅ |
| `POST /api/pfs` | Create PFS | ✅ | ✅ |
| `GET /api/pfs` | Get/list PFS | ✅ | ✅ |
| `POST /api/mount_vdev` | Mount vdev (mint access token) | ✅ | ❌ |
| `POST /api/snap` | Create snapshot | ✅ | ❌ |
| `POST /api/nisd_args` · `GET /api/nisd_args` | NISD launch args (singleton) | ✅ | ❌ |
| `POST /api/infra` | **Atomic** create-infra | ❌ (no server op) | ✅ |
| `GET /api/chunks` | Paginated chunk map | ❌ (deferred) | ✅ |
| `POST /api/chunk/reassign` | Reassign failed replica | ❌ (no server op) | ✅ |
| `POST /api/reset` | Reset CP state | ❌ (no server op) | ✅ |
| `POST /users/login` | Login (JWT) | ✅ | ✅ |
| `POST /api/users` | Create user | ✅ | ✅ |
| `GET /api/users` | List users | ✅ | ✅ |
| `GET /api/users/{id}` | Get user | ✅ | ✅ |
| `PUT /api/users/{id}` | Update user | ✅ | ✅ |

---

## 2. Shared endpoints — field-level comparison

### `POST /api/vdev` — create a vdev and allocate its chunk map

| | niova request | TiDB request |
|---|---|---|
| fields | `size_bytes`, `num_replicas`, `failure_domain?`, `entity_ids[]?`, `pfs?`, `name?` | `size_bytes`, `num_replicas`, `failure_domain?`, `entity_ids[]?`, `pfs?`, `name?` |

Requests are equivalent. `name` is a niova extension but TiDB also carries it via
`BaseRequest`. niova applies only `entity_ids[0]` (its allocator scopes to a
single entity). niova writes require `X-RNCUI`.

| | niova response | TiDB response |
|---|---|---|
| fields | `success`, `vdev_id`, `num_chunks`, `message?`, `error?` | `…BaseResponse`, `vdev_id`, `name`, `num_chunks`, `failure_domain`, `pfs_id` |

→ TiDB echoes more (`name`, `failure_domain`, `pfs_id`).

### `GET /api/vdev?id=` — vdev metadata

| niova response | TiDB response |
|---|---|
| `success`, `id`, `size`, `num_chunks`, `num_replicas`, `error?` | `…BaseResponse`, `id`, `name`, `size`, `num_chunks`, `num_replicas`, `failure_domain` |

→ niova resolves `id` as **UUID or name**; TiDB adds `name`/`failure_domain` to the body.

### `DELETE /api/vdev/{id}` — delete a vdev

| niova response | TiDB response |
|---|---|
| `WriteResponse{ success, id, message?, error? }` (needs `X-RNCUI`) | `BaseResponse` |

### `GET /api/nisd?id=` — NISD connection info

| niova response | TiDB response |
|---|---|
| `success`, `id`, `peer_port`, `net_info[{ip_addr,port}]`, `error?` | `…BaseResponse`, `id`, `peer_port`, `net_info[{ip_addr,port}]` |

→ same fields.

### `GET /api/chunk?vdev_id=&chunk_idx=` — placement of one chunk

| niova response | TiDB response |
|---|---|
| `success`, `vdev_id`, `chunk_idx`, `nisd_ids[]`, `error?` | `…BaseResponse`, `vdev_id`, `chunk_idx`, `nisd_ids[]` |

→ same fields, same query params (`nisd_ids` are one per replica, replica-idx order).

### `GET /api/resource?type=[&id=]` — list/get infra entities

| niova response | TiDB response |
|---|---|
| `success`, `type`, typed slices `pdus[] / racks[] / hypervisors[] / devices[] / nisds[] / partitions[]`, `error?` | `…BaseResponse`, `type`, `resources` (generic array) |

→ niova returns **typed per-type slices** and supports single-fetch via `?id=`;
TiDB returns a generic `resources`. niova adds a **partition** resource type TiDB lacks.

### `PUT /api/resource?type=` — upsert one entity (per-entity JSON body)

| niova response | TiDB response | Supported types |
|---|---|---|
| `WriteResponse{ success, id, … }` (needs `X-RNCUI`) | `BaseResponse` | niova: pdu / rack / hypervisor / device / nisd / **partition**; TiDB: pdu / rack / hypervisor / device / nisd |

### `GET /api/infra` — full PDU → Rack → Hypervisor → Device → NISD tree

| niova response | TiDB response |
|---|---|
| `success`, `infra{ pdus[ …nested… ] }`, `error?` | `…BaseResponse`, `infra{ pdus[ …nested… ] }` |

→ same nested shape (niova assembles it proxy-side from `GET_ALL_RESOURCES`).

### `POST /api/pfs` — create a PFS

| niova request | TiDB request |
|---|---|
| `PFS{ id?, name?, offset?, vdev_ids[]? }` (id optional; server mints it) | `{ name? }` |

| niova response | TiDB response |
|---|---|
| `WriteResponse{ success, id, … }` | `…BaseResponse`, `pfs_id`, `name` |

### `GET /api/pfs?id=`

| niova response | TiDB response |
|---|---|
| `success`, `pfs[ { id, name, offset, vdev_ids[] } ]`, `error?` (**list**) | `…BaseResponse`, `id`, `name`, `offset`, `vdev_ids[]` (**single**) |

### User auth & management

niova exposes these over REST (proxy → the existing `/func` user ops), mirroring
the TiDB user handler skeleton but keeping the `{success,error}` envelope and
niova's **secret-key model**:

- **TiDB** — password-based; users carry `role` and `status`.
- **niova** — secret-key based: create **generates and returns the key once**
  (no password input); login authenticates with username + that key (sent in
  `password`); update supports **username + (admin) secret-key only**, not
  role/status. Writes (create/update) require `X-RNCUI`; the bearer token is
  forwarded so the control plane enforces RBAC.

#### `POST /users/login`
| niova | TiDB |
|---|---|
| req `{ username, password }` — `password` is the niova secret key | req `{ username, password }` |
| resp `{ success, access_token, token_type, expires_in, user_id, username, role, is_admin, error? }` | resp `…BaseResponse`, `access_token`, `expires_in` |

#### `POST /api/users` — create (needs `X-RNCUI`)
| niova | TiDB |
|---|---|
| req `{ username, role? }` — server generates the secret key | req `{ username, password, role? }` |
| resp `{ success, id, username, role, status, secret_key (returned once), error? }` | resp `…BaseResponse` + `UserData` |

#### `GET /api/users` — list (admin-only, server-enforced)
| niova response | TiDB response |
|---|---|
| `success`, `users[ { id, username, role, status } ]`, `error?` | `…BaseResponse`, `users[]` |

#### `GET /api/users/{id}`
| niova response | TiDB response |
|---|---|
| `success`, `id`, `username`, `role`, `status`, `error?` | `…BaseResponse` + `UserData` |

#### `PUT /api/users/{id}` — update (needs `X-RNCUI`)
| niova | TiDB |
|---|---|
| req `{ username?, new_secret_key? }` — no role/status | req `{ role?, status?, new_password? }` |
| resp `{ success, id, username, role, status, secret_key? (if key changed), error? }` | resp `…BaseResponse` + `UserData` |

> **Not yet aligned:** broader JWT/RBAC middleware. The niova proxy only forwards
> the bearer token; enforcement stays server-side. TiDB runs JWT + RBAC/ABAC
> middleware in front of these routes.

---

## 3. niova-only endpoints

| Endpoint | Request | Response | Action |
|---|---|---|---|
| `POST /api/mount_vdev` | `{ vdev_id }` + `X-RNCUI` | full `VdevCfg` (`ID`, `Name`, `Size`, `NumChunks`, `NumReplica`, `VdevMountInfo{ MountCounter, LastUpdatedLTS }`, `PFSID`, `AccessToken`, …) — **PascalCase JSON** | Mount vdev; mint the data-path access token, bump the mount counter |
| `POST /api/snap` | `{ vdev_id, snap_name, chunk_seq[]? }` + `X-RNCUI` | `{ success, snap_name, error? }` | Create a snapshot |
| `POST /api/nisd_args` | `{ defrag, allow_defrag_mcib_cache, mbc_cnt, merge_h_cnt, mcib_read_cache, s3, dsync }` + `X-RNCUI` | `WriteResponse` | Set the singleton NISD launch-args record |
| `GET /api/nisd_args` | — | `{ success, nisd_args{…}, error? }` | Get the NISD launch-args record |

---

## 4. TiDB-only endpoints

| Endpoint | Request | Response | Action / niova status |
|---|---|---|---|
| `POST /api/infra` | `{ pdus[ …nested… ] }` | `BaseResponse` | Atomic infra create — niova has **no atomic op** (would need proxy fan-out over PUT_*) |
| `GET /api/chunks` | `?vdev_id=&start_chunk_idx=&limit=` | `…BaseResponse`, `vdev_id`, `start_chunk_idx`, `limit`, `next_start_chunk_idx`, `has_more`, `total_chunks`, `chunks[]` | Paginated chunk map — niova DTO exists but the **route is deferred** |
| `POST /api/chunk/reassign` | `{ vdev_id, chunk_idx, failed_replica_idx, vdev_config_num, snapshot_seqno }` | `…BaseResponse`, `nisd_id`, `vdev_config_num`, `snapshot_seqno`, `applied` | Reassign a failed replica — **no niova server op** |
| `POST /api/reset` | — | `BaseResponse` | Wipe CP state — **no niova server op** |

*(User auth/management endpoints are now supported by both — see §2.)*

---

## 5. Summary

- The **15 shared endpoints** (10 infra/vdev + 5 user) are request-compatible:
  query/path/body shapes align, so a data-path client can talk to either control
  plane.
- Cross-cutting differences: (1) the **envelope** (`{success,error}` vs
  `BaseResponse{status,ncp_status_code}`), (2) niova's **`X-RNCUI`** write header,
  (3) niova returns **typed slices / lists** where TiDB returns generic/single objects.
- **User endpoints** are shared in shape but differ in model: niova is
  **secret-key** based (no password/role/status) vs TiDB's password + role/status.
  Broader JWT/RBAC middleware is **not yet aligned** — the niova proxy forwards the
  token and the control plane enforces RBAC.
- niova adds vdev-lifecycle ops (mount / snap / nisd_args) and a **partition**
  resource type; TiDB adds **atomic infra**, **chunk pagination/reassign**, and
  **reset**.
- niova keeps the legacy `/func` RPC endpoint alive alongside REST, so unmodified
  clients continue to work; REST is an additive surface, not a replacement.
