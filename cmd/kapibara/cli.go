package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// cliConfig is the client-side state persisted at ~/.kapibara/cli.json so the
// deploy commands don't re-authenticate on every call.
type cliConfig struct {
	Server string `json:"server"`
	Token  string `json:"token"`
	OrgID  string `json:"orgId"`
	Email  string `json:"email"`
	// RegistryToken is a cached kap_ API token used as the Docker password when
	// pushing to the registry gateway (minted on first `image build`).
	RegistryToken string `json:"registryToken,omitempty"`
}

func cliConfigPath() string {
	if p := os.Getenv("KAPIBARA_CLI_CONFIG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".kapibara-cli.json"
	}
	return filepath.Join(home, ".kapibara", "cli.json")
}

func loadCLIConfig() cliConfig {
	// Precedence: saved config (from `login`) wins over environment, which is
	// only a fallback default for a CLI that hasn't logged in yet. This avoids a
	// stray KAPIBARA_URL in the shell silently redirecting an authenticated CLI
	// to a different server than the one it holds a token for.
	c := cliConfig{
		Server: os.Getenv("KAPIBARA_URL"),
		Token:  os.Getenv("KAPIBARA_TOKEN"),
	}
	if b, err := os.ReadFile(cliConfigPath()); err == nil {
		var f cliConfig
		if json.Unmarshal(b, &f) == nil {
			if f.Server != "" {
				c.Server = f.Server
			}
			if f.Token != "" {
				c.Token = f.Token
			}
			c.OrgID = f.OrgID
			c.Email = f.Email
			c.RegistryToken = f.RegistryToken
		}
	}
	if c.Server == "" {
		c.Server = "http://localhost:9000"
	}
	return c
}

func (c cliConfig) save() error {
	p := cliConfigPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(p, b, 0o600)
}

// apiClient is a tiny REST client for the kapibara control-plane API.
type apiClient struct {
	server string
	token  string
	http   *http.Client
}

func newAPIClient(cfg cliConfig) *apiClient {
	return &apiClient{
		server: strings.TrimRight(cfg.Server, "/"),
		token:  cfg.Token,
		http:   &http.Client{Timeout: 180 * time.Second},
	}
}

// do performs a JSON request. out may be nil. Non-2xx responses return an error
// carrying the server's message.
func (a *apiClient) do(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.server+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return fmt.Errorf("request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(raw))
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Error != "" {
			msg = e.Error
		}
		return fmt.Errorf("%s %s → HTTP %d: %s", method, path, resp.StatusCode, msg)
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// --- command wiring ---

func cliCommands() []*cobra.Command {
	return []*cobra.Command{
		loginCmd(),
		infoCmd(),
		projectsCmd(),
		deployCmd(),
		imageCmd(),
		appCmd(),
		databaseCmd(),
		deploymentCmd(),
		secretCmd(),
	}
}

// secretCmd manages cluster secrets (import/list/remove). Secret values are
// write-only server-side: the list endpoint returns only names + key counts.
func secretCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "secret", Short: "Manage cluster secrets"}

	list := &cobra.Command{
		Use:   "list",
		Short: "List cluster secrets (names + key counts; values are never returned)",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newAPIClient(loadCLIConfig())
			var out struct {
				Secrets []struct {
					Name string `json:"name"`
					Keys int    `json:"keys"`
				} `json:"secrets"`
			}
			if err := client.do(cmd.Context(), http.MethodGet, "/api/v1/secrets", nil, &out); err != nil {
				return err
			}
			if len(out.Secrets) == 0 {
				fmt.Println("no secrets")
				return nil
			}
			for _, s := range out.Secrets {
				fmt.Printf("%-32s  %d key(s)\n", s.Name, s.Keys)
			}
			return nil
		},
	}

	var data []string
	var envFile string
	put := &cobra.Command{
		Use:   "put NAME",
		Short: "Create or replace a secret from --data KEY=VALUE pairs and/or --env-file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			kv := map[string]string{}
			if envFile != "" {
				b, err := os.ReadFile(envFile)
				if err != nil {
					return fmt.Errorf("read env file: %w", err)
				}
				for _, line := range strings.Split(string(b), "\n") {
					line = strings.TrimSpace(line)
					if line == "" || strings.HasPrefix(line, "#") {
						continue
					}
					k, v, ok := strings.Cut(line, "=")
					if !ok {
						return fmt.Errorf("invalid env-file line %q (want KEY=VALUE)", line)
					}
					kv[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
				}
			}
			for _, d := range data {
				k, v, ok := strings.Cut(d, "=")
				if !ok {
					return fmt.Errorf("invalid --data %q (want KEY=VALUE)", d)
				}
				kv[k] = v
			}
			if len(kv) == 0 {
				return fmt.Errorf("provide at least one --data KEY=VALUE or --env-file")
			}
			client := newAPIClient(loadCLIConfig())
			body := map[string]any{"name": args[0], "data": kv}
			if err := client.do(cmd.Context(), http.MethodPost, "/api/v1/secrets", body, nil); err != nil {
				return err
			}
			fmt.Printf("✓ secret %q saved (%d key(s))\n", args[0], len(kv))
			return nil
		},
	}
	put.Flags().StringArrayVar(&data, "data", nil, "KEY=VALUE secret entry (repeatable)")
	put.Flags().StringVar(&envFile, "env-file", "", "read KEY=VALUE lines from a .env file")

	rm := &cobra.Command{
		Use:   "rm NAME",
		Short: "Delete a cluster secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newAPIClient(loadCLIConfig())
			if err := client.do(cmd.Context(), http.MethodDelete, "/api/v1/secrets/"+args[0], nil, nil); err != nil {
				return err
			}
			fmt.Printf("✓ secret %q deleted\n", args[0])
			return nil
		},
	}

	cmd.AddCommand(list, put, rm)
	return cmd
}

