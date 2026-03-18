#!/usr/bin/env bash
# ============================================================================
# SkyTracker Device — One-Line Installer
# ============================================================================
# Installs readsb, GPS support, the SkyTracker Agent, Display UI,
# and starts everything as a systemd service. The agent auto-registers
# with skytracker.ai on first boot — no API key needed.
#
# Usage:
#   curl -sSL https://get.skytracker.ai | sudo bash
#
# Environment variables (optional):
#   SKYTRACKER_LAT   — Station latitude  (auto-detected from IP if unset)
#   SKYTRACKER_LON   — Station longitude (auto-detected from IP if unset)
#   SKYTRACKER_NAME  — Station name      (default: auto-detected city)
# ============================================================================

set -euo pipefail

# ── Constants ────────────────────────────────────────────────────────────────

REPO="Sky-Tracker-AI/device"
GITHUB_API="https://api.github.com/repos/${REPO}/releases/latest"
AGENT_BIN="/usr/local/bin/skytracker-agent"
CONFIG_DIR="/etc/skytracker"
CONFIG_FILE="${CONFIG_DIR}/config.yaml"
INSTALL_DIR="/opt/skytracker"
UI_DIR="${INSTALL_DIR}/ui"
DATA_DIR="${INSTALL_DIR}/data"
SERVICE_NAME="skytracker"
DISPLAY_PORT=8888

# ── Colors ───────────────────────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

info()    { echo -e "${BOLD}[*]${NC} $*"; }
success() { echo -e "${GREEN}[✓]${NC} $*"; }
warn()    { echo -e "${YELLOW}[!]${NC} $*"; }
fail()    { echo -e "${RED}[✗]${NC} $*"; exit 1; }

# ── Banner ───────────────────────────────────────────────────────────────────

banner() {
    echo ""
    echo -e "${CYAN}"
    cat << 'ART'
   _____ _          _______             _
  / ____| |        |__   __|           | |
 | (___ | | ___   _   | |_ __ __ _  ___| | _____ _ __
  \___ \| |/ / | | |  | | '__/ _` |/ __| |/ / _ \ '__|
  ____) |   <| |_| |  | | | | (_| | (__|   <  __/ |
 |_____/|_|\_\\__, |  |_|_|  \__,_|\___|_|\_\___|_|
               __/ |
              |___/
ART
    echo -e "${NC}"
    echo -e "  ${BOLD}SkyTracker Device Installer${NC}"
    echo "  https://skytracker.ai"
    echo ""
    echo "  ──────────────────────────────────────────"
    echo ""
}

# ── Preflight ────────────────────────────────────────────────────────────────

check_root() {
    if [[ $EUID -ne 0 ]]; then
        fail "This script must be run as root. Try: curl -sSL https://get.skytracker.ai | sudo bash"
    fi
}

detect_arch() {
    local machine
    machine="$(uname -m)"
    case "$machine" in
        aarch64|arm64)  ARCH="arm64" ;;
        x86_64)         ARCH="amd64" ;;
        armv7l|armhf)   ARCH="armhf" ;;
        *)              fail "Unsupported architecture: ${machine}. SkyTracker requires arm64, armhf, or amd64." ;;
    esac
    success "Architecture: ${ARCH}"
}

detect_os() {
    if [[ ! -f /etc/os-release ]]; then
        warn "Cannot detect OS — continuing anyway"
        return
    fi
    # shellcheck source=/dev/null
    . /etc/os-release
    success "OS: ${PRETTY_NAME}"
}

# ── Install ADS-B decoder ────────────────────────────────────────────────────
# readsb is a maintained ADS-B decoder in Debian's default repos (trixie+).
# It serves the same aircraft.json format on port 8080.

