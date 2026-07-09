package api

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// maxUploadBytes caps an uploaded build context (compressed). Larger contexts
// almost always mean a missing .dockerignore (node_modules, .git, build output).
const maxUploadBytes = 500 << 20 // 500 MiB

// handleUploadAppSource stores an uploaded build context (gzip-compressed tar)
// for an application so the next deploy builds from it instead of cloning Git.
// The client (`kapibara up`) streams the archive as the request body; the server
// builds it in-cluster (railpack/Dockerfile) — no Docker or Git on the client.
func (s *Server) handleUploadAppSource(w http.ResponseWriter, r *http.Request) {
	app, _ := s.loadAppWithAccess(w, r)
	if app == nil {
		return
	}
	dir := filepath.Join(s.Cfg.DataDir, "uploads")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dest := filepath.Join(dir, app.ID+".tgz")

	body := http.MaxBytesReader(w, r.Body, maxUploadBytes)
	tmp, err := os.CreateTemp(dir, app.ID+"-*.tmp")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tmpName := tmp.Name()
	n, err := io.Copy(tmp, body)
	tmp.Close()
	if err != nil {
		os.Remove(tmpName)
		writeError(w, http.StatusBadRequest, "upload failed (too large or truncated): "+err.Error())
		return
	}
	if n == 0 {
		os.Remove(tmpName)
		writeError(w, http.StatusBadRequest, "empty upload")
		return
	}
	// Atomically replace the app's stored context.
	if err := os.Rename(tmpName, dest); err != nil {
		os.Remove(tmpName)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	app.SourceArchive = dest
	if err := s.Store.UpdateApplication(app); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "uploaded", "bytes": n})
}
