---
feature: finalize-frontend-refactor
branch: feat/finalize-frontend-refactor
work_type: SMALL
description: Seal the in-progress shadcn/ui + Tailwind v4 frontend refactor by verifying gates, regenerating the embedded dist, and committing the untracked WIP as one atomic change
created_at: 2026-07-03T16:23:25.585Z
depth: deep
---

# Plan: Close M1–M9 Gaps + UI/UX Overhaul

## Approach
Re-analysis (per feedback: "beberapa belum di-implement di M1–M9 dan UI/UX-nya masih jelek") contradicts the earlier "everything is done, just commit the WIP" reading. The Go backend is ~90% real — routes and handlers exist for auth/2FA, orgs/projects, compose deploy, git+Dockerfile+Nixpacks build (`pkg/build/build.go` shells out to `docker`/`nixpacks` and imports into k3s containerd), databases, secrets/plugins, logs (HTTP chunked-flush stream), metrics (metrics-server), templates, backups, notifications, nodes, audit, preview deploy/teardown, scale/rollback. But a real audit surfaces concrete gaps, split into (a) genuinely missing M1–M9 capability and (b) UI/UX that is functional-but-ugly.

**Confirmed backend gaps:** M3 git integration is raw-URL shallow clone only (`pkg/git/git.go`) — no GitHub OAuth/App connect, no repo/branch picker, no persisted `GitProvider` credential the private-repo `Token` path can read from; metrics are instantaneous only (no history for graphs); logs stream over HTTP flush rather than WebSocket (works, keep).

**Confirmed UI/UX gaps (the bulk of the work):** the SPA covers most endpoints but is crude — it uses native `alert()`/`prompt()`/`confirm()` for deploy/scale/rollback/delete/2FA-disable (`ProjectDetail.tsx`, `Settings.tsx`), logs are a one-shot fetch with no live-follow/auto-scroll (backend already supports `follow=true`), metrics render as raw numbers in a table cell instead of CPU/RAM graphs, there is a raw unstyled `<input type="checkbox">`, no loading skeletons, and no dark/light toggle. Whole capabilities have **no UI at all**: env/secrets editor (M4), domain/TLS management beyond one create-form field (M4), autoscale/rollout config (M6 — fields exist in `appReq` but are unreachable from the UI), a deployment build-log viewer (M3/M6), preview deployments (M9), git-provider connect (M3), org member/RBAC management (M1), and backup cron/S3 config (M8).

This plan therefore *implements the missing pieces and overhauls the UI*, not "finalize + commit." Verification stays as agreed: `make build` + `make test` green, plus the embedded dist regenerated. Scope is prioritized so each phase lands usable value; lower-priority items are explicitly deferred so the change stays reviewable.

## Phases
1. Baseline gates — `cd web && npm run build` and `make build && make test`; confirm the current tree compiles green before changing anything, and re-confirm the `web/e2e.mjs` exact-string/role contracts so the overhaul does not silently break them.
2. UX foundation (highest leverage for "jelek") — add the missing shadcn primitives: `dialog`, `dropdown-menu`, `checkbox`, `tooltip`, `skeleton`, and a toast system (`sonner`); mount a `<Toaster/>` in `App.tsx`. Replace every `alert()`/`prompt()`/`confirm()` in `ProjectDetail.tsx` and `Settings.tsx` with toasts + confirm dialogs; swap the raw checkbox for the shadcn `Checkbox`; add skeleton loaders to replace "loading…" text; add a light/dark theme toggle in the sidebar.
3. Live logs + build-log viewer (M3/M6) — rewrite the Logs tab to stream with `follow=true` (incremental `fetch` reader, auto-scroll, stop/clear controls) instead of one-shot; add a deployment detail drawer/dialog that streams the build log for a running/finished deployment so "push repo → live URL" is observable.
4. Env/Secrets + Domains/TLS UI (M4) — add a per-app Env-vars key/value editor and a project/app Secrets editor wired to `/secrets` CRUD and the app `env`/`secretKeys` fields; surface domain + TLS management (host, TLS on/off, cert status) beyond the single create-form field.
5. Day-2 ops UI (M6) — replace `prompt()`-based scale/rollback with proper dialogs; expose the existing autoscale (`autoscaleMin/Max/Cpu/Memory`) and `rollout` fields in the app form/edit UI; render CPU/RAM as compact graphs/bars in Overview (lightweight inline/CSS bars or `recharts` if a dep is acceptable).
6. Remaining milestone UI (M8/M9) — backup cron schedule + S3 destination inputs (backend/`s3.go` already support it); preview-deploy trigger + teardown UI on the app row (M9); org member/RBAC management surface (M1) if low-risk. Defer git-provider OAuth (needs backend `GitProvider` entity — see Out of Scope) unless explicitly pulled in.
7. Regenerate dist, verify, commit — `make ui` so `pkg/webui/dist` matches source (old bundle deleted, new embedded via `pkg/webui/embed.go`); re-run `make build && make test`; `git add` the new components/pages/deps + rebuilt dist; land atomic commit(s) per AGENTS.md.
8. Log learnings — append to `.jonggrang/progress.txt`: the real M1–M9 state, the e2e-contract fragility, and the embedded-dist sync rule.

