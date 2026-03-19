#!/usr/bin/env bash
# agent-daemon installer
# One-liner: curl -fsSL https://raw.githubusercontent.com/kenotron-ms/agent-daemon/main/.amplifier/scripts/install.sh | bash

set -euo pipefail

REPO="kenotron-ms/agent-daemon"
BINARY="agent-daemon"
APP_NAME="AgentDaemon"
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
  x86_64)        ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) die "Unsupported architecture: $ARCH" ;;
esac
case "$OS" in
  darwin|linux) ;;
  *) die "Unsupported OS: $OS (Windows: download from https://github.com/$REPO/releases)" ;;
esac

# ── Fetch latest release metadata ─────────────────────────────────────────────

API_URL="https://api.github.com/repos/${REPO}/releases/latest"
RELEASE_JSON="$(curl -fsSL "$API_URL")"
VERSION="$(echo "$RELEASE_JSON" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\(.*\)".*/\1/')"

# ── Already installed? ─────────────────────────────────────────────────────────

SKIP_DOWNLOAD=false
if [ "$OS" = "darwin" ] && [ -d "/Applications/${APP_NAME}.app" ]; then
  CURRENT="$("$INSTALL_DIR/$BINARY" version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "unknown")"
  info "${APP_NAME}.app $CURRENT already installed in /Applications"
  info "Run 'agent-daemon update' to upgrade to $VERSION."
  SKIP_DOWNLOAD=true
elif [ "$OS" = "linux" ] && command -v "$BINARY" &>/dev/null; then
  CURRENT="$(agent-daemon version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "unknown")"
  info "agent-daemon $CURRENT already installed at $(command -v $BINARY)"
  info "Run 'agent-daemon update' to upgrade to $VERSION."
  SKIP_DOWNLOAD=true
fi

# ── Download & install ─────────────────────────────────────────────────────────

if [ "$SKIP_DOWNLOAD" = false ]; then
  echo ""
  echo "Installing agent-daemon $VERSION..."
  echo ""
  info "Platform: ${OS}/${ARCH}"

  if [ "$OS" = "darwin" ]; then
    # ── macOS: download DMG, install .app, symlink binary ──────────────────────

    ASSET="${BINARY}-${OS}-${ARCH}.dmg"
    DOWNLOAD_URL="$(echo "$RELEASE_JSON" | grep "browser_download_url" | grep "\"${ASSET}\"" | sed 's/.*"browser_download_url": *"\(.*\)".*/\1/')"
    [ -z "$DOWNLOAD_URL" ] && die "No DMG found for ${OS}/${ARCH} in release $VERSION. Check https://github.com/${REPO}/releases"

    info "Downloading $ASSET..."
    TMP_DMG="$(mktemp).dmg"
    curl -fsSL --progress-bar "$DOWNLOAD_URL" -o "$TMP_DMG"

    info "Mounting DMG..."
    MOUNT_POINT="$(mktemp -d)"
    hdiutil attach "$TMP_DMG" -mountpoint "$MOUNT_POINT" -nobrowse -quiet

    info "Installing ${APP_NAME}.app to /Applications..."
    if [ -d "/Applications/${APP_NAME}.app" ]; then
      rm -rf "/Applications/${APP_NAME}.app"
    fi
    cp -R "$MOUNT_POINT/${APP_NAME}.app" /Applications/

    hdiutil detach "$MOUNT_POINT" -quiet
    rm -f "$TMP_DMG"
    success "${APP_NAME}.app installed to /Applications"

    # Symlink binary so it's available in PATH
    APP_BINARY="/Applications/${APP_NAME}.app/Contents/MacOS/${BINARY}"
    if [ -w "$INSTALL_DIR" ]; then
      ln -sf "$APP_BINARY" "${INSTALL_DIR}/${BINARY}"
    else
      sudo ln -sf "$APP_BINARY" "${INSTALL_DIR}/${BINARY}"
    fi
    success "Symlinked agent-daemon to $INSTALL_DIR"

  else
    # ── Linux: download raw binary ──────────────────────────────────────────────

    ASSET="${BINARY}-${OS}-${ARCH}"
    DOWNLOAD_URL="$(echo "$RELEASE_JSON" | grep "browser_download_url" | grep "\"${ASSET}\"" | sed 's/.*"browser_download_url": *"\(.*\)".*/\1/')"
    [ -z "$DOWNLOAD_URL" ] && die "No binary found for ${OS}/${ARCH} in release $VERSION. Check https://github.com/${REPO}/releases"

    info "Downloading $ASSET..."
    TMP="$(mktemp)"
    curl -fsSL --progress-bar "$DOWNLOAD_URL" -o "$TMP"
    chmod +x "$TMP"

    if [ -w "$INSTALL_DIR" ]; then
      mv "$TMP" "${INSTALL_DIR}/${BINARY}"
    else
      info "Installing to $INSTALL_DIR (requires sudo)..."
      sudo mv "$TMP" "${INSTALL_DIR}/${BINARY}"
    fi
    success "Binary installed to ${INSTALL_DIR}/${BINARY}"
  fi
fi

# ── macOS: ensure binary symlink exists in PATH ────────────────────────────────

if [ "$OS" = "darwin" ]; then
  APP_BINARY="/Applications/${APP_NAME}.app/Contents/MacOS/${BINARY}"
  if [ ! -f "${INSTALL_DIR}/${BINARY}" ] || [ "$(readlink "${INSTALL_DIR}/${BINARY}" 2>/dev/null)" != "$APP_BINARY" ]; then
    if [ -w "$INSTALL_DIR" ]; then
      ln -sf "$APP_BINARY" "${INSTALL_DIR}/${BINARY}"
    else
      sudo ln -sf "$APP_BINARY" "${INSTALL_DIR}/${BINARY}"
    fi
    success "Symlinked agent-daemon to $INSTALL_DIR"
  fi
fi

# ── Register + start background service ───────────────────────────────────────

echo ""
echo "Configuring background service..."
echo ""

if "$BINARY" status &>/dev/null 2>&1; then
  info "Daemon already running — skipping service install."
else
  "$BINARY" install
  "$BINARY" start
  success "Daemon installed and started (auto-starts on login)"
fi

# ── macOS: open the app (tray) ─────────────────────────────────────────────────

if [ "$OS" = "darwin" ]; then
  echo ""
  echo "Launching menu bar app..."
  echo ""
  open "/Applications/${APP_NAME}.app"
  success "AgentDaemon launched — look for the icon in your menu bar"
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
success "agent-daemon $VERSION is installed and running!"
echo ""
echo "  Status:  agent-daemon status"
echo "  Web UI:  http://localhost:7700"
if [ "$OS" = "darwin" ]; then
echo "  Tray:    Look for the AgentDaemon icon in your menu bar"
fi
echo ""
