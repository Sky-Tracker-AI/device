<p align="center">
  <img src="logo.png" alt="SkyTracker" width="120" />
</p>

<h1 align="center">SkyTracker Device</h1>

<p align="center">
Open-source on-device software that turns any Linux single-board computer with RTL-SDR receivers into a real-time ADS-B aircraft tracker and weather satellite receiver. Part of the <a href="https://skytracker.ai">SkyTracker</a> platform.
</p>

## Quick Start

### One-Line Install

```bash
curl -sSL https://get.skytracker.ai | sudo bash
```

This installs readsb (with RTL-SDR support), SatDump, GPS support, the SkyTracker agent, and starts everything as a systemd service. It also sets up a Chromium kiosk for the display and downloads the [tar1090-db](https://github.com/wiedehopf/tar1090-db) aircraft database. Your station location is auto-detected from IP. See [skytracker.ai/setup](https://skytracker.ai/setup) for the full guide.

### Manual Install

```bash
# Download the latest agent binary
curl -L https://github.com/Sky-Tracker-AI/device/releases/latest/download/skytracker-agent-linux-arm64 \
    -o /usr/local/bin/skytracker-agent
chmod +x /usr/local/bin/skytracker-agent

# Download the aircraft database
mkdir -p /opt/skytracker/data
curl -L https://raw.githubusercontent.com/wiedehopf/tar1090-db/csv/aircraft.csv.gz \
    -o /opt/skytracker/data/aircraft.csv.gz

# Start the agent
skytracker-agent --config /etc/skytracker/config.yaml
```

## Onboarding

New devices are onboarded via BLE from the SkyTracker iOS app.

### How it works

1. **Power on** — the device boots and starts advertising via BLE as `SkyTracker-XXXXXX`
2. **Connect** — the app discovers the device and connects over Bluetooth
3. **WiFi** — the app scans for nearby networks, the user picks one and enters the password. The app sends the credentials over BLE and the device connects.
4. **Registration** — the device auto-registers with the SkyTracker platform (no API key or manual config needed)
5. **Claim** — the app reads a claim code from the device and submits it to the platform. The user's account is linked to the station.

After onboarding, the device runs autonomously — syncing aircraft data, satellite observations, and health reports to the platform.

### What gets auto-detected

The device figures out its capabilities from hardware at startup. No manual signal type selection is needed during onboarding.

| Capability | How it's detected | Config needed? |
|---|---|---|
| **ADS-B** | readsb systemd service is running, claims one SDR | No — pre-configured at OS image level |
| **Weather satellites** | Remaining SDRs after readsb + ACARS get the scheduler | No — runs automatically if SDRs are available |
| **Inmarsat ACARS** | `omni.acars.enabled: true` in config.yaml | **Yes — manual config today** |

The device reports its active signal types (e.g. `["adsb", "satellite", "acars"]`) to the platform via health reports. The platform uses this to show the correct tabs and capabilities on the station profile.

### What's not in onboarding yet

- **L-band mode selection** — choosing between Inmarsat ACARS and Iridium during setup. Currently requires editing `config.yaml`.
- **Satellite target selection** — choosing which Inmarsat satellite to point at. Currently defaults to Inmarsat-4 F3 (Americas) via config.
- **SDR role assignment** — the device allocates SDRs automatically by convention. There's no UI to reassign them.

These will be added as a BLE characteristic in a future release so the iOS app can configure signal types during onboarding.

### Onboarding via SSH

If you don't have the iOS app or prefer to set up manually, you can onboard a device entirely over SSH.

**1. Connect to the Pi:**

```bash
ssh pi@skytracker.local
```

**2. Set your WiFi (if not already connected):**

```bash
sudo nmcli dev wifi connect "YourNetwork" password "YourPassword"
```

**3. Edit the config:**

```bash
sudo nano /etc/skytracker/config.yaml
```

Set your station location and any signal type options:

```yaml
station:
  name: "My Station"
  sharing: public
  lat: 30.2672
  lon: -97.7431

omni:
  acars:
    enabled: true              # Enable if you have an L-band SDR + patch antenna
    satellite: "inmarsat4-f3"  # Americas; use inmarsat4-f1 (APAC) or inmarsat4-f2 (EMEA)
```

**4. Restart the agent:**

```bash
sudo systemctl restart skytracker-agent
```

**5. Check the logs:**

```bash
journalctl -u skytracker-agent -f
```

You should see the agent start, detect SDRs, and begin syncing. Look for lines like:

```
[omni] detected 2 total SDR(s), 1 available for omni, mode=adsb_omni
[acars] Inmarsat decoder started on SDR SKT-OMNI-0 (freq=1545.0 MHz)
```

**6. Claim your station:**

The agent auto-registers on first boot. The claim code is in the state file:

```bash
sudo cat /var/lib/skytracker/state.json | python3 -m json.tool
```

Go to [skytracker.ai/claim](https://skytracker.ai/claim) and enter the claim code to link the station to your account.

## Development

No SBC or SDR hardware required. The agent includes a mock mode with synthetic aircraft data.

```bash
# Clone and build
git clone https://github.com/Sky-Tracker-AI/device.git
cd device
go build -o skytracker-agent ./cmd/agent

# Run with synthetic aircraft (no hardware needed)
./skytracker-agent --mock

# Open the display in a browser
open http://localhost:8080
```

### Cross-Compile for Raspberry Pi

```bash
GOOS=linux GOARCH=arm64 go build -o skytracker-agent-arm64 ./cmd/agent
```

### UI Development

The display UI is plain HTML, CSS, and JavaScript — no framework, no build step. Edit files in `ui/` and refresh the browser.

## Project Structure

```
device/
├── cmd/
│   ├── agent/             # Main agent entrypoint
│   └── mock-dump1090/     # Mock ADS-B JSON server for development
├── internal/              # Agent internals
│   ├── acars/             # Inmarsat L-band ACARS decoder, parser, AES lookup
│   ├── adsb/              # ADS-B polling (readsb JSON)
│   ├── config/            # YAML configuration
│   ├── enrichment/        # Aircraft type + airline lookup (tar1090-db CSV)
│   ├── geo/               # Haversine distance, bearing
│   ├── gpsd/              # GPS daemon client
│   ├── omni/              # Satellite catalog and categories
│   ├── platform/          # skytracker.ai API client
│   ├── queue/             # Offline sighting queue (SQLite)
│   ├── routes/            # Flight route lookup (adsbdb.com)
│   ├── sat/               # TLE fetcher, SGP4 pass predictor
│   ├── satellite/         # SatDump decoder, pipeline config, post-pass reporter
│   ├── scheduler/         # SDR time-sharing across satellite passes
│   ├── sdr/               # RTL-SDR detection, serial programming, readsb interop
│   ├── server/            # HTTP + WebSocket server
│   ├── state/             # Persistent agent state
│   ├── updater/           # OTA update from GitHub Releases
│   └── wifi/              # WiFi management
├── ui/                    # Display UI (static HTML/CSS/JS + Canvas)
├── scripts/
│   ├── install.sh         # One-line installer
│   └── kiosk.sh           # Chromium kiosk mode launcher
├── configs/
│   └── config.example.yaml
└── LICENSE
```

## Hardware

### Core

| Component | Example | Cost |
|-----------|---------|------|
| Raspberry Pi 4 or 5 (2GB+) | Any Linux SBC with USB | ~$45–75 |
| RTL-SDR Blog V4 + Antenna Kit | R828D tuner, 1090 MHz antenna (ADS-B) | ~$40 |
| 7" IPS Display (1024x600) | Optional — for radar display | ~$35 |
| USB GPS Dongle | Optional — for auto-positioning | ~$18 |

A single RTL-SDR handles ADS-B. A display is optional — the agent works headless and sends data to [skytracker.ai/map](https://skytracker.ai/map).

### Add-On: Weather Satellite Reception

| Component | Example | Cost |
|-----------|---------|------|
| RTL-SDR Blog V4 + V-Dipole | R828D tuner, V-dipole antenna (137 MHz) | ~$40 |

A second SDR with a V-dipole receives METEOR-M LRPT weather satellite imagery. The scheduler time-shares the SDR across passes automatically.

### Add-On: Inmarsat L-Band ACARS

| Component | Example | Cost |
|-----------|---------|------|
| RTL-SDR Blog V4 | Dedicated to L-band (1545 MHz) | ~$30 |
| Wideband L-band patch antenna | 1525–1627 MHz, pointed at Inmarsat satellite | ~$15–25 |

A third SDR with an L-band patch antenna decodes Inmarsat ACARS — aircraft position reports, dispatch messages, and maritime safety broadcasts from geostationary satellites. This fills oceanic gaps where ADS-B ground stations can't reach. The antenna must be pointed at an Inmarsat-4 satellite (e.g., Inmarsat-4 F3 at 98°W for the Americas). See [Inmarsat ACARS Setup](#inmarsat-acars-setup) below.

### Raspberry Pi 4 vs 5

Either works. Pick based on what you're running:

| | Pi 4 (2GB+) | Pi 5 (4GB+) |
|---|---|---|
| **ADS-B only** | More than enough | Overkill |
| **ADS-B + satellite** | Works well | Slightly faster SatDump decoding |
| **ADS-B + satellite + Inmarsat ACARS** | Works — budget ~25% CPU total | Headroom for future signal types |
| **Multiple SDRs (3+)** | Fine for 3 SDRs | Better USB controller helps with 4+ |
| **Display (kiosk)** | Chromium + Canvas is smooth | Noticeably snappier UI rendering |
| **Price** | ~$45 (2GB) | ~$60 (4GB), ~$75 (8GB) |

**Recommendation:** Pi 4 (2GB) is the sweet spot for most stations. Go Pi 5 if you plan to run 3+ SDRs or want the fastest possible SatDump decoding.

## Inmarsat ACARS Setup

Inmarsat ACARS decodes aircraft messages from geostationary L-band satellites — position reports over the ocean, airline dispatch, military routing, and maritime safety broadcasts. Unlike weather satellites (scheduled passes), Inmarsat runs 24/7 on a dedicated SDR.

### What you need

1. An RTL-SDR Blog V4 (or any RTL-SDR with R828D/R820T tuner)
2. A wideband L-band patch antenna (1525–1627 MHz)
3. The antenna pointed at an Inmarsat-4 satellite

### Target satellites

| Satellite | Position | Coverage |
|-----------|----------|----------|
| Inmarsat-4 F3 | 98°W | Americas, Atlantic, Gulf of Mexico, Caribbean |
| Inmarsat-4 F1 | 143.5°E | Asia-Pacific |
| Inmarsat-4 F2 | 63.9°E | EMEA, Indian Ocean |

Most US-based stations should point at **Inmarsat-4 F3** for Gulf, Caribbean, and transatlantic coverage.

### Configuration

Add to your `config.yaml`:

```yaml
omni:
  acars:
    enabled: true
    satellite: "inmarsat4-f3"
    frequency_mhz: 1545.0
```

The agent auto-detects and reserves one SDR for ACARS. It prefers R828D tuners (best L-band sensitivity). The remaining SDRs stay in the satellite scheduler pool.

### What you receive

- **Position reports** — Aircraft lat/lon, altitude, heading, speed, ETA over oceanic routes
- **Dispatch messages** — Gate changes, loadsheets, fuel data, crew scheduling
- **Weather delivery** — METAR/TAF reports transmitted to cockpits
- **OOOI events** — Pushback, wheels-up, touchdown, gate arrival timestamps
- **Military dispatch** — Tanker/transport routing commands (e.g., RCH callsign prefixes)
- **SAR alerts** — Coast guard search and rescue broadcasts (EGC/STD-C)
- **Navigation warnings** — Hazards to shipping, military exercise zones

### Development (no hardware)

Mock mode generates synthetic Inmarsat ACARS traffic:

```bash
# Enable ACARS in your config, then:
./skytracker-agent --mock
```

The mock decoder produces realistic position reports, dispatch messages, and EGC broadcasts from a fleet of 10 synthetic aircraft over Gulf/Atlantic routes.

## Aircraft Data & Privacy

Aircraft enrichment (type, registration, operator) comes from [tar1090-db](https://github.com/wiedehopf/tar1090-db), a community-maintained database of 619K+ aircraft. The database auto-updates weekly.

### LADD Compliance

The agent respects FAA **LADD** (Limiting Aircraft Data Displayed) flags. LADD-flagged aircraft:

- **Are shown** on the local radar display (position data is received over the air via ADS-B and is not subject to LADD)
- **Have identifying info suppressed** — registration, owner, and operator are stripped from WebSocket broadcasts and platform ingest
- Type code and type name are shown (describes the aircraft model, not the specific tail)

**PIA** (Privacy ICAO Address) aircraft have their registration stripped at load time.

## Contributing

We welcome contributions! Here's how to get started.

### Setting Up

1. Fork the repo and clone your fork
2. Install [Go 1.23+](https://go.dev/dl/)
3. Run `go build ./cmd/agent` to verify the build
4. Run `./skytracker-agent --mock` to start with synthetic data
5. Open `http://localhost:8080` to see the display UI

### Making Changes

- **Go code** follows standard Go conventions. Run `go vet ./...` before submitting.
- **UI code** is vanilla JS — no framework, no build step. Keep it simple and fast.
- **SCSS/CSS** uses `rem` units only, never `px` (except root font-size).
- Keep PRs focused. One feature or fix per PR.

### What to Work On

- Check [open issues](https://github.com/Sky-Tracker-AI/device/issues) for bugs and feature requests
- Issues labeled `good first issue` are great starting points
- If you want to work on something bigger, open an issue first to discuss

### Pull Request Process

1. Create a feature branch from `main`
2. Make your changes with clear commit messages
3. Test with `--mock` mode (no hardware needed)
4. Run `go vet ./...` and fix any warnings
5. Open a PR against `main` with a description of what changed and why

### Architecture Notes

- **Agent** (`cmd/agent/main.go`) — orchestrates all components, runs background sync
- **Server** (`internal/server/`) — HTTP + WebSocket server, broadcasts enriched aircraft data to the display UI
- **Platform client** (`internal/platform/`) — communicates with skytracker.ai (registration, ingest, health)
- **State** (`internal/state/`) — persists device identity and claim status across restarts
- **Display UI** (`ui/`) — Canvas-based radar, aircraft list, setup/claim screen. No build tools.

The agent auto-registers with skytracker.ai on first boot. No API key or manual configuration is needed — users claim their device by scanning a QR code shown on the display.

- **Enrichment** (`internal/enrichment/`) — loads tar1090-db CSV (619K aircraft), applies LADD/PIA suppression, auto-updates weekly from GitHub
- **OTA updater** (`internal/updater/`) — checks GitHub Releases daily, stages updates with SHA256 verification, applies on restart
- **Satellite scheduler** (`internal/scheduler/`) — time-shares idle SDRs across satellite passes with priority-based preemption
- **SatDump decoder** (`internal/satellite/`) — launches rtl_tcp + SatDump to receive and decode METEOR-M LRPT weather imagery, reports observations to the platform
- **ACARS decoder** (`internal/acars/`) — continuous Inmarsat L-band ACARS decoding on a dedicated SDR, with message classification, PII redaction, and batched platform sync

### Code of Conduct

Be respectful. We're all here because we like watching planes.

## License

[MIT](LICENSE)
