# Feature Ideas — distilled from Sambee

Source: [`sambee-app/sambee`](https://github.com/sambee-app/sambee) — a self-hosted, browser-based, **SMB-first** file manager (Python/FastAPI backend, React/TS frontend, Tauri desktop "Companion"). Ideas below are de-duplicated against what Filestash already ships and mapped to Filestash extension points. IDs match `IMPLEMENTATION_PLAN.md` / `TRACKER.md`.

## A. Security & access _(most relevant to an internal SMB box)_
- **SEC-2 — Generic OIDC/OAuth2 SSO.** Sambee ships a full OIDC stack; Filestash has only LDAP/WordPress/passthrough. → `AuthenticationMiddleware`.
- **SEC-1 — Change-audit logging + viewer.** create/rename/delete/modify (not reads), dedup rapid re-saves, retention caps, admin viewer with filters. Filestash's `IAuditPlugin` is unimplemented.
- **SEC-3 — Auth rate-limiting / lockout.** per-identity + per-IP throttle; extends `plg_security_authlock`.
- **SEC-4 — Per-directory access policy/grants.** grant/deny by connection+path+action. → `IAuthorisation`.

## B. SMB performance & correctness
- **SMB-1 — Connection pooling/reuse.** thread-safe pool keyed by (host, creds); Sambee adds SMB "credit" accounting. Filestash dials per session.
- **SMB-2 — Directory listing cache + index.** cut latency on chatty/latent SMB.
- **SMB-3 — File lock manager.** prevent concurrent-edit clobbering on shared files.
- **SMB-4 — Live directory monitoring → WebSocket push.** listings update live for multiple users.

## C. Viewers & editing (frontend)
- **UX-1 — Robust PDF viewer** that repairs problematic PDFs + rendered-derivative cache.
- **UX-2 — Markdown WYSIWYG** with table editing.
- **UX-3 — Draft recovery / autosave** — recover unsaved edits after disconnect.
- **UX-4 — In-browser archive (zip)** browse / extract / create (resumable).
- **UX-5 — Image viewer breadth** — multi-page TIFF/ICO, medical DCM, ImageMagick preprocessor.
- **UX-6 — Delete action in every viewer** (confirm + `Del` shortcut).

## D. Collaboration & ops
- **OPS-1 — Health dashboard** — SMB reachability / disk / memory snapshot + admin dialog.
- **OPS-2 — Cross-connection copy/move** — between two shares/backends in one op.
- **OPS-3 — Recent files / recent directories** — extend `plg_widget_recent`.
- **OPS-4 — Mobile-first UI + theming.**
- **OPS-5 — Prometheus `/metrics`** — good first standalone plugin.

## E. Sharing _(security-first, fits an internal-only box)_
- **SHARE-1 — E2E-encrypted public share links via an isolated public container.** internal backend never exposed; public container holds only encrypted blobs (keys never land there); link embeds key; password/expiry/max-downloads; upload links too. Safer than built-in shares for an internal NAS.

## F. Desktop integration _(biggest differentiator, largest effort)_
- **DESK-1 — Desktop "Companion" helper.** open files in native apps (Word/Excel/Photoshop), edit, auto-upload back; local-drive access; PIN pairing. Filestash only has an `xdg-open` hook.

---

**Already in Filestash (not re-proposed):** pluggable storage/plugin system, CodeMirror/OnlyOffice/WOPI editors, image thumbnailers + video transcoder, sqlite-FTS & stateless search, built-in share links, workflow engine, MCP handler.
