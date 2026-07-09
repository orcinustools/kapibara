package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// upCmd uploads the local directory as a build context and has the server build
// it (railpack/Dockerfile) in-cluster and deploy it — no Docker or Git on the
// client. It is the local-source counterpart to `app deploy --repo`.
func upCmd() *cobra.Command {
	var project, name, buildType, path, contextDir, dockerfile, domain string
	var cpuLimit, memoryLimit, volumeSize string
	var mounts, envPairs, secretKeys, command []string
	var port int
	var tls, follow bool
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Upload local source and build + deploy it on the server (no Docker/Git needed)",
		Long: "Pack the local directory, upload it, and let the server build it in-cluster\n" +
			"(railpack auto-detect or a Dockerfile) and deploy it. Honors .dockerignore\n" +
			"(else .gitignore); .git is always excluded.\n\n" +
			"  kapibara up --project shop --name api --build railpack --path . \\\n" +
			"    --port 3000 --domain api.apps.example.com --tls --env NODE_ENV=production",
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" || name == "" {
				return fmt.Errorf("--project and --name are required")
			}
			if buildType == "" {
				buildType = "railpack"
			}
			if buildType != "railpack" && buildType != "dockerfile" && buildType != "nixpacks" {
				return fmt.Errorf("--build must be railpack, dockerfile or nixpacks for `up` (got %q)", buildType)
			}
			if path == "" {
				path = "."
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			if info, err := os.Stat(abs); err != nil || !info.IsDir() {
				return fmt.Errorf("--path %q is not a directory", path)
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
				Name: name, BuildType: buildType, // no RepoURL: source is uploaded
				ContextDir: contextDir, DockerfilePath: dockerfile, Port: port, Domain: domain, TLS: tls,
				CPULimit: cpuLimit, MemoryLimit: memoryLimit, VolumeSize: volumeSize, Mounts: mounts,
				Env: env, SecretKeys: secretKeys, Command: command,
			})
			if err != nil {
				return err
			}

			// Pack the directory to a temp gzip-tar so we can report its size and
			// upload it with a clean error if packing fails.
			fmt.Printf("packing %s …\n", abs)
			tmp, err := os.CreateTemp("", "kapibara-up-*.tgz")
			if err != nil {
				return err
			}
			defer os.Remove(tmp.Name())
			n, files, err := tarDir(abs, tmp)
			tmp.Close()
			if err != nil {
				return fmt.Errorf("pack context: %w", err)
			}
			fmt.Printf("packed %d file(s), %.1f MiB\n", files, float64(n)/(1<<20))

			f, err := os.Open(tmp.Name())
			if err != nil {
				return err
			}
			defer f.Close()
			fmt.Printf("uploading context to %s …\n", strings.TrimRight(cfg.Server, "/"))
			var up struct {
				Status string `json:"status"`
				Bytes  int64  `json:"bytes"`
			}
			if err := client.postReader(cmd.Context(), "/api/v1/apps/"+app.ID+"/source", "application/gzip", f, &up); err != nil {
				return err
			}
			fmt.Printf("uploaded %d bytes\n", up.Bytes)

			fmt.Printf("building & deploying %s on the server…\n", app.Name)
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
	cmd.Flags().StringVar(&project, "project", "", "project name or id")
	cmd.Flags().StringVar(&name, "name", "", "application name")
	cmd.Flags().StringVar(&buildType, "build", "railpack", "build type: railpack | dockerfile | nixpacks")
	cmd.Flags().StringVar(&path, "path", ".", "local directory to upload as the build context")
	cmd.Flags().StringVar(&contextDir, "context-dir", "", "build context subdirectory (monorepos)")
	cmd.Flags().StringVar(&dockerfile, "dockerfile", "", "path to Dockerfile (relative to the context dir)")
	cmd.Flags().IntVar(&port, "port", 0, "container port to expose")
	cmd.Flags().StringVar(&domain, "domain", "", "ingress host/domain")
	cmd.Flags().BoolVar(&tls, "tls", false, "enable TLS (cert-manager) for the domain")
	cmd.Flags().StringVar(&cpuLimit, "cpu-limit", "", "CPU limit in cores, e.g. 0.5")
	cmd.Flags().StringVar(&memoryLimit, "memory-limit", "", "memory limit, e.g. 512M")
	cmd.Flags().StringArrayVar(&mounts, "mount", nil, "persistent volume mount name:path (repeatable)")
	cmd.Flags().StringVar(&volumeSize, "volume-size", "", "PVC size for mounts, e.g. 1Gi")
	cmd.Flags().StringArrayVar(&envPairs, "env", nil, "environment variable KEY=VALUE (repeatable)")
	cmd.Flags().StringArrayVar(&secretKeys, "secret", nil, "mark an --env key as a cluster Secret (repeatable)")
	cmd.Flags().StringArrayVar(&command, "command", nil, "override the container command (repeatable, in order)")
	cmd.Flags().BoolVar(&follow, "follow", true, "stream deployment status/logs until it finishes")
	return cmd
}

