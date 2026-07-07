// Package backup dumps managed databases (via in-pod dump tools) and stores the
// artifact locally or in an S3-compatible bucket.
package backup

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/orcinustools/kapibara/pkg/kube"
	"github.com/orcinustools/kapibara/pkg/store"
)

// Runner performs database backups.
type Runner struct {
	Kube      *kube.Client
	Namespace string
	DataDir   string
}

// dumpCommand returns the in-pod command and file extension for an engine.
func dumpCommand(db *store.Database) ([]string, string, error) {
	switch db.Engine {
	case "postgres":
		return []string{"sh", "-c",
			fmt.Sprintf("PGPASSWORD='%s' pg_dump -U '%s' '%s'", db.Password, db.Username, db.DBName)}, "sql", nil
	case "mysql", "mariadb":
		return []string{"sh", "-c",
			fmt.Sprintf("mysqldump -u root -p'%s' '%s'", db.Password, db.DBName)}, "sql", nil
	case "mongo":
		return []string{"sh", "-c",
			fmt.Sprintf("mongodump --username '%s' --password '%s' --archive", db.Username, db.Password)}, "archive", nil
	case "redis":
		return []string{"sh", "-c", "redis-cli --rdb /dev/stdout"}, "rdb", nil
	default:
		return nil, "", fmt.Errorf("backup unsupported for engine %q", db.Engine)
	}
}

// Run performs a backup and returns the artifact path (local) or object key.
func (r *Runner) Run(ctx context.Context, db *store.Database, s3cfg map[string]string, destination string) (string, error) {
	if r.Kube == nil {
		return "", fmt.Errorf("cluster access unavailable")
	}
	cmd, ext, err := dumpCommand(db)
	if err != nil {
		return "", err
	}
	// StatefulSet pods are named <service>-0.
	pod := db.Host + "-0"

	var out, errBuf bytes.Buffer
	if err := r.Kube.Exec(ctx, r.Namespace, pod, cmd, &out, &errBuf); err != nil {
		return "", fmt.Errorf("dump exec: %w (%s)", err, truncate(errBuf.String()))
	}
	if out.Len() == 0 {
		return "", fmt.Errorf("empty dump (%s)", truncate(errBuf.String()))
	}

	name := fmt.Sprintf("%s-%s.%s", db.Name, time.Now().UTC().Format("20060102-150405"), ext)

	switch destination {
	case "s3":
		key, err := uploadS3(ctx, s3cfg, name, out.Bytes())
		if err != nil {
			return "", err
		}
		return key, nil
	default: // local
		dir := filepath.Join(r.DataDir, "backups")
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return "", err
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, out.Bytes(), 0o640); err != nil {
			return "", err
		}
		return path, nil
	}
}

func truncate(s string) string {
	if len(s) > 300 {
		return s[:300]
	}
	return s
}
