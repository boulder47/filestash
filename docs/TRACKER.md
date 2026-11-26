# Implementation Tracker

Living status for the work in `IMPLEMENTATION_PLAN.md`. Update the **Status** column in PRs. Legend: ⬜ todo · 🟡 in progress · ✅ done · ⏸️ blocked · ❌ dropped.

**Status key:** Effort `S`≤2d · `M`3–8d · `L`2–4w. Keep IDs stable.

## Phase 0 — Foundations
| ID | Item | Hook / area | Effort | Status | Owner | PR | Notes |
|----|------|-------------|:------:|:------:|-------|----|-------|
| F-1 | GitHub Actions CI (build/vet/test) | repo | S | ✅ | claude | `impl/phase0-foundations` | `.github/workflows/ci.yml`; amd64, CGO-free + fts5 |
| F-2 | Unit-test seed + `fakeBackend` | utils/core | S | ✅ | claude | `impl/phase0-foundations` | crypto, path, GenerateID tests + in-memory `coretest.FakeBackend` |
| F-3 | Hard-disable telemetry | middleware | S | ✅ | claude | `impl/phase0-foundations` | `telemetryDisabled` const guards `Record`/`Flush` |
| F-4 | Plugin-authoring dev doc | docs | S | ⬜ | | | lifecycle partly covered in ASSESSMENT.md §2 |

## Phase 1 — Security & auditability
| ID | Item | Hook | Effort | Status | Owner | PR | Notes |
|----|------|------|:------:|:------:|-------|----|-------|
| SEC-1 | Audit log + admin viewer | `AuditEngine` | M | ⬜ | | | do first; feeds SEC-3/OPS-1 |
| SEC-2 | Generic OIDC/OAuth2 auth | `AuthenticationMiddleware` | M | ⬜ | | | composes w/ attribute_mapping |
| SEC-3 | Auth rate-limit / lockout | `Middleware` | S–M | ⬜ | | | extend `plg_security_authlock` |
| SEC-4 | Per-directory authz policy | `IAuthorisation` | M | ⬜ | | | connection+path+action |

## Phase 2 — SMB performance & correctness
| ID | Item | Hook | Effort | Status | Owner | PR | Notes |
|----|------|------|:------:|:------:|-------|----|-------|
| SMB-1 | Connection pool / reuse | `plg_backend_samba` | M | ⬜ | | | never share across identities |
| SMB-2 | Dir listing cache + index | backend/`Metadata` | M | ⬜ | | | TTL first, invalidate via SMB-4 |
| SMB-3 | File lock manager | service/`Middleware` | M | ⬜ | | | precedes DESK-1 |
| SMB-4 | Live dir monitor → WebSocket | `HttpEndpoint` (WS) | L | ⬜ | | | fallback to polling |

## Phase 3 — Viewers & editing UX
| ID | Item | Effort | Status | Owner | PR | Notes |
|----|------|:------:|:------:|-------|----|-------|
| UX-1 | Robust PDF viewer + derivative cache | M | ⬜ | | | |
| UX-2 | Markdown WYSIWYG + tables | M | ⬜ | | | |
| UX-3 | Draft recovery / autosave | S–M | ⬜ | | | |
| UX-4 | Archive (zip) browse/extract/create | M | ⬜ | | | |
| UX-5 | Image viewer breadth + ImageMagick | M | ⬜ | | | TIFF/ICO/DCM |
| UX-6 | Delete action in viewers | S | ⬜ | | | confirm + `Del` |

## Phase 4 — Collaboration & ops
| ID | Item | Hook | Effort | Status | Owner | PR | Notes |
|----|------|------|:------:|:------:|-------|----|-------|
| OPS-1 | Health dashboard | `HttpEndpoint` | M | ⬜ | | | SMB/disk/mem |
| OPS-2 | Cross-connection copy/move | core | M | ⬜ | | | |
| OPS-3 | Recent files/dirs | `Metadata`+widget | S | ⬜ | | | extend recent widget |
| OPS-4 | Mobile UI + theming | `Static`/`CSS` | M | ⬜ | | | |
| OPS-5 | Prometheus `/metrics` | `HttpEndpoint` | S | ✅ | claude | `impl/ops5-metrics` | `plg_handler_metrics`, dependency-free text exposition + request middleware |

## Phase 5 — Secure sharing & desktop (optional, largest)
| ID | Item | Effort | Status | Owner | PR | Notes |
|----|------|:------:|:------:|-------|----|-------|
| SHARE-1 | E2E-encrypted public share/upload links | L | ⬜ | | | needs design doc + threat model |
| DESK-1 | Desktop Companion (native open + upload-back) | L | ⬜ | | | needs design doc; depends on SMB-3 |

---

## Suggested first slice (≈2–3 weeks)
`F-1` → `F-2` → `F-3` → `OPS-5` (hello-world plugin) → `SEC-1` (highest-value security feature).

## Changelog
- 2026-11-25 — Phase 0 started on `impl/phase0-foundations`: F-1 (CI), F-2 (unit-test seed + FakeBackend), F-3 (telemetry hard-disabled). Verified locally: `go test ./server/pkg/...` green, `go build ./server/pkg/...` CGO-free green, `coretest` vet clean.
- 2026-11-26 — OPS-5 on `impl/ops5-metrics`: `plg_handler_metrics` (Prometheus `/metrics`, dependency-free). Plugin build/vet/test green; confirmed it links into the full CGO-free `fts5` binary. First plugin — proves the `init()`+`Hooks.Register` loop end to end.
