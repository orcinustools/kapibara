// Package api is the kapibara control-plane HTTP server: REST API + embedded UI.
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/orcinustools/kapibara/pkg/auth"
	"github.com/orcinustools/kapibara/pkg/config"
	"github.com/orcinustools/kapibara/pkg/deployer"
	"github.com/orcinustools/kapibara/pkg/kube"
	"github.com/orcinustools/kapibara/pkg/orcinus"
	"github.com/orcinustools/kapibara/pkg/store"
	"github.com/orcinustools/kapibara/pkg/version"
	"github.com/orcinustools/kapibara/pkg/webui"
)

// Server holds shared dependencies for HTTP handlers.
type Server struct {
	Cfg      config.Config
	Store    *store.Store
	Orcinus  *orcinus.Client
	Auth     *auth.Manager
	Deployer *deployer.Deployer
	Kube        *kube.Client
	metrics     *metricsHistory
	oauthStates *oauthStateStore
	router      chi.Router
}

// New builds a Server and wires up routes.
func New(cfg config.Config, st *store.Store) *Server {
	oc := orcinus.New(cfg.OrcinusURL, cfg.OrcinusToken)
	// Direct cluster access is optional: if the kubeconfig is missing the server
	// still runs; logs/metrics endpoints then return 503.
	kc, err := kube.New(cfg.Kubeconfig)
	if err != nil {
		kc = nil
	}
	s := &Server{
		Cfg:     cfg,
		Store:   st,
		Orcinus: oc,
		Auth:    auth.NewManager(cfg.JWTSecret),
		Kube:        kc,
		metrics:     newMetricsHistory(120),
		oauthStates: newOAuthStateStore(),
		Deployer: deployer.New(st, oc, deployer.Config{
			RegistryPrefix:   cfg.RegistryPrefix,
			Push:             cfg.BuildPush,
			ClusterContainer: cfg.ClusterContainer,
			DataDir:          cfg.DataDir,
		}),
	}
	// Wire deploy notifications through the org's configured channels.
	s.Deployer.Dispatch = s.dispatchNotifications
	s.routes()
	return s
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler { return s.router }

func (s *Server) routes() {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(150 * time.Second))

	// Open endpoints.
	r.Get("/healthz", s.handleHealthz)
	r.Get("/version", s.handleVersion)

	// API v1.
	r.Route("/api/v1", func(r chi.Router) {
		// Public auth endpoints.
		r.Post("/auth/register", s.handleRegister)
		r.Post("/auth/login", s.handleLogin)

		// Public webhook endpoint (authorized by the path secret).
		r.Post("/webhooks/{secret}", s.handleWebhook)

		// Public OAuth callback (the provider redirects the browser here; the
		// signed state links it back to the org/user that started the flow).
		r.Get("/git-providers/oauth/{type}/callback", s.handleGitOAuthCallback)

		// Authenticated endpoints.
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Use(s.auditRecorder)

			r.Get("/auth/me", s.handleMe)
			r.Post("/auth/2fa/enroll", s.handle2FAEnroll)
			r.Post("/auth/2fa/verify", s.handle2FAVerify)
			r.Post("/auth/2fa/disable", s.handle2FADisable)
			r.Get("/tokens", s.handleListTokens)
			r.Post("/tokens", s.handleCreateToken)

			r.Get("/cluster", s.handleCluster)
			r.Get("/engine/version", s.handleEngineVersion)

			// Organizations.
			r.Get("/orgs", s.handleListOrgs)
			r.Post("/orgs", s.handleCreateOrg)

			// Org members / RBAC (M1). Any member can list; owners/admins
			// manage. Add is by existing-user email (no invite flow).
			r.Get("/orgs/{orgID}/members", s.handleListMembers)
			r.Post("/orgs/{orgID}/members", s.handleAddMember)
			r.Put("/orgs/{orgID}/members/{userID}", s.handleUpdateMember)
			r.Delete("/orgs/{orgID}/members/{userID}", s.handleRemoveMember)

			// Git providers (M3): connect a source-control account (PAT or
			// OAuth), list its repositories for the repo picker.
			r.Get("/orgs/{orgID}/git-providers", s.handleListGitProviders)
			r.Post("/orgs/{orgID}/git-providers", s.handleCreateGitProvider)
			r.Get("/orgs/{orgID}/git-providers/oauth/{type}/start", s.handleGitOAuthStart)
			r.Delete("/git-providers/{providerID}", s.handleDeleteGitProvider)
			r.Get("/git-providers/{providerID}/repos", s.handleListGitProviderRepos)

			// Projects (scoped to an org via query/body).
			r.Get("/orgs/{orgID}/projects", s.handleListProjects)
			r.Post("/orgs/{orgID}/projects", s.handleCreateProject)
			r.Get("/projects/{projectID}", s.handleGetProject)
			r.Delete("/projects/{projectID}", s.handleDeleteProject)

			// Compose apps + deploy (M2).
			r.Get("/projects/{projectID}/compose", s.handleListComposeApps)
			r.Post("/projects/{projectID}/compose", s.handleCreateComposeApp)
			r.Put("/projects/{projectID}/compose/{appID}", s.handleUpdateComposeApp)
			r.Post("/projects/{projectID}/convert", s.handleConvert)
			r.Post("/projects/{projectID}/deploy", s.handleDeploy)
			r.Get("/projects/{projectID}/pods", s.handleProjectPods)
			r.Get("/projects/{projectID}/deployments", s.handleListDeployments)

			// Applications: Git/image → build → deploy (M3).
			r.Get("/projects/{projectID}/apps", s.handleListApps)
			r.Post("/projects/{projectID}/apps", s.handleCreateApp)
			r.Get("/apps/{appID}", s.handleGetApp)
			r.Put("/apps/{appID}", s.handleUpdateApp)
			r.Delete("/apps/{appID}", s.handleDeleteApp)
			r.Post("/apps/{appID}/deploy", s.handleDeployApp)
			r.Get("/deployments/{deploymentID}", s.handleGetDeployment)
			r.Post("/deployments/{deploymentID}/redeploy", s.handleRedeployDeployment)

			// Secrets + plugins (M4): domains/TLS are configured per-app and
			// rendered as x-orcinus-expose/host/tls in the generated compose.
			r.Get("/secrets", s.handleListSecrets)
			r.Post("/secrets", s.handlePutSecret)
			r.Delete("/secrets/{name}", s.handleDeleteSecret)
			r.Get("/plugins", s.handleListPlugins)
			r.Post("/plugins/{name}", s.handleInstallPlugin)
			r.Delete("/plugins/{name}", s.handleRemovePlugin)

			// Databases: one-click managed engines (M5).
			r.Get("/projects/{projectID}/databases", s.handleListDatabases)
			r.Post("/projects/{projectID}/databases", s.handleCreateDatabase)
			r.Get("/databases/{dbID}", s.handleGetDatabase)
			r.Post("/databases/{dbID}/deploy", s.handleDeployDatabase)
			r.Delete("/databases/{dbID}", s.handleDeleteDatabase)

			// Day-2 ops: scale, rollback, logs, metrics (M6).
			r.Post("/projects/{projectID}/services/{service}/scale", s.handleScaleService)
			r.Post("/projects/{projectID}/services/{service}/rollback", s.handleRollbackService)
			r.Get("/projects/{projectID}/logs", s.handleLogs)
			r.Get("/projects/{projectID}/metrics", s.handleMetrics)

			// Templates: one-click apps (M7).
			r.Get("/templates", s.handleListTemplates)
			r.Post("/projects/{projectID}/templates/{name}", s.handleDeployTemplate)

			// Notifications, backups (M8).
			r.Get("/orgs/{orgID}/notifications", s.handleListNotifications)
			r.Post("/orgs/{orgID}/notifications", s.handleCreateNotification)
			r.Delete("/notifications/{notifID}", s.handleDeleteNotification)
			r.Get("/projects/{projectID}/backups", s.handleListBackups)
			r.Post("/projects/{projectID}/backups", s.handleCreateBackup)
			r.Post("/backups/{backupID}/run", s.handleRunBackup)

			// Multi-node, preview deploys, audit (M9).
			r.Get("/nodes", s.handleListNodes)
			r.Get("/audit", s.handleAuditLog)
			r.Post("/apps/{appID}/preview", s.handlePreviewDeploy)
			r.Delete("/projects/{projectID}/preview/{branch}", s.handlePreviewTeardown)
		})
	})

	// Embedded web dashboard (SPA): assets + client-side routing fallback.
	// Registered as a catch-all so /projects/:id etc. resolve to index.html;
	// API routes above take precedence.
	r.Handle("/*", webui.Handler())

	s.router = r
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"engineHealthy": s.Orcinus.Healthy(r.Context()),
	})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"version":   version.Version,
		"gitCommit": version.GitCommit,
		"component": "kapibara",
	})
}

func (s *Server) handleCluster(w http.ResponseWriter, r *http.Request) {
	cs, err := s.Orcinus.Cluster(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cs)
}

func (s *Server) handleEngineVersion(w http.ResponseWriter, r *http.Request) {
	v, err := s.Orcinus.Version(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html><head><title>Kapibara</title></head>` +
		`<body style="font-family:sans-serif;max-width:40rem;margin:4rem auto">` +
		`<h1>🦫 Kapibara</h1><p>Control-plane is running. UI is served here once built.</p>` +
		`<p>API: <code>/api/v1</code> · Health: <code>/healthz</code> · Version: <code>/version</code></p>` +
		`</body></html>`))
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
