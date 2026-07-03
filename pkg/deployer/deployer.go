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

	imageRef := app.Image
	var contextDir string

	// 1. Fetch source (unless a prebuilt image).
	if build.Type(app.BuildType) != build.Image {
		dir, err := os.MkdirTemp(d.Cfg.DataDir, "build-*")
		if err != nil {
			fail(err)
			return
		}
		defer os.RemoveAll(dir)

		fmt.Fprintf(sink, "cloning %s (%s)\n", app.RepoURL, app.Branch)
		sha, out, err := git.Clone(ctx, git.CloneOptions{
			RepoURL: app.RepoURL, Ref: app.Branch, Dir: dir + "/src",
		})
		sink.Write([]byte(out))
		if err != nil {
			fail(err)
			return
		}
		dep.CommitSHA = sha
		contextDir = dir + "/src"
		if app.ContextDir != "" {
			contextDir = contextDir + "/" + strings.TrimPrefix(app.ContextDir, "/")
		}
		imageRef = d.imageRef(project, app, sha)
	}

	dep.ImageRef = imageRef
	_ = d.Store.UpdateDeployment(dep)

	// 2. Build + publish (skipped for prebuilt images).
	builder := &build.Builder{ClusterContainer: d.Cfg.ClusterContainer, Push: d.Cfg.Push}
	if err := builder.Build(ctx, build.Request{
		Type:       build.Type(app.BuildType),
		ContextDir: contextDir,
		Dockerfile: app.DockerfilePath,
		ImageRef:   imageRef,
		Log:        sink,
	}); err != nil {
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
	res, err := d.Orcinus.Deploy(ctx, orcinus.DeployRequest{
		Source:    source,
		Project:   target,
		Wait:      true,
		ACMEEmail: acme,
	})
	if err != nil {
		fail(err)
		return
	}

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
		fmt.Sprintf("%d objects applied (image %s)", res.Applied, imageRef))
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
	}
	svc.AutoscaleMin = app.AutoscaleMin
	svc.AutoscaleMax = app.AutoscaleMax
	svc.AutoscaleCPU = app.AutoscaleCPU
	svc.AutoscaleMemory = app.AutoscaleMemory
	svc.Rollout = app.Rollout
	return compose.Project{Services: []compose.Service{svc}}.Render()
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
