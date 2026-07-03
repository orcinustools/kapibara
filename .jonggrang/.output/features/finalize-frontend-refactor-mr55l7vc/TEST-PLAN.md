# Test Plan — finalize-frontend-refactor

Feature: `finalize-frontend-refactor-mr55l7vc` · Work type: SMALL · Phase 13 (test-planning)
Generated: 2026-07-03

## 1. Scope & nature of the change

100% of the source diff (`git diff main...HEAD`) lands under `web/src/**` — 26 files:
App/auth/api/main, `styles.css`, 6 pages (Audit, Auth, Cluster, ProjectDetail, Projects,
Settings), 15 `components/ui/*` primitives, `lib/utils.ts`, `ui.tsx`. **No Go source changed**
(`go build ./...` is green). The plus is a UI/UX overhaul + wiring previously-unreachable
backend capabilities (M3–M9) into the SPA.

Consequence: the risk surface is **frontend behavior + the embedded-dist seal**, not backend
logic. The Go test suite is a *regression guard only*, not the primary target of new tests.

## 2. Agreed acceptance (from plan.md "Key Decisions")

> Verification = `make build` + `make test` green, plus the embedded dist regenerated.
> Live `node web/e2e.mjs` stays out-of-band (needs a running server + orcinus cluster).

This plan does **not** widen that contract. It structures it into an executable gate for
Phase 14 and enumerates the manual/out-of-band checks that give real behavioral coverage.

## 3. Test layers & what runs where

| Layer | Tool | Runs offline (Phase 14)? | Purpose |
|-------|------|--------------------------|---------|
| Typecheck + bundle | `cd web && npm run build` (`tsc -b && vite build`) | ✅ yes | Compile-time contract for all 26 changed TS/TSX files; the only automated frontend gate available |
| Embedded-dist sync | `make ui` twice → `git status pkg/webui/dist` empty | ✅ yes | Proves the served bundle matches source (see progress.txt §3) |
| Backend regression | `make test` (`go test ./...`) | ✅ yes | No backend change → must stay green (auth_flow, deploy, server tests) |
| e2e-anchor contract | grep of `web/e2e.mjs` literals/roles in `web/src` | ✅ yes | Offline proxy for e2e — the build gate can NOT catch anchor drift (progress.txt §2) |
| Browser e2e | `node web/e2e.mjs` (Playwright) | ❌ out-of-band | Real login→project→compose-deploy→pod-visible flow; needs live binary + orcinus |
| Feature smoke | Manual checklist §6 | ❌ out-of-band | Exercises the newly-wired M3–M9 surfaces the e2e script does not touch |

### Decision: do NOT introduce a frontend unit-test framework

There is no vitest/jest/RTL in `web/package.json` today (only `playwright`, and no test
script). Adding a runner + config + jsdom is a new-dependency vertical that exceeds a SMALL
feature and is outside the agreed "build + existing tests pass" contract. Component-level
behavior is instead covered by the tsc typecheck (props/wiring) + the browser e2e/manual smoke
(behavior). **Recorded as a follow-up**, not part of this feature's testing.

## 4. e2e binding-anchor contract (verified intact at plan time)

`web/e2e.mjs` asserts exact strings tied to ARIA roles. `make build`/`npm run build` stay green
even if these drift — so Phase 14 MUST re-grep them. Verified present in current source:

| Anchor | Kind | Location (verified) |
|--------|------|---------------------|
| `you@example.com` | placeholder | Auth.tsx (1) |
| `min 8 chars` | placeholder | Auth.tsx (1) |
| `new project name` | placeholder | Projects.tsx (1) |
| `Your projects` | getByText | Projects.tsx (1) |
| `Create project` | button | Projects.tsx (1) |
| `Login` (exact) | button | Auth.tsx:83 `{mode==="login" ? "Login" : "Register"}` |
| `Deploy` (exact) | button | ProjectDetail.tsx Compose card |
| `Compose` / `Overview` | link (role) | TabBar `NavLink`, label = capitalized tab key |
| `Applied \d+ objects` | getByText regex | ProjectDetail.tsx:562 `setOut(\`Applied ${r.applied} objects.\`)` |
| `web-` | getByText prefix | Overview pod rows |

Rule (progress.txt §2): `Login` & `Deploy` use `exact:true` — spinners must be `aria-hidden`
and label text unchanged; never append/prefix. Overview must keep rendering `web-*` pod rows;
Compose deploy must keep surfacing `Applied N objects`.

