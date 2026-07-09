package main

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/spf13/cobra"
)

// packImageCmd assembles an OCI image entirely in-process (no Docker/daemon):
// pull a base image, append a layer with a local directory's files, set the
// runtime config, and push to the Kapibara registry gateway. It cannot run
// Dockerfile `RUN` steps — use it for compiled binaries, static sites, or other
// prebuilt artifacts. Because it repackages a chosen-platform base, it can build
// a linux/amd64 image from any host OS.
func packImageCmd() *cobra.Command {
	var project, name_, tag, base, dir, dest, workdir, osArch string
	var entrypoint, cmdArgs, env []string
	var port int
	cmd := &cobra.Command{
		Use:   "pack",
		Short: "Build an image from a base + a directory (in-process, no Docker) and push it",
		Long: "Assemble an OCI image without Docker: base image + your files as a layer.\n" +
			"For prebuilt artifacts (Go binaries, static sites) — it does not run RUN steps.\n\n" +
			"  kapibara image pack --project web --name site --tag v1 \\\n" +
			"    --base nginx:alpine --dir ./dist --dest /usr/share/nginx/html",
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" || name_ == "" || base == "" || dir == "" {
				return fmt.Errorf("--project, --name, --base and --dir are required")
			}
			if tag == "" {
				tag = "latest"
			}
			if dest == "" {
				dest = "/app"
			}
			cfg := loadCLIConfig()
			client := newAPIClient(cfg)
			var conf struct {
				RegistryHost string `json:"registryHost"`
			}
			if err := client.do(cmd.Context(), http.MethodGet, "/api/v1/config", nil, &conf); err != nil {
				return err
			}
			if conf.RegistryHost == "" {
				return fmt.Errorf("the server has no registry gateway configured")
			}
			destStr := fmt.Sprintf("%s/registry/%s/%s:%s", conf.RegistryHost, project, name_, tag)
			destRef, err := name.ParseReference(destStr)
			if err != nil {
				return err
			}

			// Parse target platform (the cluster is linux/amd64 by default).
			plat, err := v1.ParsePlatform(osArch)
			if err != nil {
				return err
			}

			// Pull the base image for the target platform (public → anonymous).
			baseRef, err := name.ParseReference(base)
			if err != nil {
				return err
			}
			fmt.Printf("pulling base %s (%s)…\n", base, osArch)
			baseImg, err := remote.Image(baseRef,
				remote.WithContext(cmd.Context()),
				remote.WithPlatform(*plat),
				remote.WithAuth(authn.Anonymous)) // public bases; avoids host cred helpers
			if err != nil {
				return fmt.Errorf("pull base: %w", err)
			}

			// Build a layer from the local directory.
			fmt.Printf("packing %s → %s\n", dir, dest)
			layer, err := dirToLayer(dir, dest)
			if err != nil {
				return err
			}
			img, err := mutate.AppendLayers(baseImg, layer)
			if err != nil {
				return err
			}

			// Apply runtime config on top of the base.
			cf, err := img.ConfigFile()
			if err != nil {
				return err
			}
			cf = cf.DeepCopy()
			if workdir != "" {
				cf.Config.WorkingDir = workdir
			}
			cf.Config.Env = append(cf.Config.Env, env...)
			if len(entrypoint) > 0 {
				cf.Config.Entrypoint = entrypoint
				cf.Config.Cmd = nil
			}
			if len(cmdArgs) > 0 {
				cf.Config.Cmd = cmdArgs
			}
			if port > 0 {
				if cf.Config.ExposedPorts == nil {
					cf.Config.ExposedPorts = map[string]struct{}{}
				}
				cf.Config.ExposedPorts[fmt.Sprintf("%d/tcp", port)] = struct{}{}
			}
			img, err = mutate.ConfigFile(img, cf)
			if err != nil {
				return err
			}

			// Push to the gateway (Basic creds → ggcr negotiates the Bearer token).
			tok, err := getRegistryToken(cmd.Context(), &cfg, client)
			if err != nil {
				return err
			}
			fmt.Printf("pushing %s …\n", destStr)
			if err := remote.Write(destRef, img,
				remote.WithContext(cmd.Context()),
				remote.WithAuth(&authn.Basic{Username: cfg.Email, Password: tok})); err != nil {
				return fmt.Errorf("push: %w", err)
			}
			fmt.Printf("✓ pushed %s\n", destStr)
			fmt.Printf("  reference it in orcinus.yml as:  image: registry/%s/%s:%s\n", project, name_, tag)
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project name (image namespace)")
	cmd.Flags().StringVar(&name_, "name", "", "image name")
	cmd.Flags().StringVar(&tag, "tag", "latest", "image tag")
	cmd.Flags().StringVar(&base, "base", "", "base image (e.g. nginx:alpine, gcr.io/distroless/static)")
	cmd.Flags().StringVar(&dir, "dir", "", "local directory to add as a layer")
	cmd.Flags().StringVar(&dest, "dest", "/app", "path inside the image to place --dir")
	cmd.Flags().StringVar(&workdir, "workdir", "", "container working directory")
	cmd.Flags().StringArrayVar(&entrypoint, "entrypoint", nil, "entrypoint (repeatable, in order)")
	cmd.Flags().StringArrayVar(&cmdArgs, "cmd", nil, "command args (repeatable, in order)")
	cmd.Flags().StringArrayVar(&env, "env", nil, "KEY=VALUE baked into the image (repeatable)")
	cmd.Flags().IntVar(&port, "port", 0, "exposed container port")
	cmd.Flags().StringVar(&osArch, "arch", "linux/amd64", "target platform os/arch")
	return cmd
}

// dirToLayer builds an (uncompressed-tar) image layer placing dir's contents
// under dest inside the image.
func dirToLayer(dir, dest string) (v1.Layer, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	dest = "/" + strings.Trim(strings.ReplaceAll(dest, "\\", "/"), "/")
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		name := strings.TrimPrefix(dest+"/"+filepath.ToSlash(rel), "/")
		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = name
		if d.IsDir() {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil // header carries the link target
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	b := buf.Bytes()
	return tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(b)), nil
	})
}

// getRegistryToken returns a cached kap_ token for the registry gateway, minting
// one if needed (shared with the docker-based `image build`).
func getRegistryToken(ctx context.Context, cfg *cliConfig, client *apiClient) (string, error) {
	if cfg.RegistryToken == "" {
		if err := mintRegistryToken(ctx, cfg, client); err != nil {
			return "", err
		}
	}
	return cfg.RegistryToken, nil
}
