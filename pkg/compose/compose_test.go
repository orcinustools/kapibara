package compose

import (
	"strings"
	"testing"
)

func TestRenderApplicationWithIngressTLS(t *testing.T) {
	out, err := Project{Services: []Service{{
		Name:     "web",
		Image:    "kapibara/shop-web:abc123",
		Ports:    []string{"3000"},
		Replicas: 2,
		Env:      map[string]string{"NODE_ENV": "production"},
		Expose:   ExposeIngress,
		Host:     "shop.example.com",
		TLS:      true,
	}}}.Render()
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{
		"services:",
		"web:",
		"image: kapibara/shop-web:abc123",
		"x-orcinus-expose: ingress",
		"x-orcinus-host: shop.example.com",
		"x-orcinus-tls: letsencrypt",
		"NODE_ENV: production",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered compose missing %q:\n%s", want, out)
		}
	}
}

func TestRenderDatabaseStatefulSet(t *testing.T) {
	out, err := Project{
		Services: []Service{{
			Name:       "db",
			Image:      "postgres:16",
			Controller: ControllerStatefulSet,
			VolumeSize: "5Gi",
			Secrets:    []string{"POSTGRES_PASSWORD"},
			Env:        map[string]string{"POSTGRES_DB": "app"},
		}},
	}.Render()
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{
		"x-orcinus-controller: statefulset",
		"x-orcinus-volume-size: 5Gi",
		"x-orcinus-secret:",
		"POSTGRES_PASSWORD",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered compose missing %q:\n%s", want, out)
		}
	}
}

func TestRenderRequiresImage(t *testing.T) {
	_, err := Project{Services: []Service{{Name: "x"}}}.Render()
	if err == nil {
		t.Fatal("expected error for service without image")
	}
}
