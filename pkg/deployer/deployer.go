// Package deployer orchestrates the application build+publish+deploy pipeline
// against the orcinus cluster engine. Deploys run asynchronously and stream
// their progress into a Deployment record.
package deployer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/orcinustools/kapibara/pkg/build"
	"github.com/orcinustools/kapibara/pkg/compose"
	"github.com/orcinustools/kapibara/pkg/git"
	"github.com/orcinustools/kapibara/pkg/orcinus"
	"github.com/orcinustools/kapibara/pkg/store"
)

// Config configures how images are built and published.
type Config struct {
	RegistryPrefix   string
	Push             bool
	ClusterContainer string
	DataDir          string
	// RegistryPublic rewrites app images written as "kapibara/<path>" to
	// "<RegistryPublic>/<path>" at deploy so the cluster pulls them from the
	// registry gateway (e.g. kapibara.mayar.io).
	RegistryPublic string

	// InClusterBuild builds Git sources server-side with buildctl + BuildKit
	// (no Docker) and pushes to the in-cluster registry; the cluster then pulls
	// the image back through the public gateway.
	InClusterBuild bool
	// BuildkitAddr is the BuildKit daemon buildctl connects to.
	BuildkitAddr string
	// BuildPlatform is the target platform for in-cluster builds (linux/amd64).
	BuildPlatform string
	// RailpackFrontend is the railpack BuildKit frontend image.
	RailpackFrontend string
	// RegistryUpstream is the in-cluster registry buildkit pushes to over HTTP
	// (e.g. http://registry.orcinus-registry.svc:5000).
	RegistryUpstream string
}

// Deployer runs application deployments.
type Deployer struct {
	Store   *store.Store
	Orcinus *orcinus.Client
	Cfg     Config
	// Dispatch, if set, is called after a deployment finishes so the caller can
	// send notifications. orgID is the owning organization.
	Dispatch func(ctx context.Context, orgID string, success bool, title, message string)
}

func (d *Deployer) notify(ctx context.Context, project *store.Project, success bool, title, msg string) {
	if d.Dispatch != nil {
		d.Dispatch(ctx, project.OrganizationID, success, title, msg)
	}
}

// New returns a Deployer.
func New(st *store.Store, oc *orcinus.Client, cfg Config) *Deployer {
	return &Deployer{Store: st, Orcinus: oc, Cfg: cfg}
}

// logSink accumulates build/deploy output and periodically flushes it to the
// Deployment record so the UI can poll progress.
type logSink struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	dep  *store.Deployment
	st   *store.Store
	last time.Time
}

func (l *logSink) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n, _ := l.buf.Write(p)
	if time.Since(l.last) > 500*time.Millisecond {
		l.dep.Log = l.buf.String()
		_ = l.st.UpdateDeployment(l.dep)
		l.last = time.Now()
	}
	return n, nil
}

func (l *logSink) flush() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.dep.Log = l.buf.String()
}

// DeployApplication creates a Deployment and runs the pipeline. If async, it
// returns immediately with a pending/running deployment; otherwise it blocks.
func (d *Deployer) DeployApplication(ctx context.Context, app *store.Application, project *store.Project, async bool) (*store.Deployment, error) {
	dep := &store.Deployment{
		ProjectID:     project.ID,
		ApplicationID: app.ID,
		Kind:          "application",
		Status:        store.DeployPending,
	}
	if err := d.Store.CreateDeployment(dep); err != nil {
		return nil, err
	}
	if async {
		go d.run(context.Background(), app, project, dep)
		return dep, nil
	}
	d.run(ctx, app, project, dep)
	return d.Store.DeploymentByID(dep.ID)
}

// targetProject returns the isolated orcinus project an application deploys to.
func targetProject(app *store.Application, project *store.Project) string {
	if app.OrcinusProject != "" {
		return app.OrcinusProject
	}
	return project.OrcinusProject
}

