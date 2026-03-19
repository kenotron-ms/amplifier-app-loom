#!/usr/bin/env bash
# agent-daemon installer
# One-liner: curl -fsSL https://raw.githubusercontent.com/kenotron-ms/agent-daemon/main/.amplifier/scripts/install.sh | bash

set -euo pipefail

REPO="kenotron-ms/agent-daemon"
BINARY="agent-daemon"
INSTALL_DIR="/usr/local/bin"

# ── Helpers ────────────────────────────────────────────────────────────────────

info()    { echo "  $*"; }
success() { echo "✓ $*"; }
warn()    { echo "⚠ $*"; }
die()     { echo "✗ $*" >&2; exit 1; }

# ── Platform detection ─────────────────────────────────────────────────────────

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) die "Unsupported architecture: $ARCH" ;;
esac
case "$OS" in
  darwin|linux) ;;
  *) die "Unsupported OS: $OS (Windows: download from https://github.com/$REPO/releases)" ;;
esac

ASSET="${BINARY}-${OS}-${ARCH}"

# ── Already installed? ─────────────────────────────────────────────────────────

if command -v "$BINARY" &>/dev/null; then
  CURRENT="$(agent-daemon version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "unknown")"
  info "agent-daemon $CURRENT already installed at $(command -v $BINARY)"
  info "Run 'agent-daemon update' to upgrade to the latest release."
  SKIP_DOWNLOAD=true
else
  SKIP_DOWNLOAD=false
fi

# ── Download ───────────────────────────────────────────────────────────────────

if [ "$SKIP_DOWNLOAD" = false ]; then
  echo ""
  echo "Installing agent-daemon..."
  echo ""

  # Fetch latest release metadata
  API_URL="https://api.github.com/repos/${REPO}/releases/latest"
  RELEASE_JSON="$(curl -fsSL "$API_URL")"
  VERSION="$(echo "$RELEASE_JSON" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\(.*\)".*/\1/')"
  DOWNLOAD_URL="$(echo "$RELEASE_JSON" | grep "browser_download_url" | grep "$ASSET\"" | sed 's/.*"browser_download_url": *"\(.*\)".*/\1/')"

  if [ -z "$DOWNLOAD_URL" ]; then
    die "No release binary found for ${OS}/${ARCH}. Check https://github.com/${REPO}/releases"
  fi

  info "Version:  $VERSION"
  info "Platform: ${OS}/${ARCH}"
  info "Download: $DOWNLOAD_URL"
  echo ""

  TMP="$(mktemp)"
  curl -fsSL --progress-bar "$DOWNLOAD_URL" -o "$TMP"
  chmod +x "$TMP"

  # Install to /usr/local/bin — try without sudo first, fall back with sudo
  if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP" "${INSTALL_DIR}/${BINARY}"
  else
    info "Installing to $INSTALL_DIR (requires sudo)..."
    sudo mv "$TMP" "${INSTALL_DIR}/${BINARY}"
  fi

  success "Binary installed to ${INSTALL_DIR}/${BINARY}"
fi

# ── Register + start background service ───────────────────────────────────────

echo ""
echo "Configuring background service..."
echo ""

# Install as user-level launchd agent (macOS) / systemd user service (Linux)
if "$BINARY" status &>/dev/null 2>&1; then
  info "Daemon already running — skipping service install."
else
  "$BINARY" install
  "$BINARY" start
  success "Daemon installed and started (auto-starts on login)"
fi

# ── macOS: tray app ────────────────────────────────────────────────────────────

if [ "$OS" = "darwin" ]; then
  echo ""
  echo "Launching menu bar app..."
  echo ""

  # Launch tray as a detached background process so the installer exits cleanly.
  # The tray app shows daemon status in the macOS menu bar.
  nohup "$BINARY" tray </dev/null >/dev/null 2>&1 &
  disown

  # Add to login items so the tray auto-starts on next login
  osascript -e "tell application \"System Events\" to make new login item at end with properties {path:\"$(command -v $BINARY)\", hidden:false, name:\"agent-daemon tray\"}" 2>/dev/null \
    && success "Tray app added to Login Items (auto-starts on login)" \
    || warn "Could not add tray to Login Items automatically — run 'agent-daemon tray' to start it manually."

  success "Tray app launched"
fi

# ── Amplifier bundle ───────────────────────────────────────────────────────────

if command -v amplifier &>/dev/null; then
  echo ""
  echo "Registering Amplifier app bundle..."
  echo ""
  amplifier bundle add "git+https://github.com/kenotron-ms/agent-daemon@main" --app \
    && success "Amplifier app bundle registered (active in every session)" \
    || warn "Could not register Amplifier bundle — run manually: amplifier bundle add git+https://github.com/kenotron-ms/agent-daemon@main --app"
else
  warn "Amplifier not found — skipping bundle registration."
  warn "Once installed, run: amplifier bundle add git+https://github.com/kenotron-ms/agent-daemon@main --app"
fi

# ── Done ───────────────────────────────────────────────────────────────────────

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
success "agent-daemon is installed and running!"
echo ""
echo "  Status:  agent-daemon status"
echo "  Web UI:  http://localhost:7700"
if [ "$OS" = "darwin" ]; then
echo "  Tray:    Look for the icon in your menu bar"
fi
echo ""
