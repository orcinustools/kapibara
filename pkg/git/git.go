// Package git provides shallow cloning of source repositories for builds.
package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// CloneOptions describes a repository to fetch.
type CloneOptions struct {
	RepoURL string // https or ssh URL
	Ref     string // branch, tag, or commit (default: repo default branch)
	Dir     string // destination directory (must not exist / be empty)
	// Token, if set, is injected into an https URL for private repos.
	Token string
}

// Clone performs a shallow clone into opts.Dir and returns the resolved commit
// SHA. Build output/errors are returned combined for surfacing to the user.
func Clone(ctx context.Context, opts CloneOptions) (sha string, out string, err error) {
	url := opts.RepoURL
	if opts.Token != "" && strings.HasPrefix(url, "https://") {
		url = "https://x-access-token:" + opts.Token + "@" + strings.TrimPrefix(url, "https://")
	}

	args := []string{"clone", "--depth", "1"}
	if opts.Ref != "" {
		args = append(args, "--branch", opts.Ref)
	}
	args = append(args, url, opts.Dir)

	cmd := exec.CommandContext(ctx, "git", args...)
	b, err := cmd.CombinedOutput()
	out = redact(string(b), opts.Token)
	if err != nil {
		return "", out, fmt.Errorf("git clone: %w", err)
	}

	sha, shaOut, err := headSHA(ctx, opts.Dir)
	out += shaOut
	return sha, out, err
}

func headSHA(ctx context.Context, dir string) (string, string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD")
	b, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(b))
	if err != nil {
		return "", s, fmt.Errorf("git rev-parse: %w", err)
	}
	return s, "", nil
}

func redact(s, token string) string {
	if token == "" {
		return s
	}
	return strings.ReplaceAll(s, token, "***")
}
