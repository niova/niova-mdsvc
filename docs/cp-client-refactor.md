# Packaging the Control Plane Client

**Status:** Proposal — decision required

## Current Situation

The control plane client (`ctlplanefuncs/client`) is currently used by:

| Consumer | Current Usage |
|----------|---------------|
| niova-block-csi | Uses `niova-mdsvc` as a git submodule |
| ccManager | Imports the client directly (same repository) |
| mdsvc-tidb | Shares no Go code; communicates only through REST |

The question is whether the client (and ccManager) should be moved into a separate repository.

## Recommendation

**Not yet.**

Moving to a separate repository does not solve the current problems—it only moves them elsewhere.

The better approach is to:

1. Split the client into its own Go module inside this repository.
2. Reduce unnecessary dependencies.
3. Make the client backend-agnostic.
4. Consider extracting it into a separate repository later.

This provides nearly all the benefits of a standalone SDK without introducing cross-repository maintenance.

---

# Current Problems

## 1. No module boundary

Everything currently belongs to the `niova-mdsvc` Go module.

As a result, external consumers inherit the entire mdsvc dependency graph even though they only require a small client library.

---

## 2. Heavy dependencies

The client currently depends on:

- PumiceDB
- Serf service discovery
- cgo (`pumicecommon`)
- Other internal packages

This unnecessarily increases the dependency graph.

Examples:

- CSI already knows the control plane URL but still depends on Serf.
- A single `C.GoBytes()` usage forces consumers to build with `CGO_ENABLED=1`.

These dependencies should become optional.

---

## 3. Backend-specific APIs

Most client APIs already communicate through REST.

However, several read APIs still use the legacy `/func` interface, which only works against the PumiceDB backend.

As a result, the client is not yet backend-independent and cannot be treated as a common SDK.

---

# Options

## Option 1 — Keep everything as-is

### Pros

- No migration effort.
- Server and client evolve together.
- No version coordination.

### Cons

- Consumers pull the entire mdsvc dependency graph.
- CSI requires git submodules.
- mdsvc-tidb cannot realistically reuse the client.
- Internal implementation packages become public dependencies.

**Verdict:** Works today but does not scale.

---

## Option 2 — Create a nested Go module (**Recommended**)

Create a dedicated SDK module inside this repository.

```
controlplane/sdk/
    go.mod
    client/
    restapi/
    types/
    user/
```

### Pros

- Smaller dependency graph for consumers.
- Clear public API boundary.
- Atomic development remains possible.
- Easy future extraction into its own repository.
- External consumers can use versioned Go modules instead of git submodules.

### Cons

- Nested modules require independent version tagging.
- CI should verify both local (`replace`) and published module builds.

**Verdict:** Best balance between simplicity and long-term maintainability.

---

## Option 3 — Separate SDK repository

Move the SDK into its own repository (for example, `niova-cp-sdk`).

### Pros

- Clean dependency model.
- Independent semantic versioning.
- Simple consumer experience (`go get`).

### Cons

- Every API or DTO change now requires changes across multiple repositories.
- Version skew becomes possible.
- Does not solve the current dependency issues.
- Backend-specific APIs still need to be addressed.

**Verdict:** A good long-term destination, but not the right first step.

---

## Option 4 — Separate ccManager

Move ccManager into its own repository.

### Pros

- Independent release cycle.

### Cons

- ccManager is only a thin wrapper around the client.
- Introduces unnecessary repository and release overhead.

**Verdict:** Not recommended.

---

## Option 5 — Generate clients from OpenAPI

Generate clients directly from `openapi.yml`.

### Pros

- Language independent.
- Eliminates backend-specific APIs.
- OpenAPI becomes the source of truth.

### Cons

- Generated clients lack higher-level functionality such as:
  - service discovery
  - authentication helpers
  - request/response conversions
- Additional wrapper code would still be required.

**Verdict:** Useful for API validation, but not a replacement for the SDK.

---

# Recommended Plan

## Phase 1

Create a new SDK module inside the repository.

Move:

- `ctlplanefuncs/client`
- `ctlplanefuncs/lib` (or `types`)
- `restapi`
- `user/{lib,client}`

Keep development within the same repository.

---

## Phase 2

Reduce coupling by:

- Removing the cgo dependency.
- Making service discovery optional.
- Allowing clients to use either:
  - a base URL, or
  - a discovery implementation.
- Converting remaining `/func` APIs to REST or isolating them into a backend-specific package.

---

## Phase 3

Move the SDK into its own repository once:

- it has minimal dependencies,
- it is backend-agnostic,
- multiple projects actively consume it, and
- the REST API has stabilized.

At that point, extraction becomes largely mechanical.

---

# Summary

The current challenges are caused by:

- a single large Go module,
- unnecessary dependencies,
- backend-specific functionality.

Creating a separate repository today would not solve these problems.

Creating a dedicated SDK module inside the existing repository addresses the dependency boundary immediately while preserving a simple development workflow. Once the SDK becomes lightweight and backend-independent, moving it into its own repository becomes a low-risk, straightforward change.

