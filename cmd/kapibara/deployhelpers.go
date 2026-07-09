package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// parseEnvFile reads a KEY=VALUE-per-line env file (.env style): blank lines and
// lines starting with '#' are ignored, a leading "export " is stripped, and
// surrounding single/double quotes on the value are removed.
func parseEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid line in %s: %q (want KEY=VALUE)", path, line)
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		if k != "" {
			out[k] = v
		}
	}
	return out, sc.Err()
}

// mergeEnv loads --env-file (if any) then overlays --env KEY=VALUE pairs (which
// win), returning the combined map.
func mergeEnv(envFile string, pairs []string) (map[string]string, error) {
	env := map[string]string{}
	if envFile != "" {
		fromFile, err := parseEnvFile(envFile)
		if err != nil {
			return nil, err
		}
		for k, v := range fromFile {
			env[k] = v
		}
	}
	for _, e := range pairs {
		k, v, ok := strings.Cut(e, "=")
		if !ok {
			return nil, fmt.Errorf("invalid --env %q (want KEY=VALUE)", e)
		}
		env[k] = v
	}
	return env, nil
}

// fetchAppsDomain returns the server's base apps domain (e.g. apps.example.com),
// or "" if unset/unavailable.
func fetchAppsDomain(ctx context.Context, client *apiClient) string {
	var conf struct {
		AppsDomain string `json:"appsDomain"`
	}
	_ = client.do(ctx, http.MethodGet, "/api/v1/config", nil, &conf)
	return strings.Trim(strings.TrimSpace(conf.AppsDomain), ".")
}

// autoDomain fills in a default public host when the user didn't set one: it
// derives "<name>.<appsDomain>" and enables TLS. Returns the (possibly
// unchanged) domain and tls. No-op if a domain is already set or the server has
// no apps domain configured.
func autoDomain(ctx context.Context, client *apiClient, name, domain string, tls bool) (string, bool) {
	if domain != "" {
		return domain, tls
	}
	base := fetchAppsDomain(ctx, client)
	if base == "" {
		return domain, tls
	}
	return name + "." + base, true
}
