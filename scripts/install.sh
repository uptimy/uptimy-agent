#!/usr/bin/env bash
# ──────────────────────────────────────────────────────────────────────
#  Uptimy Agent Installer
#
#  One-liner (Linux / macOS):
#    curl -sSfL https://raw.githubusercontent.com/uptimy/uptimy-agent/master/scripts/install.sh | sudo bash
#
#  Environment variables:
#    UPTIMY_VERSION     - version tag to install        (default: latest)
#    UPTIMY_INSTALL     - binary install directory      (default: /usr/local/bin)
#    UPTIMY_CONFIG      - config directory              (default: /etc/uptimy)
#    UPTIMY_DATA        - data directory                (default: /var/lib/uptimy)
#    UPTIMY_USER        - service user                  (default: uptimy)
#    UPTIMY_NO_SERVICE  - skip service setup (set to 1)
#    UPTIMY_NO_VERIFY   - skip checksum verification    (set to 1)
# ──────────────────────────────────────────────────────────────────────
set -euo pipefail

# ── Defaults ─────────────────────────────────────────────────────────
REPO="uptimy/uptimy-agent"
BINARY="uptimy-agent"
VERSION="${UPTIMY_VERSION:-latest}"
INSTALL_DIR="${UPTIMY_INSTALL:-/usr/local/bin}"
CONFIG_DIR="${UPTIMY_CONFIG:-/etc/uptimy}"
DATA_DIR="${UPTIMY_DATA:-/var/lib/uptimy}"
SERVICE_USER="${UPTIMY_USER:-uptimy}"
NO_SERVICE="${UPTIMY_NO_SERVICE:-0}"
NO_VERIFY="${UPTIMY_NO_VERIFY:-0}"

# ── Pretty output ───────────────────────────────────────────────────
info()  { printf "\033[1;34m==>\033[0m %s\n" "$*"; }
warn()  { printf "\033[1;33mWARN:\033[0m %s\n" "$*"; }
error() { printf "\033[1;31mERROR:\033[0m %s\n" "$*" >&2; exit 1; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || error "Required command not found: $1"
}

# ── Platform detection ───────────────────────────────────────────────
detect_os() {
  local os
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$os" in
    linux)  echo "linux"  ;;
    darwin) echo "darwin" ;;
    *)      error "Unsupported OS: $os. Only Linux and macOS are supported." ;;
  esac
}

detect_arch() {
  local arch
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64)  echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *)             error "Unsupported architecture: $arch. Only amd64 and arm64 are supported." ;;
  esac
}

# ── Version resolution ───────────────────────────────────────────────
resolve_version() {
  if [ "$VERSION" = "latest" ]; then
    info "Resolving latest release..."
    need_cmd curl
    local api_response
    api_response="$(curl -sSf "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null)" \
      || error "Could not reach GitHub API. Check your internet connection."
    VERSION="$(echo "$api_response" | grep '"tag_name"' | head -1 | sed -E 's/.*"v?([^"]+)".*/\1/')"
    [ -n "$VERSION" ] || error "Could not determine latest version. Specify UPTIMY_VERSION manually."
    info "Latest version: v${VERSION}"
  fi
  # Strip leading 'v' if present
  VERSION="${VERSION#v}"
}

# ── Checksum verification ────────────────────────────────────────────
verify_checksum() {
  local file="$1" checksums_file="$2"

  if [ "$NO_VERIFY" = "1" ]; then
    warn "Skipping checksum verification (UPTIMY_NO_VERIFY=1)"
    return 0
  fi

  local filename
  filename="$(basename "$file")"

  if [ ! -f "$checksums_file" ]; then
    warn "Checksums file not found - skipping verification"
    return 0
  fi

  local expected
  expected="$(grep "$filename" "$checksums_file" | awk '{print $1}')"
  if [ -z "$expected" ]; then
    warn "No checksum entry for $filename - skipping verification"
    return 0
  fi

  local actual
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$file" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$file" | awk '{print $1}')"
  else
    warn "Neither sha256sum nor shasum found - skipping verification"
    return 0
  fi

  if [ "$actual" != "$expected" ]; then
    error "Checksum mismatch for ${filename}!
  Expected: ${expected}
  Actual:   ${actual}
This may indicate a corrupted or tampered download. Aborting."
  fi

  info "Checksum verified: ${filename}"
}

