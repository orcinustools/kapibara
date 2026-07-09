package deployer

import "testing"

func TestRewriteRegistryImage(t *testing.T) {
	cases := []struct {
		image, public, want string
	}{
		// Friendly "kapibara/<path>" rewritten to the public gateway host.
		{"kapibara/acme/hello:1", "kapibara.mayar.io", "kapibara.mayar.io/acme/hello:1"},
		{"kapibara/orgid/app:tag", "kapibara.mayar.io/", "kapibara.mayar.io/orgid/app:tag"},
		// No public host configured → unchanged.
		{"kapibara/acme/hello:1", "", "kapibara/acme/hello:1"},
		// Non-marker images pass through untouched.
		{"nginx:alpine", "kapibara.mayar.io", "nginx:alpine"},
		{"docker.io/library/busybox:1.36", "kapibara.mayar.io", "docker.io/library/busybox:1.36"},
		{"ghcr.io/kapibara/x:1", "kapibara.mayar.io", "ghcr.io/kapibara/x:1"},
	}
	for _, c := range cases {
		if got := rewriteRegistryImage(c.image, c.public); got != c.want {
			t.Errorf("rewriteRegistryImage(%q,%q) = %q, want %q", c.image, c.public, got, c.want)
		}
	}
}
