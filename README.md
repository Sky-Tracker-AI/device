<p align="center">
  <img src="logo.png" alt="SkyTracker" width="120" />
</p>

<h1 align="center">SkyTracker Device</h1>

<p align="center">
Open-source on-device software that turns any Linux single-board computer with an RTL-SDR receiver into a real-time ADS-B aircraft tracker with a live radar display. Part of the <a href="https://skytracker.ai">SkyTracker</a> platform.
</p>

## Quick Start

### One-Line Install

```bash
curl -sSL https://get.skytracker.ai | sudo bash
```

This installs dump1090-fa, GPS support, the SkyTracker agent, and starts everything as a systemd service. Your station location is auto-detected from IP. See [skytracker.ai/setup](https://skytracker.ai/setup) for the full guide.

### Manual Install

```bash
# Install system dependencies
sudo apt install dump1090-fa gpsd sqlite3

# Download the latest agent binary and UI
curl -L https://github.com/Sky-Tracker-AI/device/releases/latest/download/skytracker-agent-linux-arm64 \
    -o /usr/local/bin/skytracker-agent
chmod +x /usr/local/bin/skytracker-agent

# Start the agent
skytracker-agent --config /etc/skytracker/config.yaml
```

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
│   └── mock-dump1090/     # Mock data generator for development
├── internal/              # Agent internals
│   ├── adsb/              # ADS-B polling (dump1090-fa)
│   ├── config/            # YAML configuration
│   ├── enrichment/        # Aircraft type + airline lookup (SQLite)
│   ├── geo/               # Haversine distance, bearing
│   ├── gpsd/              # GPS daemon client
│   ├── platform/          # skytracker.ai API client
│   ├── queue/             # Offline sighting queue (SQLite)
│   ├── routes/            # Flight route lookup (adsbdb.com)
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

| Component | Example | Cost |
|-----------|---------|------|
| RTL-SDR Blog V4 + Antenna Kit | R828D tuner, dipole antenna | ~$40 |
| Raspberry Pi 4/5 (2GB+) | Any Linux SBC with USB | ~$75 |
| 7" IPS Display (1024x600) | Optional — for radar display | ~$35 |
| USB GPS Dongle | Optional — for auto-positioning | ~$18 |

Any Linux SBC with USB works. Raspberry Pi is recommended but not required. A display is optional — the agent works headless and sends data to [skytracker.ai/map](https://skytracker.ai/map).

## Contributing

We welcome contributions! Here's how to get started.

### Setting Up

1. Fork the repo and clone your fork
2. Install [Go 1.21+](https://go.dev/dl/)
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

### Code of Conduct

Be respectful. We're all here because we like watching planes.

## License

[MIT](LICENSE)