## Key Decisions
- Decision: re-scope from "finalize WIP" to "implement missing M1–M9 UI/UX + gaps." The feedback explicitly rejects the "already done" premise; the audit confirms real missing UI and a crude UX, so committing as-is would not satisfy the request.
- Decision: prioritize UX foundation (toasts + dialogs replacing `alert`/`prompt`/`confirm`, skeletons, checkbox) first — it is the single biggest driver of "jelek" and unblocks every later screen with consistent feedback patterns.
- Decision: add the shadcn primitives + `sonner` now. The earlier plan deferred these to avoid new deps, but a non-ugly PaaS UI cannot rely on native browser dialogs; the deps are small and idiomatic to the existing shadcn setup.
- Decision: keep HTTP-flush log streaming (do not migrate to WebSocket). It already works and supports `follow`; the gap is the *frontend* not consuming it live, not the transport.
- Decision: do NOT relabel any UI text/roles asserted in `web/e2e.mjs`. They remain the migration's binding contract; new UI must add, not rename, those anchors.
- Decision: regenerate `pkg/webui/dist` via `make ui` before committing — Go embeds the dist, so stale assets serve an old UI.
- Decision: defer git-provider OAuth/App connect (M3) — it needs a new backend `GitProvider` store entity + OAuth flow, which is a separate vertical; raw-URL + token clone already works for public/token repos.
- Decision: verification = `make build` + `make test` (offline gate); live `node web/e2e.mjs` stays out-of-band (needs a running server + orcinus cluster).

## Affected Areas
- `web/src/App.tsx` — mount `<Toaster/>`, add theme (light/dark) toggle in the sidebar; possibly add nav for new surfaces (members/preview).
- `web/src/pages/ProjectDetail.tsx` — largest change: live-follow Logs tab, deployment build-log viewer, env/secrets editor, domains/TLS UI, autoscale/rollout fields, scale/rollback dialogs, metrics graphs, preview deploy/teardown, backup cron/S3 inputs; remove all `alert`/`prompt`/`confirm`.
- `web/src/pages/Settings.tsx` — replace `alert`/`prompt` (2FA disable) with dialogs/toasts; member/RBAC management if pulled in.
- `web/src/pages/Projects.tsx`, `Cluster.tsx`, `Audit.tsx`, `Auth.tsx` — skeleton loaders, consistent empty/error states, toast feedback.
- `web/src/components/ui/` — NEW: `dialog`, `dropdown-menu`, `checkbox`, `tooltip`, `skeleton`, `sonner`/`toast`; existing: `badge, button, card, input, label, select, table, tabs, textarea`.
- `web/src/lib/utils.ts` — `cn()` (existing); possible theme helper.
- `web/src/api.ts` — add a streaming reader helper for live logs (incremental `fetch`/`ReadableStream`), extend `getText` usage.
- `web/src/styles.css` — theme tokens already present; ensure light-mode variables + toast styles.
- `web/package.json` / `package-lock.json` — new deps (radix dialog/dropdown/checkbox/tooltip, `sonner`, optional `recharts`).
- `pkg/webui/dist/` — rebuilt bundle via `make ui`; `pkg/webui/embed.go` embeds it (contract, not edited).
- Backend (only if pulled in): `pkg/store` (`GitProvider` entity), `pkg/api/handlers_*` (git connect, member mgmt) — deferred by default; see Out of Scope.

## Risks
- Risk: scope is now substantially larger than the frontmatter's SMALL/`description` suggest. Mitigation: phase-order by value and land in reviewable atomic commits; treat phases 5–6 as pull-in-if-time and defer explicitly rather than half-building.
- Risk: relabeling e2e-asserted text/roles during the overhaul silently breaks `web/e2e.mjs` with no offline signal. Mitigation: re-grep contracts in phase 1; add UI, never rename existing anchors.
- Risk: new deps (`sonner`, radix primitives, optional `recharts`) bloat the bundle / churn the lockfile. Mitigation: prefer lightweight inline/CSS metric bars over `recharts` unless charts are explicitly wanted; keep primitives to those actually used.
- Risk: embedded-dist drift — source changes without `make ui` leave the binary serving stale UI. Mitigation: run `make ui` (phase 7); prefer `make build` (chains `ui` → `build-go`).
- Risk: live log streaming via incremental `fetch` can hang connections / leak readers. Mitigation: abort controllers on unmount + explicit stop button; cap buffer size.
- Risk: untracked `web/src/components/`, `web/src/lib/`, new dist assets break others' builds if omitted. Mitigation: explicitly `git add` them in the commit.

