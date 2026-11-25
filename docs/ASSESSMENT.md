# Filestash Fork — Codebase Assessment

_Scope: `server/` Go backend (~55k LOC, 388 files) + plugin system + frontend. Deployment context: internal network, **SMB passthrough**._

## 1. Architecture at a glance

- **Entry:** `cmd/main.go` → `server/routes.go::Build(router)`. Plugins are compiled in via blank imports in `server/plugin/index.go`.
- **`server/common`** is a *facade* re-exporting everything from `server/pkg/*`. Plugins import `. "server/common"` for a stable API.
- **`server/pkg/core`** holds the interfaces plugin authors implement: `IBackend`, `IAuthentication`, `IAuthorisation`, `ISearch`, `IMetadata`, `IThumbnailer`, `IAuditPlugin`, `ITracer`, `IDirectoryService`, workflow `ITrigger`/`IAction`.
- **`server/pkg/kernel/plugin.go`** is the `Hooks` registry — ~20 extension points.
- **Config:** single `state/config/config.json`. Plugins declare a `Form`/`FormElement` tree for the admin UI and login forms. Secrets are stored in-file, AES-GCM-encrypted under one `secret_key`.
- **Frontend:** custom vanilla **TypeScript + RxJS** SPA built with Vite (not React/Vue).

## 2. Extension surface (for our plugins)

| Area | Hook | Use |
|------|------|-----|
| Storage | `Backend.Register(id, IBackend)` | new storage protocol |
| Auth | `Hooks.Register.AuthenticationMiddleware(id, IAuthentication)` | IdP / login |
| Authz | `Hooks.Register.AuthorisationMiddleware(IAuthorisation)` | per-user/path rules |
| Content | `ProcessFileContentBeforeSend(fn)` | transcode / block / transform on read |
| Thumbs | `Thumbnailer(mime, IThumbnailer)` | previews |
| HTTP | `HttpEndpoint(func(*mux.Router) error)` | add REST/SSE/WS routes |
| Static | `Static`, `StaticPatch`, `CSS`, `Favicon`, `XDGOpen` | frontend overrides |
| Search | `SearchEngine(ISearch)` | swap search impl |
| Lifecycle | `Onload`, `OnQuit`, `OnConfig`, `Middleware(fn)` | startup/shutdown/req pipeline |
| Server | `Starter(fn)` | how the server is served (http/https/tor/h2) |
| Data | `Metadata`, `AuditEngine`, `Tracer`, `DirectoryService` | pluggable sinks |
| Workflow | `WorkflowTrigger`, `WorkflowAction` | automation steps |

**Plugin conventions:** a package registered in `server/plugin/index.go`; its `func init()` calls `Backend.Register(...)` or `Hooks.Register.X(...)`. Backends implement `Init(params, app) (IBackend, error)` + `LoginForm() Form`; auth implements `Setup()/EntryPoint()/Callback()`.

## 3. Strengths

- Consistent, low-friction plugin pattern — a backend or auth method is one self-contained package.
- Broad protocol coverage (35+ backends: S3, SFTP, SMB, NFS, WebDAV, GDrive, Dropbox, IPFS, Storj, git, SQL…).
- Modern touches already present: MCP handler (SSE), a workflow engine, OnlyOffice/WOPI editors, pluggable thumbnailers.
- Clean layering: `core` (interfaces) ↔ `kernel` (registry) ↔ `plugin` (impls), fronted by `common`.

## 4. Improvement opportunities

- **A — Testing is the #1 gap:** **0 unit tests** across 388 Go files; all tests are integration-only in an external repo (`filestash-test`). Seed unit tests (crypto, path utils, `Form` JSON, `GenerateID`) + a `fakeBackend`. Wire `go test ./...` into CI.
- **B — Robustness:** a plugin's `os.Exit(1)` (the license loader does this) can kill the whole server on bad input; some helpers swallow errors. Prefer graceful degradation + panic-recovery around plugin endpoints.
- **C — Security:** secrets sit in `config.json` under one symmetric key with no external store — a Vault/SOPS resolver at `Init()` avoids creds-at-rest; no on-upload AV/DLP scanning yet.
- **D — Observability:** `ITracer`/`IAuditPlugin` exist but are empty by default — no metrics/tracing out of the box.
- **E — Multi-tenancy:** connections are global in one file; per-user/group scoping via `IAuthorisation` + DB-backed config would unlock team use.
- **F/G — Frontend & CI:** niche RxJS SPA, somewhat dated dev deps, Jenkins-only (no in-repo GitHub Actions).

## 5. Telemetry & privacy findings (reviewed for our deployment)

- **One** runtime phone-home exists: `server/pkg/middleware/telemetry.go` → every 10s POSTs batched request records to `https://downloads.filestash.app/event`.
- **Gated by `log.telemetry`, default `false`** (`config.go`). With it off, `Record()` is never called and `Flush()` returns on empty — **no POST**. Our config has it unset → off.
- If enabled it would send: Host header (internal IP/hostname), **`pathname` incl. query string — i.e. file paths/names**, client IP, user-agent, referer, backend type (`samba`), a non-reversible session hash, license, version. **Credentials never leak** (passwords are in the POST body, not the URI; `GenerateID` excludes `password`/`path`/`session`/`timestamp`).
- **No other runtime callouts** on our path. Other external hosts (Dropbox, googleapis, Backblaze, JFrog, OpenAI) are only hit by their respective backends/features. The license plugin decrypts locally. The samba backend only dials the configured NAS.
- **Local `access.log`** (`log.enable` default `true`) records request lines (incl. `pathname`) **locally only**.
- **Hardening:** set `log.telemetry=false` explicitly; firewall egress to non-NAS hosts; compile telemetry out in the fork (tracked as `F-3`).

## 6. SMB passthrough flow (confirmed)

IdP `passthrough` (strategy `direct` auto-submits; `Callback` returns form data unchanged) → `attribute_mapping` templates (`{{ .user }}`/`{{ .password }}`) fill the `related_backend` params → samba backend NTLM-dials the NAS over TCP. Nothing in that chain reports anywhere. The backend `net.DialTimeout`s **per session** — see `SMB-1` (pooling) in the plan.

---

_See `FEATURE_IDEAS.md` for the Sambee-derived catalog, `IMPLEMENTATION_PLAN.md` for the phased build, and `TRACKER.md` for status._