func (d *Deployer) run(ctx context.Context, app *store.Application, project *store.Project, dep *store.Deployment) {
	sink := &logSink{dep: dep, st: d.Store}
	dep.Status = store.DeployRunning
	_ = d.Store.UpdateDeployment(dep)

	fail := func(err error) {
		sink.flush()
		dep.Status = store.DeployFailed
		dep.Error = err.Error()
		dep.Log = sink.buf.String()
		_ = d.Store.UpdateDeployment(dep)
		d.notify(ctx, project, false, "Deploy failed: "+app.Name, err.Error())
	}

	imageRef := RewriteRegistryImage(app.Image, d.Cfg.RegistryPublic, d.orgScope(project))
	var contextDir string

	// 1. Fetch source (unless a prebuilt image).
	if build.Type(app.BuildType) != build.Image {
		dir, err := os.MkdirTemp(d.Cfg.DataDir, "build-*")
		if err != nil {
			fail(err)
			return
		}
		defer os.RemoveAll(dir)

		var sha string
		if app.SourceArchive != "" {
			// Uploaded local context (`kapibara up`): extract the tarball instead
			// of cloning. The image tag is a content hash of the archive.
			fmt.Fprintf(sink, "extracting uploaded source (%s)\n", filepath.Base(app.SourceArchive))
			h, err := extractTarGz(app.SourceArchive, dir+"/src")
			if err != nil {
				fail(fmt.Errorf("extract uploaded source: %w", err))
				return
			}
			sha = h
		} else {
			fmt.Fprintf(sink, "cloning %s (%s)\n", app.RepoURL, app.Branch)
			// Inject the connected git provider's token for private repos. The
			// token is redacted from clone output by pkg/git.
			token := ""
			if app.GitProviderID != "" {
				if gp, err := d.Store.GitProviderByID(app.GitProviderID); err == nil {
					token = gp.Token
				}
			}
			s, out, err := git.Clone(ctx, git.CloneOptions{
				RepoURL: app.RepoURL, Ref: app.Branch, Dir: dir + "/src", Token: token,
			})
			sink.Write([]byte(out))
			if err != nil {
				fail(err)
				return
			}
			sha = s
		}
		dep.CommitSHA = sha
		contextDir = dir + "/src"
		if app.ContextDir != "" {
			contextDir = contextDir + "/" + strings.TrimPrefix(app.ContextDir, "/")
		}
		imageRef = d.imageRef(project, app, sha)
	}

	// 2. Build + publish (skipped for prebuilt images).
	builder := &build.Builder{ClusterContainer: d.Cfg.ClusterContainer, Push: d.Cfg.Push}
	req := build.Request{
		Type:       build.Type(app.BuildType),
		ContextDir: contextDir,
		Dockerfile: app.DockerfilePath,
		ImageRef:   imageRef,
		Log:        sink,
	}
	// Server-side, Docker-less build: buildkit pushes to the in-cluster registry
	// and the cluster pulls the image back through the public gateway. The push
	// target (internal registry host) and the pull reference (gateway host) share
	// the same org-scoped repository path.
	if d.Cfg.InClusterBuild && build.Type(app.BuildType) != build.Image {
		builder.Frontend = true
		builder.BuildkitAddr = d.Cfg.BuildkitAddr
		builder.Platform = d.Cfg.BuildPlatform
		builder.RailpackFrontend = d.Cfg.RailpackFrontend
		repo := d.registryRepo(project, app, dep.CommitSHA) // registry/<scope>/kapibara/<proj>-<app>:<tag>
		if host := registryHost(d.Cfg.RegistryUpstream); host != "" {
			req.PushImage = host + "/" + repo
		}
		if d.Cfg.RegistryPublic != "" {
			imageRef = strings.TrimRight(d.Cfg.RegistryPublic, "/") + "/" + repo
			req.ImageRef = imageRef
		}
	}

	dep.ImageRef = imageRef
	_ = d.Store.UpdateDeployment(dep)

	if err := builder.Build(ctx, req); err != nil {
		fail(err)
		return
	}

	// 3. Generate compose from the application model.
	source, err := d.composeFor(app, imageRef)
	if err != nil {
		fail(err)
		return
	}
	dep.Source = source

	// 4. Deploy to the cluster via orcinus (isolated per-unit project).
	target := targetProject(app, project)
	fmt.Fprintf(sink, "\ndeploying to cluster as project %q\n", target)
	acme := os.Getenv("KAPIBARA_ACME_EMAIL")
	// Apply without blocking on readiness: orcinus returns as soon as the
	// objects are applied. Waiting made this POST exceed the client timeout for
	// slower workloads (image pull, migrations). The rollout continues in the
	// cluster; follow it with `kapibara deployment status` / `orcinus ps`.
	res, err := d.Orcinus.Deploy(ctx, orcinus.DeployRequest{
		Source:    source,
		Project:   target,
		Wait:      false,
		ACMEEmail: acme,
	})
	if err != nil {
		fail(err)
		return
	}

	fmt.Fprintf(sink, "applied %d object(s); rollout continues in the cluster\n", res.Applied)
	sink.flush()
	dep.Status = store.DeploySuccess
	dep.Applied = res.Applied
	if b, e := json.Marshal(res.Installed); e == nil {
		dep.Installed = string(b)
	}
	dep.Log = sink.buf.String()
	_ = d.Store.UpdateDeployment(dep)

	app.CurrentImage = imageRef
	_ = d.Store.UpdateApplication(app)

	d.notify(ctx, project, true, "Deployed "+app.Name,
		fmt.Sprintf("%d objects applied (image %s); rollout in progress", res.Applied, imageRef))
}

