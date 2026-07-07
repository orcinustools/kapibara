# Phase 16 — Test Quality

Feature: `finalize-frontend-refactor` · Work type: SMALL · Date: 2026-07-03
Purpose: no low-value tests, correct assertions.

## Verdict: PASS

This feature is frontend-only (26 files under `web/src/**` + regenerated `pkg/webui/dist`).
It **introduced zero new automated test files** — a deliberate, documented decision
(TEST-PLAN.md §3, TEST-RESULTS.md §36): adding vitest/jest/RTL exceeds a SMALL feature and the
agreed "build + existing tests pass" contract. Consequently there are **no new low-value or
tautological tests to remove**, and no new assertions to correct.

## 1. Behavioral test relevant to the feature — `web/e2e.mjs`

Real Playwright flow: login → Projects → create project → Compose deploy → assert
`Applied N objects` → Overview pod visible. Assertions are meaningful, not smoke:

- Every step is a `waitFor` on a real UI transition (throws on timeout = a genuine assertion),
  not a bare navigation.
- `/Applied \d+ objects/` regex requires a real deploy-result count (≥1 digit), and the text
  is read back and logged — not a fixed string match.
- Pod row asserted via the dynamic `web-` prefix rendered from `/pods` at runtime.

**Anchor correctness re-verified against current source** (a stale anchor = an incorrect
assertion). All 10 present and correctly targeted:

| Anchor | Source (verified) |
|--------|-------------------|
| `you@example.com`, `min 8 chars` | Auth.tsx placeholders |
| `new project name`, `Your projects`, `Create project` | Projects.tsx |
| `Login` (exact button) | Auth.tsx:83 `{mode==="login" ? "Login" : "Register"}` |
| `Deploy` (exact button) | ProjectDetail.tsx:575 compose card `{busy ? "Deploying…" : "Deploy"}` |
| `Applied \d+ objects` | ProjectDetail.tsx:562 `setOut(\`Applied ${r.applied} objects.\`)` |
| `Compose` / `Overview` links | TabBar capitalized tab labels |
| `web-` pod prefix | ProjectDetail.tsx:116 `{p.name}` dynamic |

## 2. Pre-existing Go tests backing the SPA flow (not modified by this feature)

Reviewed for quality since they are the regression guard for the wired `/api/v1` handlers:

- Real assertion density, no smoke-only tests: auth_flow (16 assertions/2 tests),
  deploy (9/2), server (9/2) — 34 real `t.Error*/t.Fatal*` across 6 tests.
- No tautologies, skipped-body, `if true`, or commented-out assertions found across
  `pkg/{api,compose,database,orcinus}` test files.
- Full suite green: `go test ./...` → 0 failures, 0 panics.

## 3. Observation (not a defect for this feature — no fix applied)

Two buttons render exact accessible name `Deploy` (app-row ProjectDetail.tsx:283 and compose
card :575). `web/e2e.mjs` uses `getByRole("button", {name:"Deploy", exact:true})` and relies on
tab-scoping (Compose tab active) so only one is present when clicked. Pre-existing behavior,
exercised out-of-band; noted for the future e2e-hardening track, not changed here.

## Conclusion

No low-value tests introduced; no incorrect assertions. The one feature-relevant behavioral
test has correct, meaningful assertions verified against source; the backing Go tests are
non-tautological and green. Phase 16 gate satisfied.
