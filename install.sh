#!/usr/bin/env sh
# Kapibara installer — downloads the latest release binary for your OS/arch.
#
#   curl -fsSL https://raw.githubusercontent.com/orcinustools/kapibara/main/install.sh | sh
#
# Environment overrides:
#   KAPIBARA_VERSION   release tag to install   (default: latest)
#   KAPIBARA_INSTALL   install directory        (default: /usr/local/bin)
set -eu

REPO="orcinustools/kapibara"
BIN="kapibara"
INSTALL_DIR="${KAPIBARA_INSTALL:-/usr/local/bin}"
VERSION="${KAPIBARA_VERSION:-latest}"

say() { printf 'kapibara-install: %s\n' "$1" >&2; }
die() { say "$1"; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "required tool not found: $1"; }

need uname
need tar
if command -v curl >/dev/null 2>&1; then
  dl() { curl -fsSL "$1" -o "$2"; }
  fetch() { curl -fsSL "$1"; }
elif command -v wget >/dev/null 2>&1; then
  dl() { wget -qO "$2" "$1"; }
  fetch() { wget -qO - "$1"; }
else
  die "need curl or wget"
fi

# Detect OS.
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  linux) os="linux" ;;
  darwin) os="darwin" ;;
  *) die "unsupported OS: $os (linux and darwin only)" ;;
esac

# Detect arch.
arch="$(uname -m)"
case "$arch" in
  x86_64 | amd64) arch="amd64" ;;
  aarch64 | arm64) arch="arm64" ;;
  *) die "unsupported architecture: $arch" ;;
esac

# Resolve the version tag.
if [ "$VERSION" = "latest" ]; then
  VERSION="$(fetch "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | head -1 | cut -d '"' -f 4)"
  [ -n "$VERSION" ] || die "could not resolve the latest release tag (set KAPIBARA_VERSION)"
fi

# goreleaser strips the leading 'v' from the version in archive names.
ver_nov="${VERSION#v}"
archive="${BIN}_${ver_nov}_${os}_${arch}.tar.gz"
base="https://github.com/${REPO}/releases/download/${VERSION}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

say "downloading ${archive} (${VERSION})"
dl "${base}/${archive}" "${tmp}/${archive}" || die "download failed: ${base}/${archive}"

# Verify the checksum when available.
if dl "${base}/checksums.txt" "${tmp}/checksums.txt" 2>/dev/null; then
  if command -v sha256sum >/dev/null 2>&1; then sum="sha256sum"; else sum="shasum -a 256"; fi
  want="$(grep " ${archive}\$" "${tmp}/checksums.txt" | awk '{print $1}')"
  if [ -n "$want" ]; then
    got="$(cd "$tmp" && $sum "$archive" | awk '{print $1}')"
    [ "$want" = "$got" ] || die "checksum mismatch for ${archive}"
    say "checksum ok"
  fi
fi

tar -xzf "${tmp}/${archive}" -C "$tmp"
[ -f "${tmp}/${BIN}" ] || die "binary ${BIN} not found in archive"
chmod +x "${tmp}/${BIN}"

# Install (elevate only if the target dir is not writable).
if [ -w "$INSTALL_DIR" ] || [ "$(id -u)" = "0" ]; then
  install -m 0755 "${tmp}/${BIN}" "${INSTALL_DIR}/${BIN}"
elif command -v sudo >/dev/null 2>&1; then
  say "installing to ${INSTALL_DIR} (sudo)"
  sudo install -m 0755 "${tmp}/${BIN}" "${INSTALL_DIR}/${BIN}"
else
  die "cannot write to ${INSTALL_DIR}; set KAPIBARA_INSTALL to a writable dir"
fi

say "installed ${BIN} ${VERSION} → ${INSTALL_DIR}/${BIN}"
"${INSTALL_DIR}/${BIN}" version 2>/dev/null || true
