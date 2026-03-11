#!/usr/bin/env bash
# ──────────────────────────────────────────────────────────────────────
#  Uptimy Agent Uninstaller
#
#  Usage:
#    sudo bash uninstall.sh
#
#  Environment variables:
#    UPTIMY_INSTALL    — binary directory   (default: /usr/local/bin)
#    UPTIMY_CONFIG     — config directory   (default: /etc/uptimy)
#    UPTIMY_DATA       — data directory     (default: /var/lib/uptimy)
#    UPTIMY_USER       — service user       (default: uptimy)
#    UPTIMY_KEEP_DATA  — keep data dir      (set to 1 to preserve state)
# ──────────────────────────────────────────────────────────────────────
set -euo pipefail

BINARY="uptimy-agent"
INSTALL_DIR="${UPTIMY_INSTALL:-/usr/local/bin}"
CONFIG_DIR="${UPTIMY_CONFIG:-/etc/uptimy}"
DATA_DIR="${UPTIMY_DATA:-/var/lib/uptimy}"
SERVICE_USER="${UPTIMY_USER:-uptimy}"
KEEP_DATA="${UPTIMY_KEEP_DATA:-0}"

info()  { printf "\033[1;34m==>\033[0m %s\n" "$*"; }
warn()  { printf "\033[1;33mWARN:\033[0m %s\n" "$*"; }

# ── Stop and remove systemd service (Linux) ─────────────────────────
if command -v systemctl >/dev/null 2>&1; then
  if systemctl is-active --quiet uptimy-agent 2>/dev/null; then
    info "Stopping uptimy-agent service"
    systemctl stop uptimy-agent
  fi
  if systemctl is-enabled --quiet uptimy-agent 2>/dev/null; then
    info "Disabling uptimy-agent service"
    systemctl disable uptimy-agent
  fi
  if [ -f /etc/systemd/system/uptimy-agent.service ]; then
    info "Removing systemd unit file"
    rm -f /etc/systemd/system/uptimy-agent.service
    systemctl daemon-reload
  fi
fi

# ── Stop and remove launchd service (macOS) ──────────────────────────
PLIST_PATH="/Library/LaunchDaemons/my.upti.agent.plist"
if [ -f "$PLIST_PATH" ]; then
  info "Unloading launchd service"
  launchctl unload "$PLIST_PATH" 2>/dev/null || true
  info "Removing launchd plist"
  rm -f "$PLIST_PATH"
fi

# ── Remove binary ───────────────────────────────────────────────────
if [ -f "${INSTALL_DIR}/${BINARY}" ]; then
  info "Removing binary: ${INSTALL_DIR}/${BINARY}"
  rm -f "${INSTALL_DIR}/${BINARY}"
fi

# ── Remove config ───────────────────────────────────────────────────
if [ -d "$CONFIG_DIR" ]; then
  info "Removing config directory: ${CONFIG_DIR}"
  rm -rf "$CONFIG_DIR"
fi

# ── Remove data ─────────────────────────────────────────────────────
if [ "$KEEP_DATA" = "1" ]; then
  warn "Keeping data directory: ${DATA_DIR}"
else
  if [ -d "$DATA_DIR" ]; then
    info "Removing data directory: ${DATA_DIR}"
    rm -rf "$DATA_DIR"
  fi
fi

# ── Remove service user (Linux only) ────────────────────────────────
if [ "$(uname -s)" = "Linux" ] && id "$SERVICE_USER" >/dev/null 2>&1; then
  info "Removing service user: ${SERVICE_USER}"
  userdel "$SERVICE_USER" 2>/dev/null || warn "Could not remove user ${SERVICE_USER}"
fi

# ── Remove log files (macOS) ────────────────────────────────────────
if [ "$(uname -s)" = "Darwin" ]; then
  for f in /var/log/uptimy-agent.log /var/log/uptimy-agent.err; do
    [ -f "$f" ] && rm -f "$f" && info "Removed $f"
  done
fi

echo ""
info "Uptimy Agent has been uninstalled."
if [ "$KEEP_DATA" = "1" ]; then
  info "State data preserved at: ${DATA_DIR}"
fi
echo ""
