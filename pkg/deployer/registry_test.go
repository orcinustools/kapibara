package deployer

import "testing"

func TestRewriteRegistryImage(t *testing.T) {
	const host = "kapibara.mayar.io"
	const scope = "mayar"
	cases := []struct {
		image, public, scope, want string
	}{
		// Short "registry/<project>/<image>" gets host + scope inserted.
		{"registry/worker/api:1", host, scope, "kapibara.mayar.io/registry/mayar/worker/api:1"},
		// Host already present, scope inserted.
		{"kapibara.mayar.io/registry/worker/api:1", host, scope, "kapibara.mayar.io/registry/mayar/worker/api:1"},
		// Already scoped → unchanged (idempotent).
		{"kapibara.mayar.io/registry/mayar/worker/api:1", host, scope, "kapibara.mayar.io/registry/mayar/worker/api:1"},
		// No scope configured → host inserted, no scope segment.
		{"registry/worker/api:1", host, "", "kapibara.mayar.io/registry/worker/api:1"},
		// No public host → unchanged.
		{"registry/worker/api:1", "", scope, "registry/worker/api:1"},
		// External images pass through untouched.
		{"nginx:alpine", host, scope, "nginx:alpine"},
		{"ghcr.io/acme/x:1", host, scope, "ghcr.io/acme/x:1"},
		{"docker.io/library/busybox:1.36", host, scope, "docker.io/library/busybox:1.36"},
	}
	for _, c := range cases {
		if got := RewriteRegistryImage(c.image, c.public, c.scope); got != c.want {
			t.Errorf("RewriteRegistryImage(%q,%q,%q) = %q, want %q", c.image, c.public, c.scope, got, c.want)
		}
	}
}