func loginCmd() *cobra.Command {
	var server, email, password, totp string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate against a kapibara server and cache the token",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadCLIConfig()
			if server != "" {
				cfg.Server = server
			}
			if email == "" {
				return fmt.Errorf("--email is required")
			}
			if password == "" {
				password = os.Getenv("KAPIBARA_PASSWORD")
			}
			if password == "" {
				return fmt.Errorf("--password (or KAPIBARA_PASSWORD) is required")
			}
			client := newAPIClient(cfg)
			var resp struct {
				Token string `json:"token"`
				User  struct {
					ID    string `json:"id"`
					Email string `json:"email"`
				} `json:"user"`
			}
			body := map[string]string{"email": email, "password": password}
			if totp != "" {
				body["totpCode"] = totp
			}
			if err := client.do(cmd.Context(), http.MethodPost, "/api/v1/auth/login", body, &resp); err != nil {
				return err
			}
			cfg.Token = resp.Token
			cfg.Email = resp.User.Email
			// Cache the first organization so `deploy` can resolve projects by name.
			client.token = resp.Token
			var orgs struct {
				Organizations []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"organizations"`
			}
			if err := client.do(cmd.Context(), http.MethodGet, "/api/v1/orgs", nil, &orgs); err == nil && len(orgs.Organizations) > 0 {
				cfg.OrgID = orgs.Organizations[0].ID
			}
			if err := cfg.save(); err != nil {
				return err
			}
			fmt.Printf("logged in as %s → %s (config: %s)\n", cfg.Email, cfg.Server, cliConfigPath())
			return nil
		},
	}
	cmd.Flags().StringVar(&server, "server", "", "kapibara server URL (default http://localhost:9000)")
	cmd.Flags().StringVar(&email, "email", "", "account email")
	cmd.Flags().StringVar(&password, "password", "", "account password (or KAPIBARA_PASSWORD)")
	cmd.Flags().StringVar(&totp, "totp", "", "TOTP code if 2FA is enabled")
	return cmd
}

func projectsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "List projects in the active organization",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadCLIConfig()
			client := newAPIClient(cfg)
			orgID, err := resolveOrg(cmd.Context(), client, cfg)
			if err != nil {
				return err
			}
			var out struct {
				Projects []struct {
					ID             string `json:"id"`
					Name           string `json:"name"`
					OrcinusProject string `json:"orcinusProject"`
				} `json:"projects"`
			}
			if err := client.do(cmd.Context(), http.MethodGet, "/api/v1/orgs/"+orgID+"/projects", nil, &out); err != nil {
				return err
			}
			if len(out.Projects) == 0 {
				fmt.Println("no projects yet")
				return nil
			}
			for _, p := range out.Projects {
				fmt.Printf("%-38s  %-24s  (%s)\n", p.ID, p.Name, p.OrcinusProject)
			}
			return nil
		},
	}
	create := &cobra.Command{
		Use:   "create NAME",
		Short: "Create a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadCLIConfig()
			client := newAPIClient(cfg)
			orgID, err := resolveOrg(cmd.Context(), client, cfg)
			if err != nil {
				return err
			}
			p, err := createProject(cmd.Context(), client, orgID, args[0])
			if err != nil {
				return err
			}
			fmt.Printf("created project %s (%s)\n", p.Name, p.ID)
			return nil
		},
	}
	cmd.AddCommand(create)
	return cmd
}

func deployCmd() *cobra.Command {
	var project, file string
	var wait, noPrune, follow bool
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy a docker-compose file to a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" {
				return fmt.Errorf("--project (name or id) is required")
			}
			if file == "" {
				return fmt.Errorf("-f/--file (compose file) is required")
			}
			src, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("read compose file: %w", err)
			}
			cfg := loadCLIConfig()
			client := newAPIClient(cfg)
			p, err := resolveProject(cmd.Context(), client, cfg, project, true)
			if err != nil {
				return err
			}
			prune := !noPrune
			body := map[string]any{"source": string(src), "wait": wait, "prune": &prune}
			// The deploy runs asynchronously server-side; it returns a deployment
			// we then follow (streaming its log) unless --follow=false.
			var out struct {
				Deployment struct {
					ID string `json:"id"`
				} `json:"deployment"`
			}
			fmt.Printf("deploying %s → project %s (%s)…\n", file, p.Name, p.OrcinusProject)
			if err := client.do(cmd.Context(), http.MethodPost, "/api/v1/projects/"+p.ID+"/deploy", body, &out); err != nil {
				return err
			}
			if out.Deployment.ID == "" {
				return fmt.Errorf("server did not return a deployment id")
			}
			if !follow {
				fmt.Printf("deployment %s started\n", out.Deployment.ID)
				fmt.Printf("track it with: kapibara deployment status %s\n", out.Deployment.ID)
				return nil
			}
			return followDeployment(cmd.Context(), client, out.Deployment.ID)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project name or id")
	cmd.Flags().StringVarP(&file, "file", "f", "", "path to docker-compose file")
	cmd.Flags().BoolVar(&wait, "wait", true, "wait for pods to become ready (server-side)")
	cmd.Flags().BoolVar(&follow, "follow", true, "stream the deployment log until it finishes")
	cmd.Flags().BoolVar(&noPrune, "no-prune", false, "do not prune resources removed from the compose")
	return cmd
}

func appCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "app", Short: "Manage git/image applications"}
	var project, name, buildType, repo, branch, contextDir, dockerfile, image, domain string
	var cpuLimit, memoryLimit, volumeSize string
	var mounts, envPairs, secretKeys, command []string
	var port int
	var tls, follow bool
	deploy := &cobra.Command{
		Use:   "deploy",
		Short: "Create (if needed) and deploy an application, streaming build logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" || name == "" {
				return fmt.Errorf("--project and --name are required")
			}
			cfg := loadCLIConfig()
			client := newAPIClient(cfg)
			p, err := resolveProject(cmd.Context(), client, cfg, project, true)
			if err != nil {
				return err
			}
			env := map[string]string{}
			for _, e := range envPairs {
				k, v, ok := strings.Cut(e, "=")
				if !ok {
					return fmt.Errorf("invalid --env %q (want KEY=VALUE)", e)
				}
				env[k] = v
			}
			app, err := ensureApp(cmd.Context(), client, p.ID, appSpec{
				Name: name, BuildType: buildType, RepoURL: repo, Branch: branch,
				ContextDir: contextDir, DockerfilePath: dockerfile, Image: image, Port: port, Domain: domain, TLS: tls,
				CPULimit: cpuLimit, MemoryLimit: memoryLimit, VolumeSize: volumeSize, Mounts: mounts,
				Env: env, SecretKeys: secretKeys, Command: command,
			})
			if err != nil {
				return err
			}
			fmt.Printf("deploying app %s (%s)…\n", app.Name, app.ID)
			var dep struct {
				ID string `json:"id"`
			}
			if err := client.do(cmd.Context(), http.MethodPost, "/api/v1/apps/"+app.ID+"/deploy", nil, &dep); err != nil {
				return err
			}
			fmt.Printf("deployment %s started\n", dep.ID)
			if follow {
				return followDeployment(cmd.Context(), client, dep.ID)
			}
			fmt.Printf("track it with: kapibara deployment status %s\n", dep.ID)
			return nil
		},
	}
	deploy.Flags().StringVar(&project, "project", "", "project name or id")
	deploy.Flags().StringVar(&name, "name", "", "application name")
	deploy.Flags().StringVar(&buildType, "build", "image", "build type: dockerfile | nixpacks | railpack | image")
	deploy.Flags().StringVar(&repo, "repo", "", "git repo URL (dockerfile/nixpacks builds)")
	deploy.Flags().StringVar(&branch, "branch", "", "git branch")
	deploy.Flags().StringVar(&contextDir, "context-dir", "", "build context subdirectory within the repo (monorepos)")
	deploy.Flags().StringVar(&dockerfile, "dockerfile", "", "path to Dockerfile (relative to the context dir)")
	deploy.Flags().StringVar(&image, "image", "", "prebuilt image reference (build=image)")
	deploy.Flags().IntVar(&port, "port", 0, "container port to expose")
	deploy.Flags().StringVar(&domain, "domain", "", "ingress host/domain")
	deploy.Flags().BoolVar(&tls, "tls", false, "enable TLS (cert-manager) for the domain")
	deploy.Flags().StringVar(&cpuLimit, "cpu-limit", "", "CPU limit in cores, e.g. 0.5")
	deploy.Flags().StringVar(&memoryLimit, "memory-limit", "", "memory limit, e.g. 512M")
	deploy.Flags().StringArrayVar(&mounts, "mount", nil, "persistent volume mount name:path (repeatable)")
	deploy.Flags().StringVar(&volumeSize, "volume-size", "", "PVC size for mounts, e.g. 1Gi")
	deploy.Flags().StringArrayVar(&envPairs, "env", nil, "environment variable KEY=VALUE (repeatable)")
	deploy.Flags().StringArrayVar(&secretKeys, "secret", nil, "mark an --env key as a cluster Secret (repeatable)")
	deploy.Flags().StringArrayVar(&command, "command", nil, "override the container command (repeatable, in order)")
	deploy.Flags().BoolVar(&follow, "follow", true, "stream deployment status/logs until it finishes")
	cmd.AddCommand(deploy)
	return cmd
}

func deploymentCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "deployment", Short: "Inspect deployments"}
	status := &cobra.Command{
		Use:   "status DEPLOYMENT_ID",
		Short: "Show a deployment's status and stream its log until it finishes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadCLIConfig()
			client := newAPIClient(cfg)
			return followDeployment(cmd.Context(), client, args[0])
		},
	}
	var project string
	list := &cobra.Command{
		Use:   "list",
		Short: "List a project's deployment history",
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" {
				return fmt.Errorf("--project (name or id) is required")
			}
			cfg := loadCLIConfig()
			client := newAPIClient(cfg)
			p, err := resolveProject(cmd.Context(), client, cfg, project, false)
			if err != nil {
				return err
			}
			var out struct {
				Deployments []struct {
					ID       string `json:"id"`
					Kind     string `json:"kind"`
					Status   string `json:"status"`
					ImageRef string `json:"imageRef"`
					Applied  int    `json:"applied"`
				} `json:"deployments"`
			}
			if err := client.do(cmd.Context(), http.MethodGet, "/api/v1/projects/"+p.ID+"/deployments", nil, &out); err != nil {
				return err
			}
			for _, d := range out.Deployments {
				fmt.Printf("%-38s  %-11s  %-8s  applied=%d  %s\n", d.ID, d.Kind, d.Status, d.Applied, d.ImageRef)
			}
			return nil
		},
	}
	list.Flags().StringVar(&project, "project", "", "project name or id")
	redeploy := &cobra.Command{
		Use:   "redeploy DEPLOYMENT_ID",
		Short: "Roll back by re-applying a past deployment's snapshot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadCLIConfig()
			client := newAPIClient(cfg)
			var out struct {
				Applied        int    `json:"applied"`
				RolledBackFrom string `json:"rolledBackFrom"`
				Deployment     struct {
					ID string `json:"id"`
				} `json:"deployment"`
			}
			if err := client.do(cmd.Context(), http.MethodPost, "/api/v1/deployments/"+args[0]+"/redeploy", nil, &out); err != nil {
				return err
			}
			fmt.Printf("✓ redeployed from %s → new deployment %s (%d objects applied)\n",
				out.RolledBackFrom, out.Deployment.ID, out.Applied)
			return nil
		},
	}
	cmd.AddCommand(status, list, redeploy)
	return cmd
}

// --- shared helpers ---

type project struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	OrcinusProject string `json:"orcinusProject"`
}

type appSpec struct {
	Name, BuildType, RepoURL, Branch, ContextDir, DockerfilePath, Image, Domain string
	Port                                                                        int
	TLS                                                             bool
	CPULimit, MemoryLimit, VolumeSize                               string
	Mounts                                                          []string // "name:path"
	Env                                                             map[string]string
	SecretKeys                                                      []string
	Command                                                         []string
}

type appInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func resolveOrg(ctx context.Context, client *apiClient, cfg cliConfig) (string, error) {
	if cfg.OrgID != "" {
		return cfg.OrgID, nil
	}
	var orgs struct {
		Organizations []struct {
			ID string `json:"id"`
		} `json:"organizations"`
	}
	if err := client.do(ctx, http.MethodGet, "/api/v1/orgs", nil, &orgs); err != nil {
		return "", err
	}
	if len(orgs.Organizations) == 0 {
		return "", fmt.Errorf("no organizations available; run `kapibara login` first")
	}
	return orgs.Organizations[0].ID, nil
}

// resolveProject finds a project by id or name, optionally creating it by name.
func resolveProject(ctx context.Context, client *apiClient, cfg cliConfig, ref string, createIfMissing bool) (*project, error) {
	orgID, err := resolveOrg(ctx, client, cfg)
	if err != nil {
		return nil, err
	}
	var out struct {
		Projects []project `json:"projects"`
	}
	if err := client.do(ctx, http.MethodGet, "/api/v1/orgs/"+orgID+"/projects", nil, &out); err != nil {
		return nil, err
	}
	for i := range out.Projects {
		if out.Projects[i].ID == ref || out.Projects[i].Name == ref {
			return &out.Projects[i], nil
		}
	}
	if !createIfMissing {
		return nil, fmt.Errorf("project %q not found", ref)
	}
	return createProject(ctx, client, orgID, ref)
}

func createProject(ctx context.Context, client *apiClient, orgID, name string) (*project, error) {
	var p project
	if err := client.do(ctx, http.MethodPost, "/api/v1/orgs/"+orgID+"/projects",
		map[string]string{"name": name}, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ensureApp returns an existing application with the given name in the project,
// or creates one from the spec.
func ensureApp(ctx context.Context, client *apiClient, projectID string, spec appSpec) (*appInfo, error) {
	var list struct {
		Applications []appInfo `json:"applications"`
	}
	if err := client.do(ctx, http.MethodGet, "/api/v1/projects/"+projectID+"/apps", nil, &list); err != nil {
		return nil, err
	}
	body := map[string]any{
		"name": spec.Name, "buildType": spec.BuildType,
		"repoUrl": spec.RepoURL, "branch": spec.Branch, "contextDir": spec.ContextDir, "dockerfilePath": spec.DockerfilePath,
		"image": spec.Image, "port": spec.Port, "domain": spec.Domain, "tls": spec.TLS,
		"cpuLimit": spec.CPULimit, "memoryLimit": spec.MemoryLimit, "volumeSize": spec.VolumeSize,
	}
	if len(spec.Env) > 0 {
		body["env"] = spec.Env
	}
	if len(spec.SecretKeys) > 0 {
		body["secretKeys"] = spec.SecretKeys
	}
	if len(spec.Command) > 0 {
		body["command"] = spec.Command
	}
	var mounts []map[string]string
	for _, m := range spec.Mounts {
		name, path, ok := strings.Cut(m, ":")
		if !ok {
			return nil, fmt.Errorf("invalid --mount %q (want name:path)", m)
		}
		mounts = append(mounts, map[string]string{"name": name, "path": path})
	}
	if len(mounts) > 0 {
		body["mounts"] = mounts
	}
	// If the app already exists, update it (PUT) so env/domain/image changes on
	// re-deploy take effect; otherwise create it.
	for i := range list.Applications {
		if list.Applications[i].Name == spec.Name {
			id := list.Applications[i].ID
			if err := client.do(ctx, http.MethodPut, "/api/v1/apps/"+id, body, nil); err != nil {
				return nil, err
			}
			return &list.Applications[i], nil
		}
	}
	var created struct {
		Application appInfo `json:"application"`
	}
	if err := client.do(ctx, http.MethodPost, "/api/v1/projects/"+projectID+"/apps", body, &created); err != nil {
		return nil, err
	}
	return &created.Application, nil
}

// followDeployment polls a deployment until it reaches a terminal state,
// printing the incremental build/deploy log.
func followDeployment(ctx context.Context, client *apiClient, id string) error {
	printed := 0
	for {
		var dep struct {
			Status  string `json:"status"`
			Applied int    `json:"applied"`
			Log     string `json:"log"`
			Error   string `json:"error"`
			ImageRef string `json:"imageRef"`
		}
		if err := client.do(ctx, http.MethodGet, "/api/v1/deployments/"+id, nil, &dep); err != nil {
			return err
		}
		if len(dep.Log) > printed {
			fmt.Print(dep.Log[printed:])
			printed = len(dep.Log)
		}
		switch dep.Status {
		case "success":
			fmt.Printf("\n✓ deployment %s succeeded (%d objects applied, image %s)\n", id, dep.Applied, dep.ImageRef)
			return nil
		case "failed":
			return fmt.Errorf("deployment %s failed: %s", id, dep.Error)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
