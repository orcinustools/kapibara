package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/orcinustools/kapibara/pkg/orcinus"
)

// handleLogs streams a pod's logs. Query params: service (pick first pod of
// that service), pod (explicit pod name), follow=true, tail=N.
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	p := s.loadProjectWithAccess(w, r)
	if p == nil {
		return
	}
	if s.Kube == nil {
		writeError(w, http.StatusServiceUnavailable, "cluster access unavailable (no kubeconfig)")
		return
	}

	pod := r.URL.Query().Get("pod")
	service := r.URL.Query().Get("service")
	follow := r.URL.Query().Get("follow") == "true"
	tail, _ := strconv.ParseInt(r.URL.Query().Get("tail"), 10, 64)
	if tail == 0 {
		tail = 200
	}

	// Resolve a pod name if not given explicitly, searching every unit's project.
	if pod == "" {
		for _, u := range s.projectUnits(p.ID) {
			pods, err := s.Orcinus.Pods(r.Context(), u.OrcinusProject)
			if err != nil {
				continue
			}
			if pod = pickPod(pods, service); pod != "" {
				break
			}
		}
		if pod == "" {
			writeError(w, http.StatusNotFound, "no pods found for project/service")
			return
		}
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	flusher, _ := w.(http.Flusher)

	// Wrap the writer so we flush after each chunk (for follow mode).
	fw := &flushWriter{w: w, f: flusher}
	if err := s.Kube.StreamLogs(r.Context(), s.Cfg.Namespace, pod, follow, tail, fw); err != nil {
		// If streaming already started we can't change the status code.
		_, _ = w.Write([]byte("\n[log stream ended: " + err.Error() + "]\n"))
	}
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	p := s.loadProjectWithAccess(w, r)
	if p == nil {
		return
	}
	if s.Kube == nil {
		writeError(w, http.StatusServiceUnavailable, "cluster access unavailable (no kubeconfig)")
		return
	}
	all, err := s.Kube.PodMetrics(r.Context(), s.Cfg.Namespace, "")
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "metrics unavailable (install metrics-server): "+err.Error())
		return
	}
	// Aggregate pod metrics across every unit's isolated orcinus project.
	matched, sample := s.projectPodMetrics(r.Context(), p, all)
	sample.T = time.Now()
	// Fold this on-demand read into the rolling history so the series still
	// advances between background sampler ticks (and even without a sampler).
	s.metrics.record(p.ID, sample)

	metrics := make([]any, 0, len(matched))
	for _, m := range matched {
		metrics = append(metrics, m)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"metrics": metrics,
		"current": sample,
		"history": s.metrics.get(p.ID),
	})
}

func pickPod(pods []orcinus.Pod, service string) string {
	for _, p := range pods {
		if service == "" || p.Service == service {
			return p.Name
		}
	}
	return ""
}

type flushWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

func (fw *flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if fw.f != nil {
		fw.f.Flush()
	}
	return n, err
}
