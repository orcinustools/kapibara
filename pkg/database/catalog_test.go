package database

import (
	"strings"
	"testing"
)

func TestPostgresSpec(t *testing.T) {
	spec := Spec{Engine: Postgres, Name: "db", Username: "u", Password: "p", Database: "app", VolumeSize: "5Gi"}
	if spec.Image() != "postgres:16" {
		t.Errorf("image = %q", spec.Image())
	}
	if spec.Port() != 5432 {
		t.Errorf("port = %d", spec.Port())
	}
	svc, vol, err := spec.Service()
	if err != nil {
		t.Fatal(err)
	}
	if svc.Controller != "statefulset" {
		t.Errorf("controller = %q", svc.Controller)
	}
	if vol != "db-data" {
		t.Errorf("vol = %q", vol)
	}
	if svc.Env["POSTGRES_PASSWORD"] != "p" || len(svc.Secrets) == 0 {
		t.Errorf("env/secret wrong: %+v %+v", svc.Env, svc.Secrets)
	}
	cs := spec.ConnectionString("db")
	if !strings.HasPrefix(cs, "postgres://u:p@db:5432/app") {
		t.Errorf("conn string = %q", cs)
	}
}

func TestEnginesSupported(t *testing.T) {
	for _, e := range []Engine{Postgres, MySQL, MariaDB, Mongo, Redis} {
		if !Supported(e) {
			t.Errorf("%s not supported", e)
		}
		spec := Spec{Engine: e, Name: "x", Username: "u", Password: "p", Database: "d"}
		if _, _, err := spec.Service(); err != nil {
			t.Errorf("%s service: %v", e, err)
		}
	}
	if Supported("cockroach") {
		t.Error("cockroach should not be supported")
	}
}
