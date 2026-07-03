# Test Results — finalize-frontend-refactor (Phase 14, testing)

Executed: 2026-07-03 · Work type: SMALL · Offline gate per TEST-PLAN.md §5

## Offline gate (deterministic, run here)

| # | Gate | Command | Result |
|---|------|---------|--------|
| T1 | Frontend typecheck + bundle | `cd web && npm run build` (`tsc -b && vite build`) | ✅ exit 0 — 1874 modules, dist emitted (index-CsCKrj5Z.css / index-BEPlGRxj.js) |
| T2 | e2e anchor re-grep | grep of `web/e2e.mjs` literals/roles in `web/src` | ✅ no `MISSING:` — all anchors intact |
| T3 | Embedded-dist sync seal | `make ui` ×2 → `git status pkg/webui/dist` | ✅ empty diff (deterministic, dist == source) |
| T4 | Backend regression | `make test` (`go test ./...`) | ✅ exit 0 — pkg/api, compose, database, orcinus ok (cached); rest no-test |
| T5 | Full build | `make build` (chains ui → build-go) | ✅ exit 0 — bin/kapibara built; dist still sealed |

### T2 anchor detail (all present)
- `you@example.com`, `min 8 chars`, `new project name`, `Your projects`, `Create project` — present
- `Login` (exact) — Auth.tsx:83 `{mode==="login" ? "Login" : "Register"}`
- `Deploy` (exact) — ProjectDetail.tsx:283 `>Deploy</Button>`
- `Applied N objects` — ProjectDetail.tsx:562 `setOut(\`Applied ${r.applied} objects.\`)`
- `Compose` / `Overview` links — TabBar default capitalization (`t[0].toUpperCase()+t.slice(1)`); TAB_LABELS overrides only config/domains
- `web-` pod rows — Overview renders `{p.name}` from `/pods` at runtime (dynamic)

**Pass criteria (§5):** T1/T4/T5 exit 0 ✅ · T2 no MISSING ✅ · T3 empty git status ✅ → `validation.tests_passed = true`.

## Out-of-band behavioral coverage (S1–S12, TEST-PLAN.md §6)

**Status: DEFERRED.** Requires a running binary against a live orcinus cluster (see memory
`kapibara-build-verified`) — not available in this non-interactive session and explicitly
out-of-band per the agreed acceptance contract (plan.md Key Decisions). To run when a cluster
is available: `node web/e2e.mjs` (KAPIBARA_URL) for S1 core flow, then smoke S2–S12.

Known limitations (TEST-PLAN.md §7 — do NOT file as new bugs): bug-001 domain-clear `orDefault`;
write-only fields (Env/SecretKeys/S3Config/notify Config); autoscale/rollout create-only;
instantaneous metrics; `make e2e` target references absent dir (use `node web/e2e.mjs`).

## No new frontend unit-test framework introduced
Consistent with TEST-PLAN.md §3 — adding vitest/jest/RTL exceeds a SMALL feature and the agreed
"build + existing tests pass" contract. Recorded as a follow-up, not part of this feature.
