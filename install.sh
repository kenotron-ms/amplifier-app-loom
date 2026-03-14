#!/usr/bin/env sh
# install.sh — download and install agent-daemon to /usr/local/bin
# Usage: curl -fsSL https://raw.githubusercontent.com/kenotron-ms/agent-daemon/main/install.sh | sh
set -e

REPO="kenotron-ms/agent-daemon"
BIN="agent-daemon"
INSTALL_DIR="/usr/local/bin"

# ── Detect OS and arch ────────────────────────────────────────────────────────
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$OS" in
  darwin) OS="darwin" ;;
  linux)  OS="linux"  ;;
  *)
    echo "Unsupported OS: $OS"
    echo "For Windows, see: https://github.com/$REPO#windows"
    exit 1
    ;;
esac

case "$ARCH" in
  x86_64 | amd64) ARCH="amd64" ;;
  arm64 | aarch64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

ASSET="${BIN}-${OS}-${ARCH}"

# ── Resolve latest release ────────────────────────────────────────────────────
if command -v curl >/dev/null 2>&1; then
  DOWNLOAD_URL=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep "browser_download_url" \
    | grep "$ASSET\"" \
    | head -1 \
    | sed 's/.*"browser_download_url": "\(.*\)".*/\1/')
elif command -v wget >/dev/null 2>&1; then
  DOWNLOAD_URL=$(wget -qO- "https://api.github.com/repos/$REPO/releases/latest" \
    | grep "browser_download_url" \
    | grep "$ASSET\"" \
    | head -1 \
    | sed 's/.*"browser_download_url": "\(.*\)".*/\1/')
else
  echo "Error: curl or wget is required"
  exit 1
fi

if [ -z "$DOWNLOAD_URL" ]; then
  echo "Could not find a release asset for $ASSET"
  echo "Check https://github.com/$REPO/releases for available downloads."
  exit 1
fi

# ── Download ──────────────────────────────────────────────────────────────────
TMPFILE=$(mktemp)
trap 'rm -f "$TMPFILE"' EXIT

echo "Downloading $ASSET..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$DOWNLOAD_URL" -o "$TMPFILE"
else
  wget -qO "$TMPFILE" "$DOWNLOAD_URL"
fi

chmod +x "$TMPFILE"

# ── Install ───────────────────────────────────────────────────────────────────
if [ -w "$INSTALL_DIR" ]; then
  mv "$TMPFILE" "$INSTALL_DIR/$BIN"
else
  echo "Installing to $INSTALL_DIR (sudo required)..."
  sudo mv "$TMPFILE" "$INSTALL_DIR/$BIN"
fi

# ── PATH check ────────────────────────────────────────────────────────────────
echo ""
echo "✓ Installed: $INSTALL_DIR/$BIN"
echo "  Version: $($INSTALL_DIR/$BIN --version 2>/dev/null || echo 'unknown')"
echo ""

if ! echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
  echo "Note: $INSTALL_DIR is not in your PATH."
  echo "Add this to your shell profile (~/.zshrc, ~/.bashrc, etc.):"
  echo ""
  echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
  echo ""
fi

echo "Next steps:"
echo "  agent-daemon install   # register as a user-level service"
echo "  agent-daemon start     # start the daemon"
echo "  open http://localhost:7700"