## Alternatives Considered
- Option A — Finalize + commit the WIP as-is (the previous plan): rejected. The user explicitly says M1–M9 has unimplemented pieces and the UI is ugly; committing the current tree does not address either.
- Option B — Backend-first (git OAuth, metrics history, WebSocket logs): rejected as the primary focus. The dominant complaint is UI/UX; the backend is largely functional, so frontend + targeted UI gaps deliver far more visible value per unit effort. Backend git OAuth is deferred, not dropped.
- Option C — Full rewrite of the SPA design system: rejected. The shadcn/Tailwind v4 foundation is sound; the fix is filling in primitives, replacing native dialogs, and adding missing screens — not restarting.

## Out of Scope
- Git-provider OAuth/App connect + repo picker (M3) — needs a new backend `GitProvider` entity + OAuth flow; raw-URL/token clone already works. Deferred to a follow-up unless explicitly requested.
- Migrating log/event transport to WebSocket — current HTTP-flush streaming is functional; only the frontend live-consumption is added.
- Metrics time-series history/storage (M6) — instantaneous metrics are rendered as graphs/bars; historical retention is a separate backend feature.
- Backend milestones already implemented and compiling (auth, compose deploy, build, databases, templates, notifications, backups, preview, nodes) — no re-implementation, only wiring missing UI to them.
- Any change to `/api/v1` call shapes beyond additive helpers; no breaking `web/src/api.ts` contract changes.
- Fixing the aspirational `make e2e` target (`go test ./test/e2e/...`, directory absent).
- Live browser e2e execution requiring a running server + orcinus cluster.
- The M9 multi-app prune isolation caveat (tracked separately in memory) — unrelated to this work.

## Dependencies
- Already installed (`web/package.json`): `@radix-ui/react-{label,slot,tabs}`, `class-variance-authority`, `clsx`, `tailwind-merge`, `lucide-react`, `react@18.3`, `react-dom`, `react-router-dom@6.26`; dev: `@tailwindcss/vite@^4.3`, `tailwindcss@^4.3`, `@vitejs/plugin-react`, `typescript@5.5`, `vite@5.4`, `playwright@1.61`.
- NEW deps to add (phase 2+): `@radix-ui/react-dialog`, `@radix-ui/react-dropdown-menu`, `@radix-ui/react-checkbox`, `@radix-ui/react-tooltip`, and `sonner` (toasts). Optional: `recharts` for metric graphs (prefer lightweight inline/CSS bars to avoid it).
- Existing patterns: shadcn/ui conventions (`cva` variants, `cn()` merge, radix primitives), `@/*` import alias (Vite + tsconfig paths), `ui.tsx` legacy-bridge shims, HSL design tokens via `@theme inline`.
- Build/test infra: `cd web && npm run build` (tsc -b + vite build) as the typecheck gate; `make test` → `go test ./...`; `make build` = `ui` then `build-go` keeps embedded dist in sync; `pkg/webui/embed.go` embeds `pkg/webui/dist`.
- Backend endpoints the new UI consumes (all already present): `/secrets` CRUD, `/projects/{p}/logs?follow=true`, `/deployments/{id}`, `/projects/{p}/services/{s}/{scale,rollback}`, `/apps/{id}/preview` + `/projects/{p}/preview/{branch}`, `/projects/{p}/backups`, `/projects/{p}/metrics`.

<!-- jonggrang:clarifications -->
## Clarifications
_Captured from the planning Q&A:_

Goal: User wants me to read PLAN.md, identify which tasks/milestones are still unfinished, and produce an implementation plan to complete them. The blocker: PLAN.md has no per-task status markers, the README claims all milestones M0-M9 are already done and e2e-verified, the Go backend compiles with routes for every feature, yet there is a large uncommitted frontend refactor in progress and no active Jonggrang tasks. What counts as 'unfinished' is therefore ambiguous.

- **PLAN.md milestones M0-M9 are all present in code and the README says they're done & e2e-verified. What is the actual gap you want this plan to close?** → Finish the in-progress frontend refactor
- **How should the plan treat the uncommitted frontend changes (App.tsx, all pages, new components/ and lib/ dirs)?** → Build on / complete this WIP
- **What does 'selesai' (done) require for verification in the plan?** → Build + existing tests pass

**Revision (user feedback):** "coba analisis lagi beberapa belum di implement di m1-m9 itu dan UI/UXnya masih jelek" — re-audit confirmed the backend is largely real but there ARE genuine gaps (git-provider OAuth, metrics history) and, more importantly, significant missing UI (env/secrets, domains/TLS, autoscale/rollout, live logs, build-log viewer, preview deploy, member/RBAC, backup cron/S3) plus a crude UX (native alert/prompt/confirm, no toasts/dialogs/skeletons, raw checkbox, no metric graphs, no theme toggle). Plan re-scoped from "finalize + commit WIP" to "implement missing M1–M9 UI/UX gaps + overhaul the ugly UI."
</content>
