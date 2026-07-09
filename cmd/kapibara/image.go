package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// imageCmd builds a container image with Docker and pushes it to Kapibara's
// registry gateway, handling the registry login automatically — so users don't
// run docker login/build/push by hand.
func imageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "image",
		Aliases: []string{"images"},
		Short:   "Build and push app images to the Kapibara registry",
	}

	var project, name, tag, dockerfile string
	var noPush bool
	build := &cobra.Command{
		Use:   "build [CONTEXT]",
		Short: "docker build an image and push it to the Kapibara registry gateway",
		Long: "Build an image with Docker and push it to the Kapibara registry, then\n" +
			"reference it in a deploy as registry/<project>/<name>:<tag>.\n\n" +
			"  kapibara image build --project worker --name api --tag v1 -f apps/api/Dockerfile .",
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" || name == "" {
				return fmt.Errorf("--project and --name are required")
			}
			if _, err := exec.LookPath("docker"); err != nil {
				return fmt.Errorf("docker is required on this machine to build images")
			}
			ctxDir := "."
			if len(args) > 0 {
				ctxDir = args[0]
			}
			if tag == "" {
				tag = "latest"
			}

			cfg := loadCLIConfig()
			client := newAPIClient(cfg)

			// Resolve the registry gateway host from the server.
			var conf struct {
				RegistryHost string `json:"registryHost"`
			}
			if err := client.do(cmd.Context(), http.MethodGet, "/api/v1/config", nil, &conf); err != nil {
				return err
			}
			if conf.RegistryHost == "" {
				return fmt.Errorf("the server has no registry gateway configured (KAPIBARA_REGISTRY_PUBLIC unset)")
			}
			ref := fmt.Sprintf("%s/registry/%s/%s:%s", conf.RegistryHost, project, name, tag)

			// Build.
			buildArgs := []string{"build", "-t", ref}
			if dockerfile != "" {
				buildArgs = append(buildArgs, "-f", dockerfile)
			}
			buildArgs = append(buildArgs, ctxDir)
			fmt.Printf("building %s …\n", ref)
			if err := runDocker(cmd.Context(), buildArgs...); err != nil {
				return fmt.Errorf("docker build failed: %w", err)
			}

			if noPush {
				fmt.Printf("✓ built %s (not pushed)\n", ref)
				return nil
			}

			// Log in to the gateway (mint + cache a kap_ token as the password).
			if err := ensureRegistryLogin(cmd.Context(), &cfg, client, conf.RegistryHost); err != nil {
				return err
			}
			fmt.Printf("pushing %s …\n", ref)
			if err := runDocker(cmd.Context(), "push", ref); err != nil {
				return fmt.Errorf("docker push failed: %w", err)
			}
			fmt.Printf("✓ pushed %s\n", ref)
			fmt.Printf("  reference it in orcinus.yml as:  image: registry/%s/%s:%s\n", project, name, tag)
			return nil
		},
	}
	build.Flags().StringVar(&project, "project", "", "project name (image namespace)")
	build.Flags().StringVar(&name, "name", "", "image name")
	build.Flags().StringVar(&tag, "tag", "latest", "image tag")
	build.Flags().StringVarP(&dockerfile, "dockerfile", "f", "", "path to the Dockerfile (default: <context>/Dockerfile)")
	build.Flags().BoolVar(&noPush, "no-push", false, "build only; do not push")

	cmd.AddCommand(build, packImageCmd())
	return cmd
}

// runDocker runs a docker command, streaming its output to the terminal.
func runDocker(ctx context.Context, args ...string) error {
	c := exec.CommandContext(ctx, "docker", args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// ensureRegistryLogin logs Docker into the registry gateway using a cached kap_
// API token (minting one on first use). Re-mints once if the cached token is
// rejected.
func ensureRegistryLogin(ctx context.Context, cfg *cliConfig, client *apiClient, host string) error {
	if cfg.Email == "" {
		return fmt.Errorf("not logged in; run `kapibara login` first")
	}
	if cfg.RegistryToken == "" {
		if err := mintRegistryToken(ctx, cfg, client); err != nil {
			return err
		}
	}
	if err := dockerLogin(ctx, host, cfg.Email, cfg.RegistryToken); err != nil {
		// Cached token may be stale — mint a fresh one and retry once.
		if e := mintRegistryToken(ctx, cfg, client); e != nil {
			return e
		}
		return dockerLogin(ctx, host, cfg.Email, cfg.RegistryToken)
	}
	return nil
}

func mintRegistryToken(ctx context.Context, cfg *cliConfig, client *apiClient) error {
	var out struct {
		Token string `json:"token"`
	}
	if err := client.do(ctx, http.MethodPost, "/api/v1/tokens", map[string]string{"name": "kapibara-cli"}, &out); err != nil {
		return err
	}
	if out.Token == "" {
		return fmt.Errorf("could not mint a registry token")
	}
	cfg.RegistryToken = out.Token
	_ = cfg.save()
	return nil
}

func dockerLogin(ctx context.Context, host, user, password string) error {
	c := exec.CommandContext(ctx, "docker", "login", host, "-u", user, "--password-stdin")
	c.Stdin = strings.NewReader(password)
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker login to %s failed: %s", host, strings.TrimSpace(string(out)))
	}
	return nil
}
