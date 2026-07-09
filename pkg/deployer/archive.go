package deployer

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// extractTarGz extracts a gzip-compressed tar archive into destDir and returns a
// short content hash of the archive (used as the image tag for uploaded builds).
// It guards against path traversal ("zip slip") and skips non-regular, non-dir,
// non-symlink entries.
func extractTarGz(archivePath, destDir string) (string, error) {
	// Content hash of the archive → deterministic image tag.
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		f.Close()
		return "", err
	}
	f.Close()
	tag := hex.EncodeToString(h.Sum(nil))[:12]

	f, err = os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	dest, err := filepath.Abs(destDir)
	if err != nil {
		return "", err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		// Reject absolute paths and traversal outside destDir.
		clean := filepath.Clean("/" + hdr.Name)
		target := filepath.Join(dest, clean)
		if target != dest && !strings.HasPrefix(target, dest+string(os.PathSeparator)) {
			return "", fmt.Errorf("unsafe path in archive: %q", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return "", err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return "", err
			}
			// Cap per-file copy to avoid a decompression bomb hanging the build.
			if _, err := io.CopyN(out, tr, hdr.Size); err != nil && err != io.EOF {
				out.Close()
				return "", err
			}
			out.Close()
		case tar.TypeSymlink:
			// Only allow relative symlinks that stay inside the tree.
			if filepath.IsAbs(hdr.Linkname) {
				continue
			}
			_ = os.MkdirAll(filepath.Dir(target), 0o755)
			_ = os.Symlink(hdr.Linkname, target)
		default:
			// skip other types (devices, fifos, …)
		}
	}
	return tag, nil
}