## 5. Offline test procedure (Phase 14 — deterministic, runnable here)

Run in order; each must pass before the next:

```bash
# T1 — frontend typecheck + bundle (primary automated gate)
cd web && npm install && npm run build          # tsc -b && vite build → must exit 0

# T2 — e2e anchor re-grep (offline proxy for e2e; see §4 table)
cd web/src
for s in "you@example.com" "min 8 chars" "new project name" "Your projects" \
         "Create project"; do grep -qF "$s" -r . || echo "MISSING: $s"; done
grep -qF 'Applied ' -r . || echo "MISSING: Applied N objects"
# manual-confirm exact-role anchors: Login button, Deploy button, Compose/Overview links, web- pods

# T3 — embedded-dist sync seal (progress.txt §3)
cd /home/ibnu/research/kapibara
make ui                                          # regenerate pkg/webui/dist
make ui                                          # deterministic 2nd pass
git status --short pkg/webui/dist                # MUST be empty → dist == source

# T4 — backend regression (no backend change → must stay green)
make test                                        # go test ./... → exit 0

# T5 — full build (chains ui → build-go; final seal)
make build                                        # exit 0
```

Pass criteria: T1/T4/T5 exit 0; T2 prints no `MISSING:`; T3 shows an empty `git status`.

## 6. Out-of-band behavioral coverage (manual / live cluster)

Requires a running binary against orcinus (see memory `kapibara-build-verified`). Not part of
the Phase 14 offline gate, but the true acceptance for the wired-up features. Run
`node web/e2e.mjs` (KAPIBARA_URL) for the core flow, then smoke each new surface:

| # | Surface (task) | Check |
|---|----------------|-------|
| S1 | Core e2e (task-001/014) | `node web/e2e.mjs` → "ALL UI E2E CHECKS PASSED ✅" |
| S2 | Toasts/dialogs (task-002/003) | deploy/scale/rollback/delete/2FA-disable show toasts + confirm dialogs, no native `alert/prompt/confirm` |
| S3 | Skeletons/empty states (task-004) | list pages show skeletons then data or a styled empty state |
| S4 | Live logs (task-005) | Logs tab follows (`follow=true`), auto-scrolls, stop/clear work, no leaked reader on unmount |
| S5 | Build-log viewer (task-006) | deployment drawer streams build log for a running/finished deploy |
| S6 | Env/Secrets (task-007) | key/value editor saves; write-only fields shown as set-on-save (no fake read-back) |
| S7 | Domains/TLS (task-008) | host + TLS toggle persist. **Known limitation bug-001**: domain cannot be cleared to "" (backend `orDefault`) — verify copy reflects this, do not assert clear works |
| S8 | Scale/rollback + autoscale/rollout (task-009) | dialogs replace prompts; autoscale/rollout are create-only per backend |
| S9 | CPU/RAM bars (task-010) | Overview renders comparative CSS bars (instantaneous, not %-of-limit) |
| S10 | Backup cron + S3 (task-011) | schedule + S3 destination inputs save (S3Config is write-only) |
| S11 | Preview deploy/teardown (task-012) | app-row trigger + teardown |
| S12 | Member/RBAC (task-013) | surface renders without error (backend member routes limited) |

## 7. Known limitations feeding test expectations (do NOT file as new bugs)

- **bug-001 (open, backend, out of scope):** `handleUpdateApp` `orDefault` prevents clearing an
  app domain to `""`. S7 must assert "replace domain" not "clear domain".
- **Write-only fields** (`json:"-"`): `Application.Env`/`SecretKeys`, `Backup.S3Config`,
  notification `Config` are never returned — UI is set-on-save; do not test for read-back.
- **Autoscale/rollout** are create-only (`handleUpdateApp` ignores them).
- **Metrics** are instantaneous (no history) → bars are comparative-across-peers only.
- **`make e2e`** target (`go test ./test/e2e/...`) references an absent dir — not exercised; use
  `node web/e2e.mjs` for live e2e.

## 8. Phase 14 exit checklist

- [ ] T1 `npm run build` exits 0
- [ ] T2 all e2e anchors present (§4)
- [ ] T3 `git status pkg/webui/dist` empty after double `make ui`
- [ ] T4 `make test` green
- [ ] T5 `make build` green
- [ ] validation.tests_passed set true only if T1–T5 pass
- [ ] Out-of-band S1–S12 (§6) run if a live cluster is available; otherwise recorded as deferred
