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
#   SKYTRACKER_LAT              — Station latitude  (auto-detected from IP if unset)
#   SKYTRACKER_LON              — Station longitude (auto-detected from IP if unset)
#   SKYTRACKER_NAME             — Station name      (default: auto-detected city)
#   SKYTRACKER_REPLACE_READSB   — Set to 1 to replace an existing readsb / feeder
#                                 install (fr24feed, piaware, rbfeeder, etc.).
#                                 Default: abort if another feeder is detected.
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
# The Debian-packaged readsb lacks RTL-SDR support, so we use wiedehopf's
# build script which compiles readsb from source with full SDR support.
# It also installs tar1090 (lighttpd) which serves aircraft.json on port 80.
#
# Attribution: readsb is maintained by wiedehopf (github.com/wiedehopf/readsb,
# GPL-2.0), descended from Mutability's dump1090 fork and antirez's original
# dump1090. SkyTracker does not modify or redistribute readsb — we install it
# unchanged via the upstream installer. See ACKNOWLEDGMENTS.md for full credits.

# Checks for other ADS-B feeder software that would be disrupted by replacing
# readsb. Returns 0 if a conflicting feeder is detected.
detect_existing_feeder() {
    local found=()
    # Packages
    for pkg in fr24feed piaware rbfeeder adsbexchange-feed dump1090-fa dump1090-mutability; do
        if dpkg -l "$pkg" >/dev/null 2>&1; then
            found+=("$pkg (package)")
        fi
    done
    # Services (covers manual installs)
    for svc in fr24feed piaware rbfeeder adsbexchange-feed dump1090-fa readsb; do
        if systemctl list-unit-files "${svc}.service" >/dev/null 2>&1 \
           && systemctl is-enabled "${svc}.service" >/dev/null 2>&1; then
            found+=("${svc}.service")
        fi
    done
    if [[ ${#found[@]} -gt 0 ]]; then
        printf '%s\n' "${found[@]}"
        return 0
    fi
    return 1
}

install_adsb_decoder() {
    if command -v readsb &>/dev/null; then
        success "ADS-B decoder already installed"
        return
    fi

    info "Installing readsb via wiedehopf/adsb-scripts (github.com/wiedehopf/readsb, GPL-2.0)"
    info "SkyTracker uses readsb unchanged — see ACKNOWLEDGMENTS.md for full credits"

    # Refuse to touch an existing feeder setup unless the user opts in.
    if existing=$(detect_existing_feeder); then
        if [[ "${SKYTRACKER_REPLACE_READSB:-0}" != "1" ]]; then
            warn "Detected existing ADS-B feeder software on this device:"
            while IFS= read -r line; do
                echo "       - $line"
            done <<< "$existing"
            echo ""
            warn "Installing SkyTracker's readsb build would replace your current setup"
            warn "and likely break feeds to FlightRadar24, FlightAware, ADS-B Exchange, etc."
            echo ""
            echo "       If you want SkyTracker to share this device with other feeders,"
            echo "       see the multi-feeder guide: https://skytracker.ai/docs/shared-feeder"
            echo ""
            echo "       To replace the existing decoder anyway, re-run with:"
            echo "         SKYTRACKER_REPLACE_READSB=1 curl -sSL https://get.skytracker.ai | sudo -E bash"
            fail "Aborting to protect your existing feeder setup"
        fi
        warn "SKYTRACKER_REPLACE_READSB=1 set — proceeding to replace existing decoder"
    fi

    apt-get update -qq >/dev/null 2>&1
    apt-get install -y -qq curl librtlsdr0 rtl-sdr >/dev/null 2>&1

    # Remove Debian-packaged readsb if present (no RTL-SDR support).
    # Only reached when no other feeder was detected, or user opted in above.
    if dpkg -l readsb >/dev/null 2>&1; then
        info "Removing Debian-packaged readsb (lacks RTL-SDR support)"
        apt-get remove -y -qq readsb >/dev/null 2>&1 || true
    fi

    if bash -c "$(curl -fsSL https://raw.githubusercontent.com/wiedehopf/adsb-scripts/master/readsb-install.sh)" >/dev/null 2>&1; then
        success "readsb installed with RTL-SDR support (thanks to wiedehopf)"
    else
        warn "Could not install readsb — see https://github.com/wiedehopf/adsb-scripts"
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

# ── Install Bluetooth support ────────────────────────────────────────────────

install_bluetooth() {
    if command -v bluetoothctl &>/dev/null; then
        success "BlueZ already installed"
    else
        info "Installing Bluetooth support..."
        apt-get install -y -qq bluez >/dev/null 2>&1 || {
            warn "BlueZ install failed — BLE provisioning will be unavailable"
            return
        }
        success "BlueZ installed"
    fi
    systemctl enable bluetooth >/dev/null 2>&1 || true
    systemctl start bluetooth >/dev/null 2>&1 || true
}

# ── Install SatDump (satellite decoder) ──────────────────────────────────────

install_satdump() {
    if command -v satdump &>/dev/null; then
        local current_ver
        current_ver="$(satdump --version 2>&1 | grep -oP 'v\K[0-9]+\.[0-9]+' | head -1 || echo "0")"
        if [[ "$current_ver" == "2.0" ]]; then
            success "SatDump 2.0 already installed"
            return
        fi
        info "Removing old SatDump..."
        apt-get remove -y -qq satdump satdump-data >/dev/null 2>&1 || true
    fi

    info "Installing SatDump 2.0 (nightly)..."

    local deb_url="https://github.com/SatDump/SatDump/releases/download/nightly/satdump_rpi64_latest_arm64.deb"
    local deb_path="/tmp/satdump_nightly.deb"

    if [[ "$ARCH" != "arm64" ]]; then
        warn "SatDump nightly .deb only available for arm64 — skipping"
        return
    fi

    # Pre-install runtime dependencies. The nightly .deb targets Bookworm but
    # Debian Trixie ships newer sonames for some libraries. Install what we can
    # before dpkg so that apt stays healthy.
    info "Installing SatDump runtime dependencies..."
    apt-get install -y -qq \
        libfftw3-bin libfftw3-dev \
        libjemalloc2 \
        libhackrf0 \
        libairspy0 \
        libairspyhf1 \
        libglfw3-wayland \
        libportaudiocpp0 \
        libdbus-1-dev \
        libvolk-bin \
        libnng1 \
        >/dev/null 2>&1 || warn "Some optional SatDump dependencies unavailable (non-fatal)"

    curl -fsSL -o "$deb_path" "$deb_url" || {
        warn "Failed to download SatDump — satellite decoding will be unavailable"
        return
    }

    # The nightly .deb may have dependency mismatches on newer Debian (trixie).
    # Force install and hold the package to prevent apt from removing it.
    dpkg --force-depends -i "$deb_path" >/dev/null 2>&1 || {
        warn "SatDump install had dependency warnings (non-fatal)"
    }
    apt-mark hold satdump >/dev/null 2>&1 || true
    rm -f "$deb_path"

    # The nightly .deb links against libvolk.so.2.5 (Bookworm) but Trixie
    # ships libvolk 3.x. Create a compat symlink if the expected soname is
    # missing but a newer version exists.
    if ! ldconfig -p 2>/dev/null | grep -q "libvolk.so.2.5"; then
        local volk_real
        volk_real="$(find /usr/lib -name 'libvolk.so.*' ! -name '*.so.2.5' 2>/dev/null | head -1)"
        if [[ -n "$volk_real" ]]; then
            ln -sf "$volk_real" /usr/lib/aarch64-linux-gnu/libvolk.so.2.5
            ldconfig
            info "Created libvolk compat symlink: libvolk.so.2.5 -> $(basename "$volk_real")"
        fi
    fi

    # SatDump 2.0 looks for plugins in ./plugins relative to its cwd
    # (/usr/share/satdump). The .deb installs them to /usr/lib/satdump/plugins.
    if [[ -d /usr/lib/satdump/plugins ]] && [[ ! -e /usr/share/satdump/plugins ]]; then
        ln -sf /usr/lib/satdump/plugins /usr/share/satdump/plugins
    fi

    # The SatDump 2.0 nightly .deb has duplicate pipeline names across plugins
    # (e.g. himawaricast_data_decoder, goes_gvar_decoder) which crash the CLI
    # parser. Keep only the plugins SkyTracker actually needs and move the rest
    # out of the way.
    if [[ -d /usr/lib/satdump/plugins ]]; then
        local keep="libmeteor_support.so librtltcp_support.so librtlsdr_sdr_support.so \
libsimd_neon.so libnet_source_support.so libnoaa_metop_support.so \
libinmarsat_support.so libanalog_support.so libgoes_support.so libxrit_support.so"
        mkdir -p /usr/lib/satdump/plugins.disabled
        for f in /usr/lib/satdump/plugins/*.so; do
            local base; base="$(basename "$f")"
            if ! echo "$keep" | grep -qw "$base"; then
                mv "$f" /usr/lib/satdump/plugins.disabled/ 2>/dev/null || true
            fi
        done
    fi

    if command -v satdump &>/dev/null; then
        success "SatDump 2.0 installed"
    else
        warn "SatDump installation failed — satellite decoding will be unavailable"
    fi
}

# ── Install dump978-fa (UAT 978 MHz decoder) ────────────────────────────────

install_dump978() {
    if command -v dump978-fa &>/dev/null; then
        success "dump978-fa already installed"
        return
    fi

    info "Installing dump978-fa (UAT decoder)..."

    apt-get install -y -qq \
        libsoapysdr-dev soapysdr-module-rtlsdr \
        libboost-system-dev libboost-program-options-dev \
        libboost-filesystem-dev libboost-regex-dev \
        build-essential git >/dev/null 2>&1 || {
        warn "Could not install dump978-fa build dependencies — UAT decoding will be unavailable"
        return
    }

    local build_dir="/tmp/dump978-build"
    rm -rf "$build_dir"
    git clone --depth 1 https://github.com/flightaware/dump978.git "$build_dir" >/dev/null 2>&1 || {
        warn "Failed to clone dump978 — UAT decoding will be unavailable"
        return
    }

    if make -C "$build_dir" dump978-fa -j"$(nproc)" >/dev/null 2>&1; then
        cp "$build_dir/dump978-fa" /usr/local/bin/
        chmod +x /usr/local/bin/dump978-fa
        success "dump978-fa installed"
    else
        warn "dump978-fa build failed — UAT decoding will be unavailable"
    fi

    rm -rf "$build_dir"
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

    info "Downloading aircraft database (tar1090-db)..."
    local csv_url="https://raw.githubusercontent.com/wiedehopf/tar1090-db/csv/aircraft.csv.gz"
    if curl -fsSL -o "${DATA_DIR}/aircraft.csv.gz" "$csv_url"; then
        success "Aircraft database installed ($(du -h "${DATA_DIR}/aircraft.csv.gz" | cut -f1) compressed)"
    else
        warn "Aircraft database download failed — enrichment will be limited"
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
  dump1090_url: "http://localhost/tar1090/data/aircraft.json"
  gpsd_host: "localhost"
  gpsd_port: 2947

advanced:
  poll_interval_ms: 1000
  max_range_nm: 250
  data_queue_max_mb: 100

ble:
  enabled: true
  window_seconds: 300
  auto_pair_on_boot: true
  device_name_prefix: "SkyTracker-"
CFGEOF

    success "Config created at ${CONFIG_FILE}"
}

# ── Create systemd service ───────────────────────────────────────────────────

create_service() {
    info "Creating systemd service..."

    cat > "/etc/systemd/system/${SERVICE_NAME}.service" << SVCEOF
[Unit]
Description=SkyTracker Agent
After=network.target readsb.service
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

setup_kiosk() {
    info "Setting up kiosk display..."

    # Install kiosk launcher script
    local kiosk_src="${INSTALL_DIR}/scripts/kiosk.sh"
    local kiosk_dst="${INSTALL_DIR}/kiosk.sh"
    if [[ -f "$kiosk_src" ]]; then
        cp "$kiosk_src" "$kiosk_dst"
    fi
    chmod +x "$kiosk_dst" 2>/dev/null || true

    # Determine the desktop user (the human user, not root)
    local desktop_user
    desktop_user="$(logname 2>/dev/null || echo pi)"
    local user_home
    user_home="$(eval echo "~${desktop_user}")"

    # Disable screensaver in system autostart
    local autostart_dir="/etc/xdg/lxsession"
    for session_dir in "$autostart_dir"/*/; do
        if [[ -f "${session_dir}autostart" ]]; then
            sed -i '/@xscreensaver/d' "${session_dir}autostart"
        fi
    done

    # Disable gnome-keyring (Chromium triggers its unlock prompt in kiosk mode).
    # XDG autostart suppression:
    for component in gnome-keyring-pkcs11 gnome-keyring-secrets gnome-keyring-ssh; do
        cat > "/etc/xdg/autostart/${component}.desktop" << GKEOF
[Desktop Entry]
Type=Application
Name=${component}
Hidden=true
GKEOF
    done
    # PAM-level suppression (keyring is also started by lightdm session):
    for pam_file in /etc/pam.d/login /etc/pam.d/lightdm /etc/pam.d/lightdm-autologin; do
        if [[ -f "$pam_file" ]]; then
            sed -i '/pam_gnome_keyring.so/s/^[^#]/#&/' "$pam_file"
        fi
    done

    # Create autostart entry for kiosk browser.
    # Detect compositor: labwc (Wayland, Pi OS Bookworm+) vs LXDE (X11, older Pi OS).
    local kiosk_url="http://localhost:${DISPLAY_PORT}"

    if [[ -d "${user_home}/.config/labwc" ]]; then
        # labwc (Wayland) — uses ~/.config/labwc/autostart
        local labwc_autostart="${user_home}/.config/labwc/autostart"
        # Append kiosk line if not already present
        if ! grep -q "kiosk.sh" "$labwc_autostart" 2>/dev/null; then
            echo "${kiosk_dst} ${kiosk_url} &" >> "$labwc_autostart"
        fi
        chown "${desktop_user}:" "$labwc_autostart"
        info "Kiosk autostart: labwc (Wayland)"
    else
        # LXDE / XDG autostart — uses ~/.config/autostart/*.desktop
        local user_autostart="${user_home}/.config/autostart"
        mkdir -p "$user_autostart"
        cat > "${user_autostart}/skytracker-kiosk.desktop" << KIOSKEOF
[Desktop Entry]
Type=Application
Name=SkyTracker Kiosk
Exec=${kiosk_dst} ${kiosk_url}
Hidden=false
X-GNOME-Autostart-enabled=true
KIOSKEOF
        chown -R "${desktop_user}:" "${user_autostart}"
        info "Kiosk autostart: LXDE (X11)"
    fi

    success "Kiosk display configured (launches on login)"
}

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
    install_bluetooth
    install_satdump
    install_dump978
    download_agent
    download_ui
    download_enrichment
    create_config
    create_service
    setup_kiosk
    start_service
    print_success
}

main "$@"