install_adsb_decoder() {
    if command -v readsb &>/dev/null || command -v dump1090-fa &>/dev/null; then
        success "ADS-B decoder already installed"
        return
    fi

    info "Installing readsb..."

    apt-get update -qq >/dev/null 2>&1

    if apt-get install -y -qq readsb >/dev/null 2>&1; then
        success "readsb installed"
        systemctl enable readsb >/dev/null 2>&1 || true
        systemctl start readsb 2>/dev/null || true
    else
        warn "Could not install readsb — install it manually: sudo apt install readsb"
    fi
}

# ── Install GPS support ──────────────────────────────────────────────────────

install_gps() {
    if command -v gpsd &>/dev/null; then
        success "gpsd already installed"
        return
    fi

    info "Installing GPS support..."
    if apt-get install -y -qq gpsd gpsd-clients >/dev/null 2>&1; then
        success "GPS support installed"
    else
        warn "GPS install failed — optional, continuing"
    fi
}

# ── Download agent binary ────────────────────────────────────────────────────

download_agent() {
    info "Fetching latest release..."

    apt-get install -y -qq curl jq >/dev/null 2>&1 || true

    RELEASE_JSON="$(curl -fsSL "$GITHUB_API")" || fail "Failed to fetch release info from GitHub"
    RELEASE_TAG="$(echo "$RELEASE_JSON" | jq -r '.tag_name')"
    info "Latest release: ${RELEASE_TAG}"

    local asset_name="skytracker-agent-linux-${ARCH}"
    local download_url
    download_url="$(echo "$RELEASE_JSON" | jq -r --arg name "$asset_name" \
        '.assets[] | select(.name == $name) | .browser_download_url')"

    if [[ -z "$download_url" || "$download_url" == "null" ]]; then
        fail "No binary for ${ARCH} in release ${RELEASE_TAG}"
    fi

    info "Downloading ${asset_name}..."
    curl -fsSL -o "$AGENT_BIN" "$download_url" || fail "Download failed"
    chmod +x "$AGENT_BIN"
    success "Agent installed at ${AGENT_BIN}"
}

# ── Download UI files ────────────────────────────────────────────────────────

download_ui() {
    info "Downloading UI files..."
    mkdir -p "$UI_DIR"

    local ui_url
    ui_url="$(echo "$RELEASE_JSON" | jq -r \
        '.assets[] | select(.name == "ui.tar.gz") | .browser_download_url')"

    if [[ -z "$ui_url" || "$ui_url" == "null" ]]; then
        warn "No ui.tar.gz in release — display UI will need manual install"
        return
    fi

    curl -fsSL "$ui_url" | tar xz -C "$UI_DIR" || {
        warn "UI download failed — agent will still work without display"
        return
    }
    success "UI installed at ${UI_DIR}"
}

# ── Download enrichment database ─────────────────────────────────────────────

download_enrichment() {
    mkdir -p "$DATA_DIR"

    local db_url
    db_url="$(echo "$RELEASE_JSON" | jq -r \
        '.assets[] | select(.name == "enrichment.db") | .browser_download_url')"

    if [[ -n "$db_url" && "$db_url" != "null" ]]; then
        info "Downloading aircraft database..."
        curl -fsSL -o "${DATA_DIR}/enrichment.db" "$db_url" || warn "Enrichment DB download failed"
        success "Aircraft database installed"
    fi
}

# ── Create config ────────────────────────────────────────────────────────────

