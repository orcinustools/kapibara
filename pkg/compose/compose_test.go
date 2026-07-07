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

func TestRenderResourcesCommandMountsHealth(t *testing.T) {
	out, err := Project{
		Services: []Service{{
			Name:           "web",
			Image:          "nginx:alpine",
			Replicas:       2,
			Command:        []string{"nginx", "-g", "daemon off;"},
			Path:           "/api",
			Expose:         ExposeIngress,
			Host:           "x.example.com",
			CPULimit:       "0.5",
			MemLimit:       "512M",
			CPUReservation: "0.25",
			MemReservation: "256M",
			VolumeSize:     "2Gi",
			Volumes:        []string{"data:/var/lib/data"},
			Health:         &HealthCheck{Test: []string{"CMD", "curl", "-f", "http://localhost/"}, Interval: "10s", Retries: 3},
		}},
		Volumes: []string{"data"},
	}.Render()
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{
		"deploy:",
		"replicas: 2",
		"resources:",
		"limits:",
		"cpus: \"0.5\"",
		"memory: 512M",
		"reservations:",
		"command:",
		"daemon off;",
		"x-orcinus-path: /api",
		"x-orcinus-volume-size: 2Gi",
		"data:/var/lib/data",
		"healthcheck:",
		"retries: 3",
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
