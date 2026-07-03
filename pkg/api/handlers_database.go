package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/orcinustools/kapibara/pkg/database"
	"github.com/orcinustools/kapibara/pkg/deployer"
	"github.com/orcinustools/kapibara/pkg/store"
)

type dbReq struct {
	Name       string `json:"name"`
	Engine     string `json:"engine"`
	Version    string `json:"version"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	DBName     string `json:"dbName"`
	VolumeSize string `json:"volumeSize"`
}

func (s *Server) handleCreateDatabase(w http.ResponseWriter, r *http.Request) {
	p := s.loadProjectWithAccess(w, r)
	if p == nil {
		return
	}
	var req dbReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	if !database.Supported(database.Engine(req.Engine)) {
		writeError(w, http.StatusBadRequest, "unsupported engine (postgres|mysql|mariadb|mongo|redis)")
		return
	}
	if req.Username == "" {
		req.Username = "kapibara"
	}
	if req.Password == "" {
		req.Password = randomSecret()
	}
	if req.DBName == "" {
		req.DBName = "app"
	}
	if req.VolumeSize == "" {
		req.VolumeSize = "1Gi"
	}
	db := &store.Database{
		ProjectID:      p.ID,
		Name:           req.Name,
		OrcinusProject: unitProject(p, req.Name),
		Engine:         req.Engine,
		Version:    req.Version,
		Username:   req.Username,
		Password:   req.Password,
		DBName:     req.DBName,
		VolumeSize: req.VolumeSize,
	}
	if err := s.Store.CreateDatabase(db); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, dbView(db))
}

func (s *Server) handleListDatabases(w http.ResponseWriter, r *http.Request) {
	p := s.loadProjectWithAccess(w, r)
	if p == nil {
		return
	}
	dbs, err := s.Store.DatabasesForProject(p.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	views := make([]map[string]any, 0, len(dbs))
	for i := range dbs {
		views = append(views, dbView(&dbs[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"databases": views})
}

func (s *Server) handleGetDatabase(w http.ResponseWriter, r *http.Request) {
	db, _ := s.loadDatabaseWithAccess(w, r)
	if db == nil {
		return
	}
	writeJSON(w, http.StatusOK, dbView(db))
}

func (s *Server) handleDeployDatabase(w http.ResponseWriter, r *http.Request) {
	db, project := s.loadDatabaseWithAccess(w, r)
	if db == nil {
		return
	}
	dep, err := s.Deployer.DeployDatabase(r.Context(), db, project)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deployment":       dep,
		"connectionString": deployer.ConnectionString(db),
		"host":             db.Host,
		"port":             db.Port,
	})
}

func (s *Server) handleDeleteDatabase(w http.ResponseWriter, r *http.Request) {
	db, _ := s.loadDatabaseWithAccess(w, r)
	if db == nil {
		return
	}
	if err := s.Store.DB.Delete(&store.Database{}, "id = ?", db.ID).Error; err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) loadDatabaseWithAccess(w http.ResponseWriter, r *http.Request) (*store.Database, *store.Project) {
	db, err := s.Store.DatabaseByID(chi.URLParam(r, "dbID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "database not found")
		return nil, nil
	}
	p, err := s.Store.ProjectByID(db.ProjectID)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return nil, nil
	}
	if s.requireOrgAccess(w, r, p.OrganizationID) == nil {
		return nil, nil
	}
	return db, p
}

// dbView includes the connection string (with password) so the UI can show it.
func dbView(db *store.Database) map[string]any {
	return map[string]any{
		"id":               db.ID,
		"name":             db.Name,
		"engine":           db.Engine,
		"version":          db.Version,
		"username":         db.Username,
		"dbName":           db.DBName,
		"volumeSize":       db.VolumeSize,
		"host":             db.Host,
		"port":             db.Port,
		"connectionString": deployer.ConnectionString(db),
	}
}

func randomSecret() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
