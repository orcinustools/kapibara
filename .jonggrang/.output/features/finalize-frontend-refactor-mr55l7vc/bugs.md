# Bug Reports — finalize-frontend-refactor

## [open] bug-001 · 2026-07-03T17:12:17.490Z
handleUpdateApp (pkg/api/handlers_app.go:145) uses orDefault for app.Domain, so PUT /apps/{id} with domain:"" cannot clear an app's ingress domain — only replace it with a non-empty value. Blocks a true 'remove domain' in the Domains & TLS UI; TLS toggles off correctly but the host persists. Needs a backend clear path (e.g. a pointer/explicit-clear field).
