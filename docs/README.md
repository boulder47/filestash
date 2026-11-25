# Fork planning docs

Planning and assessment for our Filestash fork (internal, **SMB passthrough** deployment).

| Doc | Purpose |
|-----|---------|
| [`ASSESSMENT.md`](./ASSESSMENT.md) | Codebase assessment: architecture, extension surface, strengths, improvement opportunities, **telemetry/privacy findings**, SMB-passthrough flow |
| [`FEATURE_IDEAS.md`](./FEATURE_IDEAS.md) | Feature catalog distilled from [`sambee-app/sambee`](https://github.com/sambee-app/sambee), mapped to Filestash hooks |
| [`IMPLEMENTATION_PLAN.md`](./IMPLEMENTATION_PLAN.md) | Phased build plan (Phase 0–5) with per-feature approach, data model, risks, effort |
| [`TRACKER.md`](./TRACKER.md) | Living status tracker — update in each PR |

**Start here:** read `ASSESSMENT.md` for the lay of the land, then `IMPLEMENTATION_PLAN.md` §"Recommended first slice".

Feature IDs (`F-1`, `SEC-1`, `SMB-1`, `UX-1`, `OPS-1`, `SHARE-1`, `DESK-1`) are stable across all docs.
