# Implementation Plan — Sambee-inspired features for our Filestash fork

**Deployment context:** self-hosted, internal network, **SMB passthrough** (IdP `passthrough` → `attribute_mapping` → samba backend). Priorities, in order: **security & auditability → SMB performance/correctness → editing UX → ops → sharing & desktop integration.**

Feature ideas are distilled from [`sambee-app/sambee`](https://github.com/sambee-app/sambee) (an SMB-first browser file manager) and mapped onto Filestash's existing extension points.

## Guiding principles

1. **Build as plugins, not core forks.** Use the `Hooks.Register.*` surface (`server/pkg/kernel/plugin.go`) and `server/pkg/core` interfaces. Register in `server/plugin/index.go`. This keeps us rebaseable on upstream.
2. **Tests ship with each feature.** Upstream has **0 Go unit tests**; we add table tests + a `fakeBackend` as we go. Nothing merges to our `master` without `go test ./...` green.
3. **Privacy first.** Telemetry stays compiled out (Phase 0); no new outbound calls without an explicit, documented config flag that defaults off.
4. **Each feature is independently shippable** behind config, so we can deploy incrementally.

## Effort key
`S` ≈ ≤2 days · `M` ≈ 3–8 days · `L` ≈ 2–4 weeks (one dev). IDs (`SEC-1`…) are stable and referenced by `docs/TRACKER.md`.

---

## Phase 0 — Foundations (enablers)

| ID | Item | Hook / area | Effort | Notes |
|----|------|-------------|--------|-------|
| F-1 | GitHub Actions CI | repo | S | build matrix (amd64/arm64), `go vet`, `go test ./...`; mirror Jenkinsfile's `plg_image_c`→`plg_image_golang` swap for CGO-free builds |
| F-2 | Unit-test seed + `fakeBackend` | `pkg/utils`, `pkg/core` | S | crypto round-trip, path utils, Form JSON, `GenerateID`; an in-memory `IBackend` for handler tests |
| F-3 | Hard-disable telemetry | `pkg/middleware/telemetry.go` | S | neutralize `Flush()`/`Record`; assert `log.telemetry=false`; already scoped as a 5-line patch |
| F-4 | Plugin-authoring dev doc | `docs/` | S | document the `init()`+`Hooks.Register` lifecycle, `Init/LoginForm`, `Form`/`FormElement` |

**Exit criteria:** green CI on every push; a template plugin + test others can copy.

---

## Phase 1 — Security & auditability (highest value)

### SEC-1 — Audit log plugin + admin viewer · `AuditEngine(IAuditPlugin)` · **M**
- **What:** record change events (create/rename/delete/content-modify; not reads), with dedup of rapid re-saves (≤1 event / 24 h per user+file), retention + max-entries caps.
- **Why:** Filestash's `IAuditPlugin` interface exists but is unimplemented; essential for an internal multi-user NAS.
- **Approach:** implement `IAuditPlugin`; storage = sqlite (reuse the pattern in `plg_metadata_sqlite`) or append-only JSONL; admin viewer via an `HttpEndpoint` + a back-office page; filters (user, path, action, time).
- **Data:** `audit(id, ts, user_hash, action, connection, path, from_path, size, request_id)`.
- **Risks:** path privacy (store under the same secret scheme; allow path hashing); volume (enforce retention).

### SEC-2 — Generic OIDC / OAuth2 auth · `AuthenticationMiddleware(id, IAuthentication)` · **M**
- **What:** standards OIDC login (auth-code + PKCE), claim→attribute mapping into `attribute_mapping` (so it composes with the samba backend exactly like `passthrough`).
- **Why:** upstream only has LDAP/WordPress/passthrough; OIDC is the most-requested SSO and unblocks Entra/Keycloak/Okta.
- **Approach:** implement `Setup()/EntryPoint()/Callback()`; discovery doc + JWKS caching; map `preferred_username`/`email`/groups into the session map consumed by attribute mapping.
- **Risks:** token/refresh handling; clock skew; keep all calls to the IdP only (documented host).

### SEC-3 — Auth rate-limiting / lockout hardening · `Middleware` + review `plg_security_authlock` · **S–M**
- **What:** per-identity + per-IP throttle and temporary lockout on repeated auth failures; audit the lockouts (ties to SEC-1).
- **Approach:** extend/replace `plg_security_authlock`; sliding-window counter in an `AppCache`.

### SEC-4 — Per-directory authorisation policy · `AuthorisationMiddleware(IAuthorisation)` · **M**
- **What:** grant/deny by connection + path prefix + action (list/read/write/delete), per user/group.
- **Why:** passthrough today inherits whatever SMB allows; this adds an app-level policy layer and makes least-privilege explicit.
- **Data:** policy rules in config or DB; evaluated in request pipeline.

**Exit criteria:** every auth path logged; OIDC usable alongside SMB passthrough; lockout + per-dir policy enforced and tested.

---

## Phase 2 — SMB performance & correctness

### SMB-1 — SMB connection pool / reuse · `plg_backend_samba` · **M**
- **What:** thread-safe pool keyed by (host, credentials), reusing `smb2.Session`/`Share` across requests instead of dialing per session.
- **Why:** SMB is chatty/latent; today the backend `net.DialTimeout`s per `Init`. Sambee uses a pool with SMB "credit" accounting.
- **Risks:** credential isolation (never share a session across identities); idle eviction; reconnect on broken pipe.

### SMB-2 — Directory listing cache + index · backend wrapper / `Metadata` · **M**
- **What:** short-TTL cache of `Ls` results + an optional background index for fast navigation/search over latent shares.
- **Risks:** invalidation (tie to SMB-4 monitoring where available); bound memory.

### SMB-3 — File lock manager · `Middleware`/service · **M**
- **What:** advisory locks to prevent concurrent-edit clobbering of the same file across users/sessions.
- **Approach:** in-process lock registry keyed by connection+path with TTL + heartbeat from the editor; surfaced in the UI.

### SMB-4 — Live directory monitoring → WebSocket push · `HttpEndpoint` (WS) + `directory_monitor` service · **L**
- **What:** push "directory changed" events to the browser so listings update live for multiple users.
- **Risks:** SMB change-notify support varies; fall back to TTL polling (SMB-2).

**Exit criteria:** measurable latency drop on repeat navigation; no cross-identity session reuse; safe concurrent editing.

---

## Phase 3 — Viewers & editing UX (frontend)

> Note: Filestash's frontend is a custom RxJS/TS SPA (not React). These land as frontend overrides via `Static`/`StaticPatch`/`CSS` plus any needed `HttpEndpoint` for server-side rendering/derivatives.

| ID | Feature | Effort | Notes |
|----|---------|--------|-------|
| UX-1 | Robust PDF viewer + rendered-derivative cache | M | normalize/repair problematic PDFs server-side; cache renders (cf. `pdf_normalizer`/`pdf_derivative_cache`) |
| UX-2 | Markdown WYSIWYG w/ table editing | M | richer than the CodeMirror editor |
| UX-3 | Draft recovery / autosave | S–M | recover unsaved edits after disconnect — valuable over flaky SMB |
| UX-4 | In-browser archive (zip) browse / extract / create | M | resumable archive operations |
| UX-5 | Image viewer breadth (multi-page TIFF/ICO, DCM) + ImageMagick preprocessor | M | reuse `Thumbnailer`/`ProcessFileContentBeforeSend` |
| UX-6 | Delete action in every viewer (confirm + `Del`) | S | small cross-cutting UX gap |

---

## Phase 4 — Collaboration & ops

| ID | Feature | Hook | Effort | Notes |
|----|---------|------|--------|-------|
| OPS-1 | Health dashboard (SMB reachability, disk, mem) | `HttpEndpoint` + back-office | M | ops signal for SMB outages |
| OPS-2 | Cross-connection copy/move | core/handlers | M | move between two shares/backends in one op |
| OPS-3 | Recent files / recent directories | `Metadata` + widget | S | extend `plg_widget_recent` |
| OPS-4 | Mobile-first UI polish + theming | `Static`/`CSS` | M | compact actions, drawer nav |
| OPS-5 | Prometheus `/metrics` | `HttpEndpoint` | S | good first standalone plugin; per-backend latency |

---

## Phase 5 — Secure sharing & desktop integration (largest, optional)

### SHARE-1 — E2E-encrypted public share links via isolated container · **L**
- **What:** internal backend never internet-exposed; a hardened public container stores only end-to-end-encrypted blobs (keys never land there); link embeds the key; options: password (combined with key), expiry/auto-delete, max-download count; **upload links** tied to a dir with a size cap.
- **Why:** a far safer external-sharing model than built-in shares for an internal NAS.
- **Risks:** key management, container hardening, pull/sync scheduling — design doc required before build.

### DESK-1 — Desktop "Companion" helper · **L**
- **What:** open a file in its native desktop app (Word/Excel/Photoshop) from the browser, edit, auto-upload changes back; local-drive access; PIN-based pairing.
- **Why:** upstream only has an `xdg-open` hook; a real companion closes the "edit in native app" gap.
- **Risks:** cross-platform packaging (Tauri-style), auth/pairing security, conflict handling with SMB-3 locks.

---

## Sequencing & dependencies
- **Phase 0 before everything** (CI + tests gate all merges).
- **SEC-1 (audit)** underpins SEC-3 and OPS-1 (shared event sink) — do it first in Phase 1.
- **SMB-2 (cache)** depends on **SMB-4 (monitoring)** for correct invalidation; ship SMB-2 with TTL first, wire SMB-4 later.
- **SMB-3 (locks)** should precede **DESK-1** (native-app editing needs locking).
- Phase 5 items each need a **design doc + threat model** before implementation.

## Recommended first slice (≈2–3 weeks)
`F-1 → F-2 → F-3` (foundations), then `OPS-5` (metrics, a throwaway "hello-world" plugin), then `SEC-1` (audit). That yields green CI, a proven plugin loop, and the single highest-value security feature.