# ── Default config template ──────────────────────────────────────────
write_default_config() {
  cat > "$1" <<'YAML'
# Uptimy Agent Configuration
# Full reference: https://github.com/uptimy/uptimy-agent
#
# Tip: use ${ENV_VAR} syntax for secrets (e.g. tokens, webhook URLs).

agent:
  name: uptimy-agent
  worker_pool_size: 4

logging:
  level: info
  format: json

telemetry:
  enabled: true
  metrics_port: 9090
  buffer_size: 10000

storage:
  path: /var/lib/uptimy/state.db

kubernetes:
  enabled: "auto"

control_plane:
  enabled: false
  # endpoint: grpc.upti.my:443
  # token: ${UPTIMY_TOKEN}

# ── Health Checks ────────────────────────────────────────────────────
# Define what to monitor. Each check runs on its own interval.
#
# Supported types: http, tcp, cpu, memory, disk, process, certificate
#
# Example:
#   - type: http
#     name: my-api
#     service: my-service
#     url: http://localhost:8080/health
#     interval: 30s
#     timeout: 5s
#     expected_status: 200

checks: []

# ── Repair Recipes ───────────────────────────────────────────────────
# Define multi-step repair workflows.
#
# Available actions:
#   restart_pod, restart_container, restart_service,
#   start_service, stop_service, rollback_deployment,
#   scale_replicas, clear_temp, rotate_logs, webhook,
#   wait, healthcheck
#
# Example:
#   - name: restart-and-verify
#     steps:
#       - action: restart_service
#         params: { service: nginx }
#       - action: wait
#         duration: 10s
#       - action: healthcheck
#         check: my-check

recipes: []

# ── Repair Rules ─────────────────────────────────────────────────────
# Map checks to recipes. When a check fails, its recipe runs.
#
# Example:
#   - rule: api-down
#     check: my-api
#     recipe: restart-and-verify
#     max_repairs_per_hour: 3

repairs: []
YAML
}

# ── macOS launchd plist ──────────────────────────────────────────────
write_launchd_plist() {
  local plist_path="$1"
  cat > "$plist_path" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>my.upti.agent</string>
  <key>ProgramArguments</key>
  <array>
    <string>${INSTALL_DIR}/${BINARY}</string>
    <string>run</string>
    <string>--config</string>
    <string>${CONFIG_DIR}/config.yaml</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>/var/log/uptimy-agent.log</string>
  <key>StandardErrorPath</key>
  <string>/var/log/uptimy-agent.err</string>
</dict>
</plist>
EOF
}

# ══════════════════════════════════════════════════════════════════════
#  MAIN
# ══════════════════════════════════════════════════════════════════════

need_cmd uname
need_cmd curl
need_cmd tar

OS="$(detect_os)"
ARCH="$(detect_arch)"

echo ""
info "Uptimy Agent Installer"
info "Platform: ${OS}/${ARCH}"
echo ""

resolve_version

# ── Download & verify ────────────────────────────────────────────────
TARBALL="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
DOWNLOAD_BASE="https://github.com/${REPO}/releases/download/v${VERSION}"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

info "Downloading ${TARBALL}..."
curl -sSfL -o "${TMPDIR}/${TARBALL}" "${DOWNLOAD_BASE}/${TARBALL}" \
  || error "Download failed. Check the version (v${VERSION}) and that a release exists for ${OS}/${ARCH}.
  URL: ${DOWNLOAD_BASE}/${TARBALL}"

# Download checksums and verify
curl -sSfL -o "${TMPDIR}/checksums.txt" "${DOWNLOAD_BASE}/checksums.txt" 2>/dev/null || true
verify_checksum "${TMPDIR}/${TARBALL}" "${TMPDIR}/checksums.txt"

info "Extracting..."
tar -xzf "${TMPDIR}/${TARBALL}" -C "$TMPDIR"

# ── Install binary ───────────────────────────────────────────────────
info "Installing binary → ${INSTALL_DIR}/${BINARY}"
install -d "$INSTALL_DIR"
install -m 0755 "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"

# Quick sanity check
if "${INSTALL_DIR}/${BINARY}" version >/dev/null 2>&1; then
  INSTALLED_VER="$("${INSTALL_DIR}/${BINARY}" version 2>&1 | head -1)"
  info "Installed: ${INSTALLED_VER}"
else
  warn "Binary installed but could not run 'version' command"
fi

# ── Config ───────────────────────────────────────────────────────────
install -d -m 0755 "$CONFIG_DIR"

