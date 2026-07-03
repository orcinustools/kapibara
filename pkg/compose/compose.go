// Package compose generates docker-compose sources (with x-orcinus-* hints)
// from kapibara's higher-level application/database models. orcinus turns these
// into Kubernetes objects, so kapibara never writes manifests directly.
package compose

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Expose selects how a service is reachable.
type Expose string

const (
	ExposeNone    Expose = ""
	ExposeIngress Expose = "ingress"
	ExposeNodePort Expose = "nodeport"
	ExposeCluster  Expose = "clusterip"
)

// Controller selects the workload controller (x-orcinus-controller).
type Controller string

const (
	ControllerDeployment  Controller = "deployment"
	ControllerStatefulSet Controller = "statefulset"
	ControllerDaemonSet   Controller = "daemonset"
)

// Service is a single compose service plus orcinus hints.
type Service struct {
	Name  string
	Image string
	// Ports are "container" or "host:container" entries.
	Ports []string
	// Env are plain (non-secret) environment variables.
	Env map[string]string
	// Replicas, when > 0, sets the desired replica count.
	Replicas int

	// x-orcinus-* hints.
	Expose     Expose
	Host       string   // x-orcinus-host (ingress host)
	TLS        bool     // enable cert-manager TLS
	TLSIssuer  string   // x-orcinus-tls ClusterIssuer name (default "letsencrypt")
	Controller Controller
	VolumeSize string   // x-orcinus-volume-size (e.g. 5Gi)
	Secrets    []string // x-orcinus-secret env var names
	// Volumes are "name:/path" mount entries (named volumes → PVC).
	Volumes []string

	// Autoscale (HPA). Min>0 enables x-orcinus-autoscale-*.
	AutoscaleMin    int
	AutoscaleMax    int
	AutoscaleCPU    int // target CPU utilization %
	AutoscaleMemory int // target memory utilization %
	// Rollout enables progressive delivery (x-orcinus-rollout), e.g. "canary".
	Rollout string
}

// Project is a set of services rendered as one compose file.
type Project struct {
	Services []Service
	// Volumes are named volumes referenced by services.
	Volumes []string
}

// Render returns the docker-compose YAML for the project.
func (p Project) Render() (string, error) {
	svc := map[string]any{}
	for _, s := range p.Services {
		if s.Name == "" || s.Image == "" {
			return "", fmt.Errorf("service needs a name and image")
		}
		m := map[string]any{"image": s.Image}
		if len(s.Ports) > 0 {
			m["ports"] = s.Ports
		}
		if len(s.Env) > 0 {
			m["environment"] = s.Env
		}
		if len(s.Volumes) > 0 {
			m["volumes"] = s.Volumes
		}
		if s.Replicas > 0 {
			m["deploy"] = map[string]any{"replicas": s.Replicas}
		}
		if s.Expose != ExposeNone {
			m["x-orcinus-expose"] = string(s.Expose)
		}
		if s.Host != "" {
			m["x-orcinus-host"] = s.Host
		}
		if s.TLS {
			// x-orcinus-tls is the cert-manager ClusterIssuer name.
			issuer := s.TLSIssuer
			if issuer == "" {
				issuer = "letsencrypt"
			}
			m["x-orcinus-tls"] = issuer
		}
		if s.Controller != "" {
			m["x-orcinus-controller"] = string(s.Controller)
		}
		if s.VolumeSize != "" {
			m["x-orcinus-volume-size"] = s.VolumeSize
		}
		if len(s.Secrets) > 0 {
			m["x-orcinus-secret"] = s.Secrets
		}
		if s.AutoscaleMin > 0 {
			m["x-orcinus-autoscale-min"] = s.AutoscaleMin
			if s.AutoscaleMax > 0 {
				m["x-orcinus-autoscale-max"] = s.AutoscaleMax
			}
			if s.AutoscaleCPU > 0 {
				m["x-orcinus-autoscale-cpu"] = s.AutoscaleCPU
			}
			if s.AutoscaleMemory > 0 {
				m["x-orcinus-autoscale-memory"] = s.AutoscaleMemory
			}
		}
		if s.Rollout != "" {
			m["x-orcinus-rollout"] = s.Rollout
		}
		svc[s.Name] = m
	}

	root := map[string]any{"services": svc}
	if len(p.Volumes) > 0 {
		vols := map[string]any{}
		for _, v := range p.Volumes {
			vols[v] = nil
		}
		root["volumes"] = vols
	}

	b, err := yaml.Marshal(root)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
