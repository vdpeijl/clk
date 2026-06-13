#!/bin/sh
# clk installer — downloads the latest release binary for this machine.
#
#   curl -sSfL https://raw.githubusercontent.com/vdpeijl/clk/main/install.sh | sh
#
# Environment overrides:
#   CLK_INSTALL_DIR   target directory (default: /usr/local/bin, or
#                     ~/.local/bin when /usr/local/bin is not writable)
#   CLK_VERSION       release tag to install (default: latest)
set -eu

REPO="vdpeijl/clk"
BINARY="clk"

err() {
	echo "install.sh: $*" >&2
	exit 1
}

# Pick a downloader.
if command -v curl >/dev/null 2>&1; then
	download() { curl -sSfL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
	download() { wget -qO "$2" "$1"; }
else
	err "need curl or wget to download releases"
fi

# Detect OS.
os=$(uname -s)
case "$os" in
	Linux) os="linux" ;;
	Darwin) os="darwin" ;;
	*) err "unsupported OS: $os (use 'brew install vdpeijl/tap/clk' on macOS, or build from source)" ;;
esac

# Detect architecture and map to the GoReleaser naming (amd64/arm64).
arch=$(uname -m)
case "$arch" in
	x86_64 | amd64) arch="amd64" ;;
	aarch64 | arm64) arch="arm64" ;;
	*) err "unsupported architecture: $arch" ;;
esac

asset="${BINARY}_${os}_${arch}.tar.gz"

# Resolve the download URL. A pinned CLK_VERSION uses the tagged path; otherwise
# we rely on GitHub's /releases/latest/download redirect.
version="${CLK_VERSION:-latest}"
if [ "$version" = "latest" ]; then
	url="https://github.com/${REPO}/releases/latest/download/${asset}"
else
	url="https://github.com/${REPO}/releases/download/${version}/${asset}"
fi

# Choose an install directory: prefer a system path, fall back to the user's.
if [ -n "${CLK_INSTALL_DIR:-}" ]; then
	install_dir="$CLK_INSTALL_DIR"
elif [ -w /usr/local/bin ] 2>/dev/null; then
	install_dir="/usr/local/bin"
else
	install_dir="$HOME/.local/bin"
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading ${BINARY} (${os}/${arch}) from ${url}"
download "$url" "$tmp/$asset" || err "download failed: $url"

tar -xzf "$tmp/$asset" -C "$tmp" || err "failed to extract $asset"
[ -f "$tmp/$BINARY" ] || err "archive did not contain a '$BINARY' binary"

mkdir -p "$install_dir"
# Try to install in place; escalate with sudo for system directories.
if mv "$tmp/$BINARY" "$install_dir/$BINARY" 2>/dev/null; then
	:
elif command -v sudo >/dev/null 2>&1; then
	echo "Elevating with sudo to write to $install_dir"
	sudo mv "$tmp/$BINARY" "$install_dir/$BINARY"
else
	err "cannot write to $install_dir (set CLK_INSTALL_DIR to a writable path)"
fi
chmod +x "$install_dir/$BINARY" 2>/dev/null || sudo chmod +x "$install_dir/$BINARY"

echo "Installed $BINARY to $install_dir/$BINARY"
case ":$PATH:" in
	*":$install_dir:"*) ;;
	*) echo "Note: $install_dir is not on your PATH — add it to use '$BINARY' directly." ;;
esac
"$install_dir/$BINARY" version || true
