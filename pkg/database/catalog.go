// Package database describes one-click database engines and renders them as
// compose services (statefulset + PVC + secret) for the orcinus engine.
package database

import (
	"fmt"

	"github.com/orcinustools/kapibara/pkg/compose"
)

// Engine is a supported database type.
type Engine string

const (
	Postgres Engine = "postgres"
	MySQL    Engine = "mysql"
	MariaDB  Engine = "mariadb"
	Mongo    Engine = "mongo"
	Redis    Engine = "redis"
)

// Spec fully describes a database instance to provision.
type Spec struct {
	Engine     Engine
	Name       string // service/db name (DNS-safe)
	Version    string // image tag; "" → engine default
	Username   string
	Password   string
	Database   string // initial database name
	VolumeSize string // e.g. "5Gi"
}

type meta struct {
	defaultVersion string
	port           int
	dataPath       string
}

var registry = map[Engine]meta{
	Postgres: {"16", 5432, "/var/lib/postgresql/data"},
	MySQL:    {"8", 3306, "/var/lib/mysql"},
	MariaDB:  {"11", 3306, "/var/lib/mysql"},
	Mongo:    {"7", 27017, "/data/db"},
	Redis:    {"7", 6379, "/data"},
}

// Supported reports whether the engine is known.
func Supported(e Engine) bool {
	_, ok := registry[e]
	return ok
}

// Port returns the engine's default service port.
func (s Spec) Port() int { return registry[s.Engine].port }

// Image returns the container image for the spec.
func (s Spec) Image() string {
	v := s.Version
	if v == "" {
		v = registry[s.Engine].defaultVersion
	}
	name := string(s.Engine)
	if s.Engine == Postgres {
		name = "postgres"
	}
	return fmt.Sprintf("%s:%s", name, v)
}

// Service renders the compose service (with x-orcinus hints) for the database.
func (s Spec) Service() (compose.Service, string, error) {
	m, ok := registry[s.Engine]
	if !ok {
		return compose.Service{}, "", fmt.Errorf("unsupported engine %q", s.Engine)
	}
	volName := s.Name + "-data"
	svc := compose.Service{
		Name:  s.Name,
		Image: s.Image(),
		// Publish the engine port so orcinus creates a ClusterIP Service — this
		// is what gives the database its in-cluster DNS name (<name>:<port>)
		// that applications connect to. Without ports there is no Service.
		Ports:      []string{fmt.Sprintf("%d", m.port)},
		Controller: compose.ControllerStatefulSet,
		VolumeSize: s.VolumeSize,
		Expose:     compose.ExposeCluster, // ClusterIP (in-cluster only, no ingress)
		Volumes:    []string{volName + ":" + m.dataPath},
		Env:        map[string]string{},
	}

	switch s.Engine {
	case Postgres:
		svc.Env["POSTGRES_USER"] = s.Username
		svc.Env["POSTGRES_PASSWORD"] = s.Password
		svc.Env["POSTGRES_DB"] = s.Database
		svc.Secrets = []string{"POSTGRES_PASSWORD"}
	case MySQL, MariaDB:
		svc.Env["MYSQL_ROOT_PASSWORD"] = s.Password
		svc.Env["MYSQL_DATABASE"] = s.Database
		svc.Env["MYSQL_USER"] = s.Username
		svc.Env["MYSQL_PASSWORD"] = s.Password
		svc.Secrets = []string{"MYSQL_ROOT_PASSWORD", "MYSQL_PASSWORD"}
	case Mongo:
		svc.Env["MONGO_INITDB_ROOT_USERNAME"] = s.Username
		svc.Env["MONGO_INITDB_ROOT_PASSWORD"] = s.Password
		svc.Env["MONGO_INITDB_DATABASE"] = s.Database
		svc.Secrets = []string{"MONGO_INITDB_ROOT_PASSWORD"}
	case Redis:
		// Redis has no init env; auth handled by users if needed later.
	}
	return svc, volName, nil
}

// ConnectionString returns an in-cluster connection URI for the database. host
// is the service DNS name orcinus assigns (typically the service name).
func (s Spec) ConnectionString(host string) string {
	p := s.Port()
	switch s.Engine {
	case Postgres:
		return fmt.Sprintf("postgres://%s:%s@%s:%d/%s", s.Username, s.Password, host, p, s.Database)
	case MySQL, MariaDB:
		return fmt.Sprintf("mysql://%s:%s@%s:%d/%s", s.Username, s.Password, host, p, s.Database)
	case Mongo:
		return fmt.Sprintf("mongodb://%s:%s@%s:%d/%s", s.Username, s.Password, host, p, s.Database)
	case Redis:
		return fmt.Sprintf("redis://%s:%d", host, p)
	}
	return ""
}
