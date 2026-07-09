// Package config loads kapibara server configuration from the environment.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration for the kapibara control-plane.
type Config struct {
	// Addr is the HTTP listen address for the kapibara API + UI.
	Addr string
	// DatabaseURL selects the control-plane store. Empty or a path ending in
	// .db uses SQLite (pure-Go); a postgres:// URL uses Postgres.
	DatabaseURL string
	// JWTSecret signs session/API tokens. Auto-generated if empty (dev only).
	JWTSecret string

	// OrcinusURL is the base URL of the orcinus HTTP API (cluster engine).
	OrcinusURL string
	// OrcinusToken is the bearer token for the orcinus API.
	OrcinusToken string
	// Kubeconfig is the path used for direct cluster access (logs, metrics).
	Kubeconfig string
	// Namespace is the cluster namespace orcinus deploys projects into.
	Namespace string

	// DataDir holds server-local state (default SQLite file, build cache, …).
	DataDir string

	// RegistryPrefix is prepended to built image names, e.g.
	// "registry.example.com/kapibara". Empty → local image names.
	RegistryPrefix string
	// BuildPush pushes built images to a registry (needs RegistryPrefix).
	BuildPush bool
	// ClusterContainer is the docker container name of the single-node k3s
	// node; when set (and not pushing) images are imported into its containerd.
	ClusterContainer string

	// RegistryUpstream, when set, enables the Docker Registry v2 gateway at
	// /v2/* — reverse-proxying to this in-cluster registry (e.g.
	// http://registry.orcinus-registry.svc:5000). Reads are anonymous; pushes
	// require a Kapibara account.
	RegistryUpstream string
	// RegistryPublic is the public host of the registry gateway (e.g.
	// kapibara.mayar.io). At deploy time an app image written as
	// "kapibara/<path>" is rewritten to "<RegistryPublic>/<path>" so users
	// reference a short name and the cluster pulls it back through the gateway.
	RegistryPublic string

	// InClusterBuild enables server-side, Docker-less Git builds: the
	// control-plane clones the repo, builds it with buildctl against BuildkitAddr
	// (railpack/Dockerfile frontends), and pushes the image to the in-cluster
	// registry — the cluster then pulls it back through the public gateway. No
	// Docker daemon on the control-plane is required.
	InClusterBuild bool
	// BuildkitAddr is the BuildKit daemon buildctl talks to, e.g.
	// tcp://buildkitd.orcinus-build.svc:1234.
	BuildkitAddr string
	// BuildPlatform is the target platform for in-cluster builds (default
	// linux/amd64), so images match the cluster regardless of build host arch.
	BuildPlatform string
	// RailpackFrontend is the railpack BuildKit frontend image (pinned to the
	// railpack version bundled in this image).
	RailpackFrontend string

	// PublicURL is the externally reachable base URL of this kapibara server,
	// used to build OAuth redirect URIs (e.g. https://paas.example.com). Falls
	// back to the request host when empty.
	PublicURL string
	// AppsDomain is the base wildcard domain deployed apps live under, e.g.
	// "apps.example.com" (a wildcard *.apps.example.com → the cluster). Agents,
	// the UI, and the CLI use it to derive "<app>.<AppsDomain>" when the user
	// doesn't specify a host. Stored without any leading "*." or ".".
	AppsDomain string
	// Git provider OAuth app credentials (optional; enables the OAuth connect
	// flow — PAT connect works without them).
	GitHubClientID     string
	GitHubClientSecret string
	GitLabClientID     string
	GitLabClientSecret string
}

// Load reads configuration from environment variables, applying sensible
// defaults for local development.
func Load() Config {
	dataDir := env("KAPIBARA_DATA_DIR", defaultDataDir())
	return Config{
		Addr:         env("KAPIBARA_ADDR", ":9000"),
		DatabaseURL:  env("KAPIBARA_DATABASE_URL", dataDir+"/kapibara.db"),
		JWTSecret:    os.Getenv("KAPIBARA_JWT_SECRET"),
		OrcinusURL:   env("KAPIBARA_ORCINUS_URL", "http://localhost:8899"),
		OrcinusToken: os.Getenv("KAPIBARA_ORCINUS_TOKEN"),
		Kubeconfig:   env("KAPIBARA_KUBECONFIG", defaultKubeconfig()),
		Namespace:    env("KAPIBARA_NAMESPACE", "default"),
		DataDir:      dataDir,

		RegistryPrefix:   os.Getenv("KAPIBARA_REGISTRY"),
		BuildPush:        os.Getenv("KAPIBARA_BUILD_PUSH") == "1" || os.Getenv("KAPIBARA_BUILD_PUSH") == "true",
		ClusterContainer: env("KAPIBARA_CLUSTER_CONTAINER", "orcinus"),
		RegistryUpstream: os.Getenv("KAPIBARA_REGISTRY_UPSTREAM"),
		RegistryPublic:   os.Getenv("KAPIBARA_REGISTRY_PUBLIC"),

		InClusterBuild:   os.Getenv("KAPIBARA_INCLUSTER_BUILD") == "1" || os.Getenv("KAPIBARA_INCLUSTER_BUILD") == "true",
		BuildkitAddr:     os.Getenv("KAPIBARA_BUILDKIT_ADDR"),
		BuildPlatform:    env("KAPIBARA_BUILD_PLATFORM", "linux/amd64"),
		RailpackFrontend: env("KAPIBARA_RAILPACK_FRONTEND", "ghcr.io/railwayapp/railpack-frontend"),

		PublicURL:          strings.TrimRight(os.Getenv("KAPIBARA_PUBLIC_URL"), "/"),
		AppsDomain:         strings.Trim(strings.TrimPrefix(strings.TrimSpace(os.Getenv("KAPIBARA_APPS_DOMAIN")), "*."), "."),
		GitHubClientID:     os.Getenv("KAPIBARA_GITHUB_CLIENT_ID"),
		GitHubClientSecret: os.Getenv("KAPIBARA_GITHUB_CLIENT_SECRET"),
		GitLabClientID:     os.Getenv("KAPIBARA_GITLAB_CLIENT_ID"),
		GitLabClientSecret: os.Getenv("KAPIBARA_GITLAB_CLIENT_SECRET"),
	}
}

func defaultKubeconfig() string {
	if kc := os.Getenv("KUBECONFIG"); kc != "" {
		return kc
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home + "/.orcinus/kubeconfig"
	}
	return ""
}

func defaultDataDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home + "/.kapibara"
	}
	return "./.kapibara"
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envInt reads an integer environment variable with a default.
func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
