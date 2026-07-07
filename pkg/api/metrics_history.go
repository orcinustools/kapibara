package api

import (
	"context"
	"sync"
	"time"

	"github.com/orcinustools/kapibara/pkg/kube"
	"github.com/orcinustools/kapibara/pkg/store"
)

// metricSample is one point of aggregated resource usage for a project at a
// moment in time (summed across all of the project's pods).
type metricSample struct {
	T             time.Time `json:"t"`
	CPUMillicores int64     `json:"cpuMillicores"`
	MemoryBytes   int64     `json:"memoryBytes"`
	Pods          int       `json:"pods"`
}

// metricsHistory keeps a bounded, in-memory time-series of aggregated pod
// metrics per kapibara project. It is populated by a background sampler so
// history accrues even when no UI is polling. History is not persisted — it is
// a rolling window that resets on restart (acceptable for a live dashboard).
type metricsHistory struct {
	mu     sync.Mutex
	series map[string][]metricSample
	max    int
}

func newMetricsHistory(max int) *metricsHistory {
	if max <= 0 {
		max = 120
	}
	return &metricsHistory{series: map[string][]metricSample{}, max: max}
}

// record appends a sample for a project, evicting the oldest past the cap.
func (h *metricsHistory) record(projectID string, s metricSample) {
	h.mu.Lock()
	defer h.mu.Unlock()
	series := append(h.series[projectID], s)
	if len(series) > h.max {
		series = series[len(series)-h.max:]
	}
	h.series[projectID] = series
}

// get returns a copy of a project's samples (oldest first).
func (h *metricsHistory) get(projectID string) []metricSample {
	h.mu.Lock()
	defer h.mu.Unlock()
	src := h.series[projectID]
	out := make([]metricSample, len(src))
	copy(out, src)
	return out
}

// projectPodMetrics resolves the pods belonging to a project's units and
// returns their raw per-pod metrics plus an aggregated sample.
func (s *Server) projectPodMetrics(ctx context.Context, p *store.Project, all []kube.PodMetric) ([]kube.PodMetric, metricSample) {
	podSet := map[string]bool{}
	for _, u := range s.projectUnits(p.ID) {
		if pods, err := s.Orcinus.Pods(ctx, u.OrcinusProject); err == nil {
			for _, pd := range pods {
				podSet[pd.Name] = true
			}
		}
	}
	var matched []kube.PodMetric
	sample := metricSample{}
	for _, m := range all {
		if podSet[m.Pod] {
			matched = append(matched, m)
			sample.CPUMillicores += m.CPUMillicores
			sample.MemoryBytes += m.MemoryBytes
			sample.Pods++
		}
	}
	return matched, sample
}

// StartMetricsSampler runs a background loop that samples aggregated pod
// metrics for every project on an interval and records them into the history
// buffer. It returns when ctx is cancelled or if cluster access is unavailable.
func (s *Server) StartMetricsSampler(ctx context.Context, interval time.Duration) {
	if s.Kube == nil {
		return // no direct cluster access → nothing to sample
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.sampleAllProjects(ctx, now)
		}
	}
}

func (s *Server) sampleAllProjects(ctx context.Context, now time.Time) {
	all, err := s.Kube.PodMetrics(ctx, s.Cfg.Namespace, "")
	if err != nil {
		return
	}
	projects, err := s.Store.AllProjects()
	if err != nil {
		return
	}
	for i := range projects {
		_, sample := s.projectPodMetrics(ctx, &projects[i], all)
		sample.T = now
		s.metrics.record(projects[i].ID, sample)
	}
}