// composeFor renders a single-service compose file from an application.
func (d *Deployer) composeFor(app *store.Application, imageRef string) (string, error) {
	svc := compose.Service{
		Name:     sanitize(app.Name),
		Image:    imageRef,
		Replicas: app.Replicas,
	}
	if app.Port > 0 {
		svc.Ports = []string{fmt.Sprintf("%d", app.Port)}
	}
	if env := parseEnv(app.Env); len(env) > 0 {
		svc.Env = env
	}
	if keys := parseList(app.SecretKeys); len(keys) > 0 {
		svc.Secrets = keys
	}
	if app.Domain != "" {
		svc.Expose = compose.ExposeIngress
		svc.Host = app.Domain
		svc.TLS = app.TLS
		svc.Path = app.Path
	}
	svc.AutoscaleMin = app.AutoscaleMin
	svc.AutoscaleMax = app.AutoscaleMax
	svc.AutoscaleCPU = app.AutoscaleCPU
	svc.AutoscaleMemory = app.AutoscaleMemory
	svc.Rollout = app.Rollout

	// Resource limits/reservations.
	svc.CPULimit = app.CPULimit
	svc.MemLimit = app.MemoryLimit
	svc.CPUReservation = app.CPURequest
	svc.MemReservation = app.MemoryRequest

	// Command override.
	if cmd := parseList(app.Command); len(cmd) > 0 {
		svc.Command = cmd
	}
	// Exec liveness probe.
	if test := parseList(app.HealthCmd); len(test) > 0 {
		svc.Health = &compose.HealthCheck{Test: test}
	}

	// Persistent volume mounts (named volumes → PVC).
	var projectVolumes []string
	for _, mnt := range parseMounts(app.Mounts) {
		if mnt.Name == "" || mnt.Path == "" {
			continue
		}
		svc.Volumes = append(svc.Volumes, mnt.Name+":"+mnt.Path)
		projectVolumes = append(projectVolumes, mnt.Name)
	}
	if len(svc.Volumes) > 0 && app.VolumeSize != "" {
		svc.VolumeSize = app.VolumeSize
	}
	return compose.Project{Services: []compose.Service{svc}, Volumes: projectVolumes}.Render()
}

// mount is one persistent volume mount for an application.
type mount struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func parseMounts(s string) []mount {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var m []mount
	_ = json.Unmarshal([]byte(s), &m)
	return m
}

// imageRef builds the tagged image name for a build.
func (d *Deployer) imageRef(project *store.Project, app *store.Application, sha string) string {
	tag := "latest"
	if len(sha) >= 12 {
		tag = sha[:12]
	}
	name := fmt.Sprintf("kapibara/%s-%s", project.OrcinusProject, sanitize(app.Name))
	if d.Cfg.RegistryPrefix != "" {
		name = strings.TrimRight(d.Cfg.RegistryPrefix, "/") + "/" + name
	}
	return name + ":" + tag
}

// registryRepo returns the org-scoped repository path (no host) used for
// in-cluster builds: "registry/<scope>/kapibara/<orcinusProject>-<app>:<tag>".
// The buildkit push target and the gateway pull reference share this path, so
// what buildkit pushes to the in-cluster registry is exactly what the cluster
// pulls back through the public gateway.
func (d *Deployer) registryRepo(project *store.Project, app *store.Application, sha string) string {
	tag := "latest"
	if len(sha) >= 12 {
		tag = sha[:12]
	}
	name := fmt.Sprintf("kapibara/%s-%s", project.OrcinusProject, sanitize(app.Name))
	repo := "registry/" + name
	if scope := d.orgScope(project); scope != "" {
		repo = "registry/" + scope + "/" + name
	}
	return repo + ":" + tag
}

// registryHost strips the scheme from a registry upstream URL, yielding the
// host[:port] buildkit pushes to (e.g. registry.orcinus-registry.svc:5000).
func registryHost(upstream string) string {
	u := strings.TrimSpace(upstream)
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimPrefix(u, "https://")
	return strings.TrimRight(u, "/")
}

// orgScope returns the org slug used to namespace a project's registry images.
func (d *Deployer) orgScope(project *store.Project) string {
	if org, err := d.Store.OrgByID(project.OrganizationID); err == nil {
		return org.Slug
	}
	return ""
}

// rewriteRegistryImage expands a Kapibara registry image reference to the full,
// org-scoped path the cluster pulls through the gateway. It handles the
// "registry/<project>/<image>:tag" convention (optionally already prefixed with
// the public host), inserting the org scope right after "registry/":
//
//	registry/worker/api:1                 → <host>/registry/<scope>/worker/api:1
//	<host>/registry/worker/api:1          → <host>/registry/<scope>/worker/api:1
//	<host>/registry/<scope>/worker/api:1  → unchanged (idempotent)
//
// External references (nginx:alpine, ghcr.io/...) pass through unchanged.
//
// Exported so the compose deploy path can rewrite each service image too.
func RewriteRegistryImage(image, publicHost, scope string) string {
	host := strings.TrimRight(publicHost, "/")
	if host == "" {
		return image
	}
	path := strings.TrimPrefix(image, host+"/") // drop the host if already present
	if !strings.HasPrefix(path, "registry/") {
		return image // not a Kapibara-registry reference
	}
	rest := strings.TrimPrefix(path, "registry/")
	if scope != "" && !strings.HasPrefix(rest, scope+"/") {
		rest = scope + "/" + rest
	}
	return host + "/registry/" + rest
}

func parseEnv(s string) map[string]string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	m := map[string]string{}
	_ = json.Unmarshal([]byte(s), &m)
	return m
}

func parseList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var l []string
	_ = json.Unmarshal([]byte(s), &l)
	return l
}

var nonDNS = regexp.MustCompile(`[^a-z0-9-]+`)

func sanitize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonDNS.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "app"
	}
	return s
}
