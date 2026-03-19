#!/usr/bin/env bash
# ============================================================================
# SkyTracker — Chromium Kiosk Mode Launcher
# ============================================================================
# Launches Chromium in full-screen kiosk mode pointing at the SkyTracker
# Display UI served on localhost. Designed for Raspberry Pi with an attached
# display (reference: 7" 1024x600).
#
# Usage:
#   ./scripts/kiosk.sh [URL]
#
# Add to autostart on boot:
#   Edit ~/.config/lxsession/LXDE-pi/autostart (or equivalent) and add:
#   @/opt/skytracker/scripts/kiosk.sh
#
# Or create a systemd user service:
#   systemctl --user enable skytracker-kiosk
# ============================================================================

set -euo pipefail

URL="${1:-http://localhost:8888}"

# ── Disable screen blanking and screensaver ──────────────────────────────────

# X11 screensaver off
if command -v xset &> /dev/null; then
    xset s off          # Disable screen saver
    xset -dpms 2>/dev/null || true  # Disable DPMS (not all X servers support it)
    xset s noblank      # Don't blank the screen
fi

# Wayland / wlr-randr alternative (for newer Pi OS with Wayland)
if command -v wlr-randr &> /dev/null; then
    # No direct DPMS toggle via wlr-randr, but setting is handled by compositor
    true
fi

# ── Clean up Chromium crash flags (prevents "restore session" dialog) ────────

CHROMIUM_DIR="${HOME}/.config/chromium"
if [[ -d "$CHROMIUM_DIR/Default" ]]; then
    sed -i 's/"exited_cleanly":false/"exited_cleanly":true/' \
        "${CHROMIUM_DIR}/Default/Preferences" 2>/dev/null || true
    sed -i 's/"exit_type":"Crashed"/"exit_type":"Normal"/' \
        "${CHROMIUM_DIR}/Default/Preferences" 2>/dev/null || true
fi

# ── Wait for the SkyTracker Agent to be ready ────────────────────────────────

echo "[kiosk] Waiting for SkyTracker UI at ${URL}..."
MAX_WAIT=60
WAITED=0
while ! curl -sf "${URL}" > /dev/null 2>&1; do
    sleep 1
    WAITED=$((WAITED + 1))
    if [[ $WAITED -ge $MAX_WAIT ]]; then
        echo "[kiosk] WARNING: Timed out waiting for ${URL} after ${MAX_WAIT}s. Launching anyway."
        break
    fi
done

if [[ $WAITED -lt $MAX_WAIT ]]; then
    echo "[kiosk] UI is ready (waited ${WAITED}s)."
fi

# ── Launch Chromium ──────────────────────────────────────────────────────────

# ── Find Chromium binary ──────────────────────────────────────────────────
CHROMIUM=""
for bin in chromium-browser chromium; do
    if command -v "$bin" &> /dev/null; then
        CHROMIUM="$bin"
        break
    fi
done
if [[ -z "$CHROMIUM" ]]; then
    echo "[kiosk] ERROR: Chromium not found"
    exit 1
fi

echo "[kiosk] Launching ${CHROMIUM} in kiosk mode → ${URL}"

exec "$CHROMIUM" \
    --kiosk \
    --no-first-run \
    --noerrdialogs \
    --disable-infobars \
    --disable-session-crashed-bubble \
    --disable-restore-session-state \
    --disable-translate \
    --disable-features=TranslateUI \
    --disable-component-update \
    --check-for-update-interval=31536000 \
    --autoplay-policy=no-user-gesture-required \
    --password-store=basic \
    --disable-pinch \
    --overscroll-history-navigation=0 \
    --enable-features=OverlayScrollbar \
    --enable-gpu-rasterization \
    --enable-zero-copy \
    --ignore-gpu-blocklist \
    --disable-smooth-scrolling \
    --window-size=1024,600 \
    --window-position=0,0 \
    "${URL}"
