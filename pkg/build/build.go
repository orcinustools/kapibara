// Package build turns source into a container image and makes it available to
// the cluster — either by pushing to a registry or importing directly into the
// cluster's containerd (single-node / no-registry dev mode).
package build

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Type selects how source is built into an image.
type Type string

const (
	// Dockerfile builds using a Dockerfile in the build context.
	Dockerfile Type = "dockerfile"
	// Nixpacks builds using nixpacks (auto-detects the language).
	Nixpacks Type = "nixpacks"
	// Railpack builds using Railway's railpack (auto-detects the language;
	// nixpacks' successor). Requires a reachable BuildKit (BUILDKIT_HOST).
	Railpack Type = "railpack"
	// Image skips building and uses a prebuilt image reference.
	Image Type = "image"
)

// Request describes a build.
type Request struct {
	Type       Type
	ContextDir string // directory containing the source
	Dockerfile string // path relative to ContextDir (default "Dockerfile")
	ImageRef   string // tag to produce, e.g. kapibara/shop-web:abc123
	// PushImage, when set (Frontend mode), is the registry reference buildctl
	// pushes to. It may differ from ImageRef: the cluster pulls ImageRef through
	// the public gateway while buildkit pushes to the in-cluster registry.
	PushImage string
	Log       io.Writer
}

// Builder builds images from source.
type Builder struct {
	// ClusterContainer, if set, is the docker container name of the single-node
	// k3s node; images are imported into its containerd instead of pushed.
	ClusterContainer string
	// Push, if true, runs `docker push` after building (registry mode).
	Push bool

	// Frontend enables server-side, Docker-less builds: source is built with
	// `buildctl` against a BuildKit daemon (using a BuildKit frontend) and the
	// image is pushed straight to the registry. This is how the in-cluster
	// control-plane builds from Git without a Docker daemon. Requires buildctl
	// (and railpack, for railpack builds) on PATH and a reachable BuildkitAddr.
	Frontend bool
	// BuildkitAddr is the buildctl --addr (e.g. tcp://buildkitd:1234). Falls
	// back to $BUILDKIT_HOST when empty.
	BuildkitAddr string
	// Platform is the target build platform (e.g. linux/amd64). Defaults to
	// linux/amd64 so builds run for the cluster regardless of the host arch.
	Platform string
	// RailpackFrontend is the railpack BuildKit frontend image, pinned to the
	// railpack version in the image (e.g. ghcr.io/railwayapp/railpack-frontend).
	RailpackFrontend string
}

// Build produces the image described by req and publishes it to the cluster.
func (b *Builder) Build(ctx context.Context, req Request) error {
	log := req.Log
	if log == nil {
		log = io.Discard
	}

	// Server-side path: build with buildctl + a BuildKit frontend and push the
	// image directly to the registry — no Docker daemon on this host.
	if b.Frontend {
		return b.frontendBuild(ctx, req, log)
	}

	switch req.Type {
	case Dockerfile:
		if err := b.dockerBuild(ctx, req, log); err != nil {
			return err
		}
	case Nixpacks:
		if err := b.nixpacksBuild(ctx, req, log); err != nil {
			return err
		}
	case Railpack:
		if err := b.railpackBuild(ctx, req, log); err != nil {
			return err
		}
	case Image:
		// Nothing to build; the image is expected to be pullable already.
		return nil
	default:
		return fmt.Errorf("unknown build type %q", req.Type)
	}

	return b.publish(ctx, req.ImageRef, log)
}

func (b *Builder) dockerBuild(ctx context.Context, req Request, log io.Writer) error {
	dockerfile := req.Dockerfile
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	if _, err := os.Stat(filepath.Join(req.ContextDir, dockerfile)); err != nil {
		return fmt.Errorf("dockerfile %q not found in context", dockerfile)
	}
	args := []string{"build", "-t", req.ImageRef, "-f", filepath.Join(req.ContextDir, dockerfile), req.ContextDir}
	return run(ctx, log, "docker", args...)
}

func (b *Builder) nixpacksBuild(ctx context.Context, req Request, log io.Writer) error {
	if _, err := exec.LookPath("nixpacks"); err != nil {
		return fmt.Errorf("nixpacks not installed; use build type %q or install nixpacks", Dockerfile)
	}
	return run(ctx, log, "nixpacks", "build", req.ContextDir, "--name", req.ImageRef)
}

// railpackBuild builds with Railway's railpack (auto-detects the stack, no
// Dockerfile). railpack drives BuildKit, so BUILDKIT_HOST must point at a
// reachable buildkitd (e.g. `docker run --privileged -d --name buildkit
// moby/buildkit` + BUILDKIT_HOST=docker-container://buildkit). It loads the
// result into the local Docker image store, so publish() can push/import it.
func (b *Builder) railpackBuild(ctx context.Context, req Request, log io.Writer) error {
	if _, err := exec.LookPath("railpack"); err != nil {
		return fmt.Errorf("railpack not installed; install it (https://github.com/railwayapp/railpack) or use build type %q", Dockerfile)
	}
	if os.Getenv("BUILDKIT_HOST") == "" {
		return fmt.Errorf("railpack needs a BuildKit: start one and set BUILDKIT_HOST " +
			"(e.g. `docker run --rm --privileged -d --name buildkit moby/buildkit` then " +
			"BUILDKIT_HOST=docker-container://buildkit)")
	}
	return run(ctx, log, "railpack", "build", req.ContextDir, "--name", req.ImageRef)
}

