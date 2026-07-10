package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/orcinustools/kapibara/pkg/version"
	"github.com/spf13/cobra"
)

// updateRepo is the GitHub repository releases are pulled from.
const updateRepo = "orcinustools/kapibara"

// updateCmd self-updates the kapibara binary to a release build, downloading the
// archive for this OS/arch, verifying its checksum, and atomically replacing the
// target binary (the running executable by default, or --path).
func updateCmd() *cobra.Command {
	var target, wantVersion string
	var checkOnly, force bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update the kapibara binary to the latest release",
		Long: "Download the latest (or --version) release for this OS/arch, verify its\n" +
			"checksum, and replace the binary at the target location (the running\n" +
			"executable by default, or --path). Set GITHUB_TOKEN for a private repo.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cur := version.Version

			// Resolve the release tag.
			tag := wantVersion
			if tag == "" {
				latest, err := latestTag(ctx)
				if err != nil {
					return fmt.Errorf("resolve latest release: %w", err)
				}
				tag = latest
			}
			if !strings.HasPrefix(tag, "v") {
				tag = "v" + tag
			}
			fmt.Printf("current: %s   latest: %s\n", cur, tag)

			if checkOnly {
				if sameVersion(cur, tag) {
					fmt.Println("up to date")
				} else {
					fmt.Printf("update available: %s → %s (run `kapibara update`)\n", cur, tag)
				}
				return nil
			}
			if sameVersion(cur, tag) && !force {
				fmt.Println("already up to date (use --force to reinstall)")
				return nil
			}

			// Resolve the target path (default: the running executable).
			dest, err := resolveTarget(target)
			if err != nil {
				return err
			}

			// Build the archive name (goreleaser strips the leading v).
			archive := fmt.Sprintf("kapibara_%s_%s_%s.tar.gz", strings.TrimPrefix(tag, "v"), runtime.GOOS, runtime.GOARCH)
			base := fmt.Sprintf("https://github.com/%s/releases/download/%s", updateRepo, tag)

			fmt.Printf("downloading %s …\n", archive)
			data, err := download(ctx, base+"/"+archive)
			if err != nil {
				return fmt.Errorf("download %s: %w", archive, err)
			}
			// Verify checksum when checksums.txt is present.
			if sums, err := download(ctx, base+"/checksums.txt"); err == nil {
				if want := checksumFor(string(sums), archive); want != "" {
					got := sha256.Sum256(data)
					if hex.EncodeToString(got[:]) != want {
						return fmt.Errorf("checksum mismatch for %s", archive)
					}
					fmt.Println("checksum ok")
				}
			}

			bin, err := extractBinary(data, "kapibara")
			if err != nil {
				return err
			}
			if err := replaceBinary(dest, bin); err != nil {
				return err
			}
			fmt.Printf("✓ updated %s → %s\n", dest, tag)
			return nil
		},
	}
	cmd.Flags().StringVar(&target, "path", "", "target binary path or directory (default: the running kapibara)")
	cmd.Flags().StringVar(&wantVersion, "version", "", "release tag to install (default: latest)")
	cmd.Flags().BoolVar(&checkOnly, "check", false, "only report whether an update is available")
	cmd.Flags().BoolVar(&force, "force", false, "reinstall even if already on the target version")
	return cmd
}

func sameVersion(a, b string) bool {
	return strings.TrimPrefix(a, "v") == strings.TrimPrefix(b, "v")
}

// resolveTarget returns the binary path to write. Empty → the running
// executable (symlinks resolved). A directory → <dir>/kapibara.
func resolveTarget(p string) (string, error) {
	if p == "" {
		exe, err := os.Executable()
		if err != nil {
			return "", err
		}
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		return exe, nil
	}
	if fi, err := os.Stat(p); err == nil && fi.IsDir() {
		return filepath.Join(p, "kapibara"), nil
	}
	return p, nil
}

// latestTag returns the tag_name of the latest GitHub release.
func latestTag(ctx context.Context) (string, error) {
	body, err := download(ctx, fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", updateRepo))
	if err != nil {
		return "", err
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &rel); err != nil {
		return "", err
	}
	if rel.TagName == "" {
		return "", fmt.Errorf("no tag_name in latest release")
	}
	return rel.TagName, nil
}

// download GETs a URL (with GITHUB_TOKEN auth when set, for private repos).
func download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "kapibara-update")
	req.Header.Set("Accept", "application/octet-stream, application/vnd.github+json")
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d (set GITHUB_TOKEN if the repo is private)", resp.StatusCode)
	}
	return body, nil
}

// checksumFor returns the sha256 hex for name from a checksums.txt body.
func checksumFor(sums, name string) string {
	for _, line := range strings.Split(sums, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[1] == name {
			return f[0]
		}
	}
	return ""
}

// extractBinary pulls the named file out of a gzip-tar archive.
func extractBinary(data []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(hdr.Name) == name && hdr.Typeflag == tar.TypeReg {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("%q not found in archive", name)
}

// replaceBinary atomically installs bin at dest (rename within the same dir),
// falling back to a copy across filesystems.
func replaceBinary(dest string, bin []byte) error {
	dir := filepath.Dir(dest)
	tmp, err := os.CreateTemp(dir, ".kapibara-update-*")
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("cannot write to %s: %w (try sudo, or --path)", dir, err)
		}
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("install to %s: %w (try sudo)", dest, err)
	}
	return nil
}
