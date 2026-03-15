# SkyTracker Device — Product Requirements Document

**Open-Source On-Device Software for ADS-B Aircraft Tracking**

v0.1 · March 2026

---

## 1. Product Overview

**skytracker-device** is open-source software that turns any Linux single-board computer with an RTL-SDR receiver into a real-time ADS-B aircraft tracker with a live radar display. It is the on-device half of the [SkyTracker](https://skytracker.ai) platform.

The software is **offline-first**: it works with zero internet connectivity. Plug in the hardware, boot up, and see every aircraft broadcasting on 1090 MHz — no account, no cloud, no WiFi required. When optionally connected to [skytracker.ai](https://skytracker.ai), the device joins the SkyTracker Network (a global community of shared ADS-B ground stations) and gains access to AI-powered features like rarity scoring, lifer tracking, and tail number stories.

**The analogy:** Davis Instruments built WeatherLink as a standalone weather station first, then added a community network on top. SkyTracker follows the same playbook — then goes further. The hardware works perfectly offline. The network is what makes it valuable. The AI is what makes it extraordinary.

### What This Repo Contains

- **SkyTracker Agent** — a single Go binary that orchestrates the device
- **Display UI** — vanilla HTML/CSS/JS rendered on an HTML Canvas, served locally
- **Setup scripts** — installation and configuration for supported hardware
- **Enrichment data** — local SQLite databases for aircraft type and airline lookups

### What This Repo Does NOT Contain

- The skytracker.ai cloud platform (proprietary, in the separate `skytracker` repo)
- AI/ML model training or inference code
- E-commerce, billing, or subscription logic

---

## 2. Open Source Strategy

### License

MIT or Apache 2.0 (final decision TBD before first public release).

### Why Open Source

The SkyTracker Network grows when more devices come online. Open-sourcing the device software removes every barrier to adoption:

- Anyone can build their own SkyTracker from commodity hardware
- The community can port it to new SBCs, displays, and configurations
- Trust — users can verify exactly what their device sends to the platform
- The device is the acquisition funnel; the platform is the business

### What's Open

| Component | Open? |
|-----------|-------|
| SkyTracker Agent (Go) | Yes |
| Display UI (JS/Canvas) | Yes |
| Setup & install scripts | Yes |
| SQLite enrichment databases | Yes |
| Documentation & build guides | Yes |

### What's Proprietary

| Component | Where |
|-----------|-------|
| skytracker.ai cloud platform | `skytracker` repo |
| AI/ML models (rarity, anomaly, fingerprinting) | `skytracker` repo |
| Web frontend (Angular) | `skytracker` repo |

### Community Contributions Welcome

- UI themes and color schemes
- Support for new SBC platforms
- Enrichment database updates (ICAO hex, airline callsign prefixes)
- Localization / translations
- Display layout variants (headless dashboards, alternate screen sizes)

---

## 3. Supported Hardware

### Reference Build

The reference build uses off-the-shelf components totaling approximately **$168**:

| Component | Example | Cost |
|-----------|---------|------|
| RTL-SDR Blog V4 + Antenna Kit | R828D tuner, dipole antenna included | ~$40 |
| Raspberry Pi 4 (2GB+) | CanaKit starter with case, fan, SD, PSU | ~$75 |
| 7" IPS Display (1024x600) | GeeekPi, non-touch, plug and play | ~$35 |
| USB GPS Dongle | Geekstory BN-808 (M8030-KT) | ~$18 |
| **Total** | | **~$168** |

### Minimum Requirements

- Any Linux SBC with at least one USB port
- 1GB RAM minimum (2GB recommended)
- Display output (HDMI or DSI) — *optional if running headless*
- RTL-SDR compatible USB dongle for 1090 MHz reception
- SD card or eMMC with at least 4GB free

### Tested Configurations

| SBC | Display | SDR | GPS | Status |
|-----|---------|-----|-----|--------|
| Raspberry Pi 4 (2GB) | GeeekPi 7" 1024x600 | RTL-SDR Blog V4 | BN-808 USB | Reference build |
| Raspberry Pi 4 (4GB) | Official Pi 7" touchscreen | RTL-SDR Blog V4 | BN-808 USB | Tested |
| Raspberry Pi 5 | Any HDMI display | RTL-SDR Blog V4 | BN-808 USB | Planned |

*Community-tested configurations will be added as contributors verify them.*

### Optional Components

- **GPS dongle** — enables automatic location detection. Without GPS, station coordinates must be set manually in the config file.
- **Display** — without a display, the device runs in **headless feeder mode**, contributing data to the SkyTracker Network without a local UI.

---

## 4. Software Architecture

### High-Level Data Flow

```
┌──────────┐     ┌────────────┐     ┌──────────────────┐     ┌───────────────┐
│ RTL-SDR  │────▶│ dump1090-fa│────▶│ SkyTracker Agent │────▶│  Display UI   │
│ receiver │     │  (decoder) │     │      (Go)        │     │ (JS + Canvas) │
└──────────┘     └────────────┘     └──────────────────┘     └───────────────┘
                                           │                        ▲
                  ┌──────┐                 │   localhost WebSocket   │
                  │ gpsd │────────────────▶│◀───────────────────────┘
                  └──────┘                 │
                                           │  (when WiFi available)
                                           ▼
                                    ┌──────────────┐
                                    │ skytracker.ai│
                                    │   (cloud)    │
                                    └──────────────┘
```

### Two Main Components

1. **SkyTracker Agent (Go)** — a single compiled binary. Polls dump1090-fa for aircraft data, reads GPS, enriches with local SQLite, computes geometry, serves the UI, manages connectivity, and optionally syncs with skytracker.ai.

2. **Display UI (Vanilla JS + Canvas)** — static HTML, CSS, and JS files served by the Agent over localhost. Renders the radar display in an HTML Canvas element. Connects to the Agent via WebSocket for real-time data. No build step, no framework, no bundler.

### System Dependencies

| Dependency | Purpose |
|------------|---------|
| `dump1090-fa` | Decodes 1090 MHz ADS-B signals from RTL-SDR, exposes `/data/aircraft.json` |
| `gpsd` | Reads NMEA sentences from USB GPS dongle, provides lat/lon |
| `chromium-browser` | Renders the Display UI in kiosk mode (optional — only needed with a display) |
| `sqlite3` | Local enrichment database (ICAO hex → type, callsign prefix → airline) |

---

## 5. SkyTracker Agent (Go)

The Agent is the core of the device. It is a single Go binary with zero runtime dependencies beyond the system services listed above.

### Responsibilities

#### Core (Offline)

- **Poll dump1090-fa** at `/data/aircraft.json` at 1 Hz for live aircraft positions
- **Read GPS position** from gpsd for station location and automatic configuration
- **Enrich aircraft data** using local SQLite databases:
  - ICAO hex code → aircraft type (e.g., `A0B1C2` → `Boeing 737-800`)
  - Callsign prefix → airline name (e.g., `SWA` → `Southwest Airlines`)
- **Compute distance and bearing** from station to each aircraft using Haversine formula
- **Serve the Display UI** as static files on `localhost:<port>` (default port: 8080)
- **Expose a WebSocket** at `localhost:<port>/ws` that pushes real-time enriched aircraft data to the UI at 1 Hz

#### Connected (When WiFi Available)

- **Manage WiFi** — run a captive portal on first boot for WiFi credential entry
- **Register device** with skytracker.ai on first successful internet connection
- **Stream aircraft data** to skytracker.ai when sharing is enabled by the user
- **Queue data locally** when offline — buffer aircraft sightings in a SQLite queue, sync when connectivity resumes
- **Receive enrichment data** from skytracker.ai — rarity scores, aircraft photos, origin/destination info
- **Handle OTA updates** — check GitHub Releases on a schedule, download and verify new versions
- **Send health metrics** — uptime, GPS fix status, aircraft count, agent version, last sync time

### Configuration

The Agent reads configuration from `/etc/skytracker/config.yaml` or `~/.skytracker/config.yaml` (user-level overrides system-level).

```yaml
# Station identity
station:
  name: "My SkyTracker"
  sharing: private  # private | unlisted | public

# skytracker.ai connection
platform:
  api_key: ""  # Set after device claim
  endpoint: "https://api.skytracker.ai"

# Display
display:
  port: 8080
  brightness: 100  # 0-100
  night_mode:
    enabled: true
    start: "21:00"
    end: "06:00"

# Data sources
sources:
  dump1090_url: "http://localhost:8080/data/aircraft.json"
  gpsd_host: "localhost"
  gpsd_port: 2947

# Advanced
advanced:
  poll_interval_ms: 1000
  max_range_nm: 250
  data_queue_max_mb: 100
```

---

## 6. Display UI (Vanilla JS + Canvas)

The Display UI is a set of static files — HTML, CSS, and JavaScript — rendered in Chromium kiosk mode on the device's attached display. There is no framework, no build step, and no bundler. Edit a file, refresh the browser, see the change.

### Three-Zone Layout (1024x600)

```
┌─────────────────────────────┬──────────────────────┐
│                             │  Selected Aircraft   │
│                             │  ─────────────────   │
│        Radar Display        │  SWA1234             │
│                             │  Boeing 737-800      │
│     (rotating sweep line,   │  Southwest Airlines  │
│      aircraft dots color-   │  Alt: 35,000 ft      │
│      coded by altitude)     │  Spd: 480 kts        │
│                             │  Hdg: 270°           │
│                             │  Dist: 12.4 nm       │
│                             │  ★ Rarity: 2         │
│                             ├──────────────────────┤
│                             │  Aircraft List       │
│                             │  ─────────────────   │
│                             │  SWA1234  B738  35k  │
│                             │  UAL445   A320  28k  │
│                             │  N172SP   C172   3k  │
│                             │  ★ EVAC01 C17   18k  │
│                             │  DAL88    B763  38k  │
└─────────────────────────────┴──────────────────────┘
```

- **Left — Radar Display**: rotating sweep line, aircraft rendered as dots on a polar projection centered on the station. Dots are color-coded by altitude. Range rings indicate distance.
- **Right Upper — Aircraft Detail**: tapping/clicking an aircraft in the list or on the radar shows full detail — callsign, type, airline, altitude, speed, heading, distance, route (if available), and rarity score (if connected to skytracker.ai).
- **Right Lower — Aircraft List**: scrollable list of all aircraft currently in range, sorted by distance. Shows callsign, type code, altitude, speed, and rarity badge (if connected).

### Altitude Color Coding

| Color | Hex | Altitude Range | Traffic Type |
|-------|-----|----------------|-------------|
| Cyan | `#60efff` | > 30,000 ft | High altitude commercial |
| Purple | `#a78bfa` | 15,000–30,000 ft | Mid altitude |
| Green | `#34d399` | 5,000–15,000 ft | Low altitude |
| Amber | `#fbbf24` | < 5,000 ft | Very low / approach / GA |

### Rarity Badge Display

- Aircraft with rarity score **7+** display a visible **gold badge** on the radar dot and in the aircraft list
- Score **9–10** triggers a **prominent card** in the detail panel
- Rarity data is only available when connected to skytracker.ai — the badge is simply absent when offline

### Network Status Indicator

| Indicator | Meaning |
|-----------|---------|
| Green dot | Connected to skytracker.ai, sharing active |
| Gold dot | AI features active (SkyTracker Pro) |
| Gray dot | WiFi connected, sharing disabled |
| No indicator | Fully offline (default, no WiFi configured) |

### Night Mode

- Automatically dims the display based on time of day (configurable schedule)
- Reduces brightness and shifts the color palette for low-light environments
- Can be manually toggled or disabled in config

### Display Compatibility

The UI is designed for 1024x600 (7" reference display) but is responsive enough to work on displays from 7" to 13". The Canvas-based rendering adapts to the available viewport.

---

## 7. Data Flow

### Offline Path (No Internet)

```
RTL-SDR → dump1090-fa → Agent (polls /data/aircraft.json at 1 Hz)
                              ↓
                     Enrich with local SQLite
                     (ICAO hex → type, callsign → airline)
                              ↓
                     Compute distance & bearing from GPS position
                              ↓
                     Push via WebSocket to Display UI
```

Every feature in this path works without any internet connectivity. This is the device's default and primary operating mode.

### Connected Path (WiFi Available)

```
[Same offline path as above]
         +
Agent → skytracker.ai API:
  - POST /api/v1/ingest (batch aircraft sightings)
  - POST /api/v1/devices/health (device metrics)

skytracker.ai → Agent:
  - Rarity scores for visible aircraft
  - Enrichment data (photos, origin/destination)
  - OTA update manifest
  - Configuration updates (sharing prefs changed on website)
```

### Local Data Queue

When WiFi is available but drops mid-session, aircraft sighting data is buffered in a local SQLite queue. When connectivity resumes, queued data is synced to skytracker.ai in chronological order. The queue has a configurable size limit (default: 100 MB) and drops oldest entries when full.

---

## 8. Setup & Installation

### Option 1: Pre-Built SD Image (Quickest)

1. Download the latest SkyTracker SD image from GitHub Releases
2. Flash it to an SD card using Raspberry Pi Imager or balenaEtcher
3. Insert SD card, plug in RTL-SDR, GPS dongle, and display
4. Power on — see planes within 60 seconds

### Option 2: DIY Install on Existing Linux System

```bash
# Install system dependencies
sudo apt install dump1090-fa gpsd chromium-browser sqlite3

# Download SkyTracker Agent binary and UI files
curl -L https://github.com/skytracker/skytracker-device/releases/latest/download/skytracker-agent-arm64 -o /usr/local/bin/skytracker-agent
chmod +x /usr/local/bin/skytracker-agent

curl -L https://github.com/skytracker/skytracker-device/releases/latest/download/skytracker-ui.tar.gz | tar xz -C /opt/skytracker/ui

# Start the agent
skytracker-agent --config /etc/skytracker/config.yaml
```

### First Boot Flow

1. **Agent starts** — detects dump1090-fa and GPS, begins polling aircraft data
2. **Display shows radar immediately** — offline mode, no internet required
3. **If WiFi is available**: Agent opens a captive portal for WiFi credential configuration
4. **On WiFi connect**: Agent registers with skytracker.ai. Device serial and QR code (printed on hardware) allow the user to claim the device at `skytracker.ai/claim`
5. **User sets sharing preference** (private/unlisted/public) — station goes live on the SkyTracker Network
6. **Ongoing**: Agent syncs data, receives rarity scores, checks for updates

### Headless Feeder Mode

No display attached? The device runs as a pure network feeder:

- Agent starts and ingests aircraft data normally
- No Chromium process launched
- Station contributes data to the SkyTracker Network
- Station appears on the community map at skytracker.ai/map
- All connected features (rarity, lifers, tail stories) are accessible via the web at the station's profile page

---

## 9. API Contract with skytracker.ai

The Agent communicates with the skytracker.ai platform over HTTPS REST endpoints. All communication is initiated by the device — the platform never pushes to the device unsolicited.

### Device → Platform

| Endpoint | Method | Purpose | Payload |
|----------|--------|---------|---------|
| `/api/v1/devices/register` | POST | First-time device registration | Serial number, hardware info (SBC model, OS version), GPS location |
| `/api/v1/ingest` | POST | Batch aircraft sighting upload | Array of sightings: `station_id`, `timestamp`, `icao_hex`, `callsign`, `type`, `altitude`, `speed`, `heading`, `lat`, `lon`, `distance` |
| `/api/v1/devices/health` | POST | Device health metrics | Uptime, GPS fix status, aircraft count, agent version, last sync timestamp |

### Platform → Device (via Response)

The platform returns enrichment data in the responses to the above requests — no separate push channel required.

| Data | Source | Description |
|------|--------|-------------|
| Rarity scores | Response to `/ingest` | Rarity score (1–10) for each ICAO hex in the batch |
| Enrichment data | Response to `/ingest` | Aircraft photos, origin/destination (when available) |
| OTA update manifest | Response to `/health` | Latest agent version, download URL, SHA-256 checksum |
| Configuration updates | Response to `/health` | Updated sharing preferences (if user changed them on the website) |

### Authentication

- Device authenticates with an API key issued during registration
- API key is stored in the local config file
- All requests include the API key in the `Authorization` header

---

## 10. Configuration

### Config File Locations

The Agent checks for configuration in this order:

1. `~/.skytracker/config.yaml` (user-level, highest priority)
2. `/etc/skytracker/config.yaml` (system-level)
3. Built-in defaults

### Full Configuration Reference

| Setting | Default | Description |
|---------|---------|-------------|
| `station.name` | `"SkyTracker"` | Display name for the station |
| `station.sharing` | `private` | Sharing mode: `private`, `unlisted`, or `public` |
| `platform.api_key` | `""` | API key for skytracker.ai (set after device claim) |
| `platform.endpoint` | `https://api.skytracker.ai` | Platform API base URL |
| `display.port` | `8080` | Port for the local web server serving the UI |
| `display.brightness` | `100` | Display brightness (0–100) |
| `display.night_mode.enabled` | `true` | Enable automatic night mode |
| `display.night_mode.start` | `21:00` | Night mode start time (24h format) |
| `display.night_mode.end` | `06:00` | Night mode end time (24h format) |
| `sources.dump1090_url` | `http://localhost:8080/data/aircraft.json` | URL for dump1090-fa aircraft data |
| `sources.gpsd_host` | `localhost` | gpsd hostname |
| `sources.gpsd_port` | `2947` | gpsd port |
| `advanced.poll_interval_ms` | `1000` | Aircraft data polling interval in milliseconds |
| `advanced.max_range_nm` | `250` | Maximum display range in nautical miles |
| `advanced.data_queue_max_mb` | `100` | Max size of offline data queue before dropping oldest entries |

---

## 11. Offline-First Design Principles

These principles are non-negotiable. Every contribution must uphold them.

1. **Every feature must work without internet.** If a feature requires connectivity, it must degrade gracefully — not block, not error, not spin.

2. **Network features are additive, never blocking.** Connected features (rarity scores, photos, enrichment) enhance the display. Their absence must be invisible, not broken.

3. **The display never waits on network calls.** Aircraft data flows from dump1090 → Agent → UI with zero network dependencies in the critical path. Network operations happen asynchronously in background goroutines.

4. **Local enrichment ships with the binary.** The SQLite database of ICAO hex codes and callsign prefixes is bundled with every release. The device enriches aircraft data on its own.

5. **Agent startup doesn't require connectivity.** The Agent must be fully operational within seconds of boot, regardless of WiFi state. Registration, sync, and updates happen when (and if) connectivity becomes available.

6. **Offline is the default, not a fallback.** The device ships in offline mode. Connecting to skytracker.ai is an opt-in enhancement, not a requirement.

---

## 12. OTA Updates

### Update Mechanism

- The Agent checks the GitHub Releases API on a configurable schedule (default: once daily)
- If a new version is available, the Agent downloads the binary and UI assets
- Downloads are verified against a SHA-256 checksum published in the release
- Updates are applied on the next device reboot (or hot-reloaded for UI-only changes)

### Rollback

- The Agent retains the previous version alongside the new one
- On boot, if the new version fails its health check (can't reach dump1090, can't serve UI), the Agent automatically reverts to the previous version
- Manual rollback is available via `skytracker-agent --rollback`

### Update Flow

```
Agent (daily) → GitHub Releases API
  → New version available?
    → No: done
    → Yes: download binary + UI tarball
      → Verify SHA-256 checksum
        → Stage update for next reboot
          → On reboot: start new version
            → Health check passes? Keep it
            → Health check fails? Rollback to previous
```

---

## 13. Roadmap (Device-Specific)

### v0.1 — Core Hardware Integration

- dump1090-fa integration (poll `/data/aircraft.json`)
- GPS auto-location via gpsd
- Basic radar UI with aircraft dots and sweep line
- Aircraft list panel with callsign, type, altitude

### v0.2 — Enrichment & Detail

- SQLite enrichment: ICAO hex → aircraft type, callsign prefix → airline name
- Aircraft detail panel (callsign, type, airline, altitude, speed, heading, distance)
- Altitude color coding (cyan / purple / green / amber)
- Range rings on radar display

### v0.3 — Platform Connection

- SkyTracker Agent: device registration with skytracker.ai
- Aircraft data streaming to platform (POST `/api/v1/ingest`)
- Health metrics reporting (POST `/api/v1/devices/health`)
- Sharing preference support (private / unlisted / public)

### v0.4 — Setup & Polish

- Captive portal for WiFi credential entry on first boot
- Night mode (auto-dim by time of day)
- OTA update system via GitHub Releases
- Chromium kiosk mode auto-start on boot

### v0.5 — Connected Features

- Rarity score display on radar and aircraft list (data from skytracker.ai)
- Gold badge for 7+ rarity, prominent card for 9–10
- Network status indicator (green / gold / gray / none)
- Headless feeder mode (no display, pure network contributor)

### v1.0 — Stable Release

Full offline + connected experience. The device works perfectly standalone and integrates seamlessly with the SkyTracker Network. Ready for community use and contribution.

---

## 14. Contributing

### Development Environment

No SBC or SDR hardware required for development. The Agent and UI can be developed on any macOS or Linux machine.

#### Agent Development (Go)

```bash
# Clone the repo
git clone https://github.com/skytracker/skytracker-device.git
cd skytracker-device

# Build the agent
go build -o skytracker-agent ./cmd/agent

# Run with mock data (no dump1090 or GPS required)
skytracker-agent --mock

# Cross-compile for Raspberry Pi (ARM64)
GOOS=linux GOARCH=arm64 go build -o skytracker-agent-arm64 ./cmd/agent
```

The `--mock` flag generates synthetic aircraft data for development and testing without SDR hardware.

#### UI Development (Vanilla JS)

```bash
# UI files are in the ui/ directory
cd ui/

# Open in any browser — no build step
open index.html

# Or serve with any static file server
python3 -m http.server 8080
```

Edit any `.html`, `.css`, or `.js` file and refresh the browser. That's the entire workflow.

#### Mock Data

A mock dump1090 JSON generator is included for development:

```bash
# Generate mock aircraft.json with realistic data
go run ./cmd/mock-dump1090

# Serves /data/aircraft.json on localhost:8080 with rotating synthetic aircraft
```

### Project Structure

```
skytracker-device/
├── cmd/
│   ├── agent/          # Main agent entrypoint
│   └── mock-dump1090/  # Mock data generator for development
├── internal/
│   ├── adsb/           # dump1090 polling and parsing
│   ├── config/         # Configuration loading
│   ├── enrichment/     # SQLite enrichment lookups
│   ├── geo/            # Distance, bearing, coordinate math
│   ├── gpsd/           # GPS daemon client
│   ├── platform/       # skytracker.ai API client
│   ├── queue/          # Offline data queue
│   ├── server/         # HTTP server + WebSocket
│   ├── updater/        # OTA update logic
│   └── wifi/           # WiFi management + captive portal
├── ui/                 # Display UI (static HTML/CSS/JS)
│   ├── index.html
│   ├── styles.css
│   ├── radar.js        # Canvas radar rendering
│   ├── aircraft.js     # Aircraft list and detail panel
│   └── websocket.js    # WebSocket client
├── data/
│   └── enrichment.db   # SQLite: ICAO hex + callsign databases
├── scripts/
│   ├── install.sh      # System setup script
│   └── kiosk.sh        # Chromium kiosk mode launcher
├── configs/
│   └── config.example.yaml
├── go.mod
├── go.sum
├── LICENSE
├── PRD.md
└── README.md
```

### Code Standards

- **Go**: standard library preferred, minimal dependencies. `go fmt` and `go vet` must pass. Standard Go project layout (`cmd/`, `internal/`).
- **JavaScript**: vanilla JS only — no frameworks, no transpilers, no bundlers. Code must work directly in Chromium without a build step.
- **CSS**: plain CSS. No preprocessors.
- **Tests**: `go test ./...` for the agent. Manual browser testing for the UI (automated tests welcome as contributions).

### Submitting Changes

1. Fork the repo and create a feature branch
2. Make your changes — keep PRs focused on a single concern
3. Ensure `go build`, `go test`, and `go vet` pass
4. Submit a pull request with a clear description of what and why

---

*SkyTracker Device · Open Source · MIT/Apache 2.0*
*Part of the [SkyTracker](https://skytracker.ai) platform*
