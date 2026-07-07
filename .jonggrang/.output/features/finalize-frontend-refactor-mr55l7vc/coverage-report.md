# Phase 15 — Coverage Verification

Feature: `finalize-frontend-refactor` (work type: SMALL)
Date: 2026-07-03

## Result: threshold NOT met (literal), but not applicable to delivered scope

- Configured threshold (`.jonggrang/jonggrang.json` → `testing.coverage_threshold`): **80%**
- Measured Go statement coverage (total): **20.0%**
- Test suite status: **all green** (`go test ./...` — 0 failures, 0 panics)

## Per-package Go coverage

| Package | Coverage |
|---|---|
| pkg/database | 86.5% |
| pkg/compose | 71.4% |
| pkg/orcinus | 42.1% |
| pkg/api | 25.7% |
| cmd/kapibara, pkg/{auth,backup,build,config,deployer,git,kube,notify,store,templates,webui} | 0.0% |
| pkg/version | no test files |

## Why the 80% gate does not apply to this feature

- The feature is a **frontend-only refactor**. Every commit in scope is `feat(web): …`
  (React/TypeScript SPA under `web/`, plus the regenerated embedded `pkg/webui/dist` bundle).
- No Go source was added or changed by this feature, so it introduces **no new untested
  Go statements** — the 20% total is pre-existing backend baseline, unchanged by this work.
- The frontend has **no unit-test / coverage tooling** (`web/package.json` has no
  vitest/jest/@testing-library/c8). Its only automated coverage is Playwright e2e
  (`make e2e`, gated behind `KAPIBARA_E2E=1`), which is not statement-instrumented and not
  part of `make test`.
- The 80% threshold is a Go-template default (`stack: go`, `testing.framework: none`) and was
  never tuned for this hybrid go+frontend repo.

## Feature-relevant API handler coverage (backs the SPA flow)

The `/api/v1` handlers the refactored SPA depends on are exercised by
`auth_flow_test.go`, `deploy_test.go`, `server_test.go`:

- server.go `New`/`routes`/`Handler`/`writeJSON`/`writeError`: 100%
- handlers_deploy.go `composeTarget`: 100%, `handleConvert`: 64%, `handleListDeployments`: 62%, `handleDeploy`: 59%
- server.go `handleCluster`: 60%

## Recommendation

The delivered frontend code cannot be measured by the Go 80% gate and adds no untested Go
code, so the gate is a scope mismatch rather than a real regression. Options for the
orchestrator/human:

1. **Waive** the coverage gate for this frontend-only feature (recommended — no Go delta).
2. **Retune** `testing.coverage_threshold` for the hybrid repo, or scope it to changed Go
   packages only.
3. If Go backend coverage is a separate goal, address it under the `harden_release` track
   (see `.plan-questions.json`), not this frontend refactor.