// frontendBuild builds with buildctl against a BuildKit daemon and pushes the
// resulting image straight to the registry — no Docker daemon required. This is
// the in-cluster build path used by the control-plane: for railpack it runs
// `railpack prepare` to generate a build plan and hands it to the railpack
// BuildKit frontend; for Dockerfile it uses the built-in dockerfile frontend.
func (b *Builder) frontendBuild(ctx context.Context, req Request, log io.Writer) error {
	addr := b.BuildkitAddr
	if addr == "" {
		addr = os.Getenv("BUILDKIT_HOST")
	}
	if addr == "" {
		return fmt.Errorf("in-cluster build needs a BuildKit daemon: set BUILDKIT_HOST (e.g. tcp://buildkitd:1234)")
	}
	if _, err := exec.LookPath("buildctl"); err != nil {
		return fmt.Errorf("buildctl not found in the image; the control-plane image must bundle buildctl for in-cluster builds")
	}
	pushRef := req.PushImage
	if pushRef == "" {
		pushRef = req.ImageRef
	}
	if pushRef == "" {
		return fmt.Errorf("no image reference to push")
	}
	platform := b.Platform
	if platform == "" {
		platform = "linux/amd64"
	}
	// The in-cluster registry is served over plain HTTP; allow buildkit to push
	// insecurely (the daemon config also marks the host http=true).
	output := fmt.Sprintf("type=image,name=%s,push=true,registry.insecure=true", pushRef)

	switch req.Type {
	case Railpack:
		if _, err := exec.LookPath("railpack"); err != nil {
			return fmt.Errorf("railpack not found in the image; required to prepare railpack build plans")
		}
		planDir, err := os.MkdirTemp("", "railpack-plan-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(planDir)
		if err := run(ctx, log, "railpack", "prepare", req.ContextDir,
			"--plan-out", filepath.Join(planDir, "railpack-plan.json"),
			"--info-out", filepath.Join(planDir, "railpack-info.json")); err != nil {
			return fmt.Errorf("railpack prepare: %w", err)
		}
		frontend := b.RailpackFrontend
		if frontend == "" {
			frontend = "ghcr.io/railwayapp/railpack-frontend"
		}
		return run(ctx, log, "buildctl", "--addr", addr, "build",
			"--local", "context="+req.ContextDir,
			"--local", "dockerfile="+planDir,
			"--opt", "filename=railpack-plan.json",
			"--frontend=gateway.v0",
			"--opt", "source="+frontend,
			"--opt", "platform="+platform,
			"--output", output)
	case Dockerfile:
		dockerfile := req.Dockerfile
		if dockerfile == "" {
			dockerfile = "Dockerfile"
		}
		if _, err := os.Stat(filepath.Join(req.ContextDir, dockerfile)); err != nil {
			return fmt.Errorf("dockerfile %q not found in context", dockerfile)
		}
		return run(ctx, log, "buildctl", "--addr", addr, "build",
			"--frontend=dockerfile.v0",
			"--local", "context="+req.ContextDir,
			"--local", "dockerfile="+req.ContextDir,
			"--opt", "filename="+dockerfile,
			"--opt", "platform="+platform,
			"--output", output)
	case Nixpacks:
		return fmt.Errorf("nixpacks is not supported for in-cluster builds; use railpack or a Dockerfile")
	case Image:
		return nil
	default:
		return fmt.Errorf("unknown build type %q", req.Type)
	}
}

// publish makes the built image available to the cluster.
func (b *Builder) publish(ctx context.Context, imageRef string, log io.Writer) error {
	if b.Push {
		return run(ctx, log, "docker", "push", imageRef)
	}
	if b.ClusterContainer != "" {
		return b.importIntoCluster(ctx, imageRef, log)
	}
	// Neither push nor import configured: assume the cluster can already pull it.
	return nil
}

// importIntoCluster saves the image and imports it into the single-node k3s
// containerd, so no registry is required for local/dev deploys.
func (b *Builder) importIntoCluster(ctx context.Context, imageRef string, log io.Writer) error {
	tar, err := os.CreateTemp("", "kapibara-img-*.tar")
	if err != nil {
		return err
	}
	tarPath := tar.Name()
	tar.Close()
	defer os.Remove(tarPath)

	if err := run(ctx, log, "docker", "save", "-o", tarPath, imageRef); err != nil {
		return fmt.Errorf("docker save: %w", err)
	}
	inside := "/tmp/" + filepath.Base(tarPath)
	if err := run(ctx, log, "docker", "cp", tarPath, b.ClusterContainer+":"+inside); err != nil {
		return fmt.Errorf("docker cp to cluster: %w", err)
	}
	// k3s bundles ctr; import into the k3s containerd namespace.
	if err := run(ctx, log, "docker", "exec", b.ClusterContainer,
		"ctr", "-a", "/run/k3s/containerd/containerd.sock", "-n", "k8s.io",
		"images", "import", inside); err != nil {
		return fmt.Errorf("ctr images import: %w", err)
	}
	_ = run(ctx, log, "docker", "exec", b.ClusterContainer, "rm", "-f", inside)
	return nil
}

func run(ctx context.Context, log io.Writer, name string, args ...string) error {
	fmt.Fprintf(log, "$ %s %s\n", name, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = log
	cmd.Stderr = log
	return cmd.Run()
}