if [ ! -f "${CONFIG_DIR}/config.yaml" ]; then
  info "Creating config → ${CONFIG_DIR}/config.yaml"
  if [ -f "${TMPDIR}/config.yaml" ]; then
    install -m 0644 "${TMPDIR}/config.yaml" "${CONFIG_DIR}/config.yaml"
  else
    write_default_config "${CONFIG_DIR}/config.yaml"
  fi
else
  info "Config already exists - keeping ${CONFIG_DIR}/config.yaml"
fi

# ── Data directory ───────────────────────────────────────────────────
install -d -m 0755 "$DATA_DIR"

# ── Linux: systemd service ───────────────────────────────────────────
if [ "$OS" = "linux" ] && [ "$NO_SERVICE" != "1" ]; then
  # Create service user
  if ! id "$SERVICE_USER" >/dev/null 2>&1; then
    info "Creating service user: ${SERVICE_USER}"
    useradd --system --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER" 2>/dev/null \
      || warn "Could not create user ${SERVICE_USER} - you may need to create it manually"
  fi
  chown -R "${SERVICE_USER}:${SERVICE_USER}" "$DATA_DIR" 2>/dev/null || true
  chown -R "${SERVICE_USER}:${SERVICE_USER}" "$CONFIG_DIR" 2>/dev/null || true

  if command -v systemctl >/dev/null 2>&1; then
    UNIT_FILE="/etc/systemd/system/uptimy-agent.service"
    if [ ! -f "$UNIT_FILE" ]; then
      info "Installing systemd service → ${UNIT_FILE}"
      if [ -f "${TMPDIR}/uptimy-agent.service" ]; then
        install -m 0644 "${TMPDIR}/uptimy-agent.service" "$UNIT_FILE"
      else
        cat > "$UNIT_FILE" <<EOF
[Unit]
Description=Uptimy Agent - Self-Healing Infrastructure Watchdog
Documentation=https://github.com/uptimy/uptimy-agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_USER}
ExecStart=${INSTALL_DIR}/${BINARY} run --config ${CONFIG_DIR}/config.yaml
Restart=on-failure
RestartSec=5
LimitNOFILE=65536
EnvironmentFile=-/etc/uptimy/env
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true
ReadWritePaths=${DATA_DIR}

[Install]
WantedBy=multi-user.target
EOF
      fi
      systemctl daemon-reload
    else
      info "systemd unit already exists - skipping"
    fi
  fi
fi

# ── macOS: launchd service ───────────────────────────────────────────
if [ "$OS" = "darwin" ] && [ "$NO_SERVICE" != "1" ]; then
  PLIST_PATH="/Library/LaunchDaemons/my.upti.agent.plist"
  if [ ! -f "$PLIST_PATH" ]; then
    info "Installing launchd service → ${PLIST_PATH}"
    write_launchd_plist "$PLIST_PATH"
    chmod 644 "$PLIST_PATH"
    chown root:wheel "$PLIST_PATH"
  else
    info "launchd plist already exists - skipping"
  fi
fi

# ── Summary ──────────────────────────────────────────────────────────
echo ""
info "────────────────────────────────────────────"
info "  Uptimy Agent v${VERSION} installed!"
info "────────────────────────────────────────────"
echo ""
info "  Binary:  ${INSTALL_DIR}/${BINARY}"
info "  Config:  ${CONFIG_DIR}/config.yaml"
info "  Data:    ${DATA_DIR}/"
echo ""
info "Next steps:"
info "  1. Edit your config:  sudo vi ${CONFIG_DIR}/config.yaml"

if [ "$OS" = "linux" ] && [ "$NO_SERVICE" != "1" ] && command -v systemctl >/dev/null 2>&1; then
  info "  2. Start the agent:  sudo systemctl enable --now uptimy-agent"
  info "  3. View logs:        sudo journalctl -fu uptimy-agent"
elif [ "$OS" = "darwin" ] && [ "$NO_SERVICE" != "1" ]; then
  info "  2. Start the agent:  sudo launchctl load ${PLIST_PATH}"
  info "  3. View logs:        tail -f /var/log/uptimy-agent.log"
else
  info "  2. Run the agent:    ${INSTALL_DIR}/${BINARY} run --config ${CONFIG_DIR}/config.yaml"
fi

echo ""
info "Examples: https://github.com/uptimy/uptimy-agent/tree/master/examples"
echo ""