// postReader POSTs a raw body (e.g. an upload) and decodes a JSON response.
func (a *apiClient) postReader(ctx context.Context, path, contentType string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.server+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}
	// A dedicated client with no overall timeout: large context uploads can take
	// longer than the JSON client's timeout.
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return fmt.Errorf("upload %s: %w", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(raw))
		return fmt.Errorf("POST %s → HTTP %d: %s", path, resp.StatusCode, msg)
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// tarDir writes a gzip-compressed tar of root to w, honoring ignore rules, and
// returns the bytes written and the number of files included.
func tarDir(root string, w io.Writer) (int64, int, error) {
	cw := &countWriter{w: w}
	gz := gzip.NewWriter(cw)
	tw := tar.NewWriter(gz)
	patterns := loadIgnore(root)
	files := 0

	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if ignored(rel, info.IsDir(), patterns) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// Only regular files and symlinks (dirs are implied by file paths).
		if info.IsDir() {
			return nil
		}
		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			if link, err = os.Readlink(p); err != nil {
				return err
			}
		}
		hdr, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		hdr.Name = rel
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			_, err = io.Copy(tw, f)
			f.Close()
			if err != nil {
				return err
			}
			files++
		}
		return nil
	})
	if err != nil {
		return cw.n, files, err
	}
	if err := tw.Close(); err != nil {
		return cw.n, files, err
	}
	if err := gz.Close(); err != nil {
		return cw.n, files, err
	}
	return cw.n, files, nil
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// loadIgnore reads ignore patterns from .dockerignore (preferred) or .gitignore.
func loadIgnore(root string) []string {
	for _, name := range []string{".dockerignore", ".gitignore"} {
		f, err := os.Open(filepath.Join(root, name))
		if err != nil {
			continue
		}
		var pats []string
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
				continue // basic subset: no negation
			}
			pats = append(pats, strings.Trim(line, "/"))
		}
		f.Close()
		return pats
	}
	return nil
}

// ignored reports whether rel (a slash path) should be excluded. .git is always
// excluded; otherwise a pragmatic subset of ignore semantics is applied: a
// pattern with no slash matches any path component; a pattern with a slash is
// matched against the full path and as a directory prefix.
func ignored(rel string, isDir bool, patterns []string) bool {
	first := rel
	if i := strings.IndexByte(rel, '/'); i >= 0 {
		first = rel[:i]
	}
	if first == ".git" {
		return true
	}
	base := rel
	if i := strings.LastIndexByte(rel, '/'); i >= 0 {
		base = rel[i+1:]
	}
	for _, pat := range patterns {
		if !strings.Contains(pat, "/") {
			// component match: any segment or the basename
			for _, seg := range strings.Split(rel, "/") {
				if ok, _ := filepath.Match(pat, seg); ok {
					return true
				}
			}
			if ok, _ := filepath.Match(pat, base); ok {
				return true
			}
			continue
		}
		if ok, _ := filepath.Match(pat, rel); ok {
			return true
		}
		if rel == pat || strings.HasPrefix(rel, pat+"/") {
			return true
		}
	}
	return false
}
