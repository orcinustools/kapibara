package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/orcinustools/kapibara/pkg/backup"
	"github.com/orcinustools/kapibara/pkg/store"
)

type backupReq struct {
	DatabaseID  string            `json:"databaseId"`
	Cron        string            `json:"cron"`
	Destination string            `json:"destination"` // local | s3
	S3Config    map[string]string `json:"s3Config"`
	Enabled     bool              `json:"enabled"`
}

func (s *Server) handleCreateBackup(w http.ResponseWriter, r *http.Request) {
	p := s.loadProjectWithAccess(w, r)
	if p == nil {
		return
	}
	var req backupReq
	if !decodeJSON(w, r, &req) {
		return
	}
	db, err := s.Store.DatabaseByID(req.DatabaseID)
	if err != nil || db.ProjectID != p.ID {
		writeError(w, http.StatusBadRequest, "database not found in project")
		return
	}
	if req.Destination == "" {
		req.Destination = "local"
	}
	s3, _ := json.Marshal(req.S3Config)
	b := &store.Backup{
		DatabaseID: db.ID, ProjectID: p.ID, Cron: req.Cron,
		Destination: req.Destination, S3Config: string(s3), Enabled: req.Enabled,
	}
	if err := s.Store.CreateBackup(b); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	p := s.loadProjectWithAccess(w, r)
	if p == nil {
		return
	}
	bs, err := s.Store.BackupsForProject(p.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"backups": bs})
}

func (s *Server) handleRunBackup(w http.ResponseWriter, r *http.Request) {
	b, err := s.Store.BackupByID(chi.URLParam(r, "backupID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	p, err := s.Store.ProjectByID(b.ProjectID)
	if err != nil || s.requireOrgAccess(w, r, p.OrganizationID) == nil {
		if err != nil {
			writeError(w, http.StatusNotFound, "project not found")
		}
		return
	}
	path, runErr := s.runBackup(r.Context(), b)
	if runErr != nil {
		writeError(w, http.StatusBadGateway, runErr.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "path": path, "backup": b})
}

// runBackup executes a backup and records its outcome on the config.
func (s *Server) runBackup(ctx context.Context, b *store.Backup) (string, error) {
	db, err := s.Store.DatabaseByID(b.DatabaseID)
	if err != nil {
		return "", err
	}
	runner := &backup.Runner{Kube: s.Kube, Namespace: s.Cfg.Namespace, DataDir: s.Cfg.DataDir}
	var s3cfg map[string]string
	_ = json.Unmarshal([]byte(b.S3Config), &s3cfg)

	now := time.Now()
	path, err := runner.Run(ctx, db, s3cfg, b.Destination)
	b.LastRunAt = &now
	if err != nil {
		b.LastStatus = "failed"
		b.LastError = err.Error()
		_ = s.Store.UpdateBackup(b)
		return "", err
	}
	b.LastStatus = "success"
	b.LastError = ""
	b.LastPath = path
	_ = s.Store.UpdateBackup(b)
	return path, nil
}

// runDueBackups is invoked by the scheduler to run backups whose cron is due.
func (s *Server) runDueBackups(ctx context.Context) {
	bs, err := s.Store.EnabledScheduledBackups()
	if err != nil {
		return
	}
	for i := range bs {
		if backupDue(&bs[i]) {
			_, _ = s.runBackup(ctx, &bs[i])
		}
	}
}