create_config() {
    mkdir -p "$CONFIG_DIR"

    if [[ -f "$CONFIG_FILE" ]]; then
        success "Config exists at ${CONFIG_FILE} — not overwriting"
        return
    fi

    info "Creating configuration..."

    # Resolve coordinates
    local lat="${SKYTRACKER_LAT:-}"
    local lon="${SKYTRACKER_LON:-}"
    local name="${SKYTRACKER_NAME:-}"

    # Auto-detect from IP geolocation
    if [[ -z "$lat" || -z "$lon" ]]; then
        info "Detecting location from IP..."
        local geo
        if geo="$(curl -fsSL --connect-timeout 5 https://ipinfo.io/json 2>/dev/null)"; then
            local loc
            loc="$(echo "$geo" | jq -r '.loc // empty' 2>/dev/null)"
            if [[ -n "$loc" ]]; then
                lat="${loc%%,*}"
                lon="${loc##*,}"
                if [[ -z "$name" ]]; then
                    name="$(echo "$geo" | jq -r '.city // empty' 2>/dev/null)"
                fi
                success "Location: ${name:-unknown} (${lat}, ${lon})"
            fi
        fi
        lat="${lat:-0}"
        lon="${lon:-0}"
    fi

    name="${name:-My SkyTracker}"
    # Sanitize name to prevent YAML/shell injection.
    name="$(echo "$name" | tr -cd '[:alnum:] _.,-')"

    cat > "$CONFIG_FILE" << CFGEOF
# SkyTracker Device Configuration
# Docs: https://github.com/skytracker-ai/skytracker-device

station:
  name: "${name}"
  sharing: private
  lat: ${lat}
  lon: ${lon}

platform:
  api_key: ""
  endpoint: "https://api.skytracker.ai"

display:
  port: ${DISPLAY_PORT}
  brightness: 100
  night_mode:
    enabled: true
    start: "21:00"
    end: "06:00"

sources:
  dump1090_url: "http://localhost:8080/data/aircraft.json"
  gpsd_host: "localhost"
  gpsd_port: 2947

advanced:
  poll_interval_ms: 1000
  max_range_nm: 250
  data_queue_max_mb: 100
CFGEOF

    success "Config created at ${CONFIG_FILE}"
}

# ── Create systemd service ───────────────────────────────────────────────────

create_service() {
    info "Creating systemd service..."

    cat > "/etc/systemd/system/${SERVICE_NAME}.service" << SVCEOF
[Unit]
Description=SkyTracker Agent
After=network.target readsb.service dump1090-fa.service
Wants=readsb.service

[Service]
Type=simple
ExecStart=${AGENT_BIN} --config ${CONFIG_FILE}
WorkingDirectory=${INSTALL_DIR}
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
SVCEOF

    systemctl daemon-reload
    systemctl enable "${SERVICE_NAME}" >/dev/null 2>&1
    success "Systemd service created and enabled"
}

# ── Start ────────────────────────────────────────────────────────────────────

start_service() {
    info "Starting SkyTracker..."
    systemctl restart "${SERVICE_NAME}"
    success "SkyTracker is running"
}

# ── Done ─────────────────────────────────────────────────────────────────────

print_success() {
    local hostname_val
    hostname_val="$(hostname 2>/dev/null || echo 'localhost')"

    echo ""
    echo -e "  ${GREEN}──────────────────────────────────────────${NC}"
    echo -e "  ${GREEN}${BOLD}  SkyTracker installed successfully!${NC}"
    echo -e "  ${GREEN}──────────────────────────────────────────${NC}"
    echo ""
    echo -e "  ${BOLD}Display:${NC}  http://${hostname_val}:${DISPLAY_PORT}"
    echo ""
    echo -e "  ${BOLD}Claim your device:${NC}"
    echo "    1. Open the display URL or connect a screen"
    echo "    2. Scan the QR code or note the claim code"
    echo "    3. Visit https://skytracker.ai/claim"
    echo ""
    echo -e "  ${BOLD}Commands:${NC}"
    echo "    sudo systemctl status ${SERVICE_NAME}      # check status"
    echo "    sudo journalctl -u ${SERVICE_NAME} -f      # view logs"
    echo "    sudo nano ${CONFIG_FILE}   # edit config"
    echo ""
}

# ── Main ─────────────────────────────────────────────────────────────────────

main() {
    banner
    check_root
    detect_os
    detect_arch
    install_adsb_decoder
    install_gps
    download_agent
    download_ui
    download_enrichment
    create_config
    create_service
    start_service
    print_success
}

main "$@"
