# SkyTracker Device

Open-source on-device software that turns any Linux single-board computer with an RTL-SDR receiver into a real-time ADS-B aircraft tracker with a live radar display. Part of the [SkyTracker](https://skytracker.ai) platform.

See [PRD.md](PRD.md) for the full product requirements document.

## Quick Start

### Option 1: Pre-Built SD Image

1. Download the latest SkyTracker SD image from [GitHub Releases](https://github.com/skytracker/skytracker-device/releases)
2. Flash to an SD card with [Raspberry Pi Imager](https://www.raspberrypi.com/software/) or [balenaEtcher](https://www.balena.io/etcher)
3. Insert SD card, plug in RTL-SDR, GPS dongle, and display
4. Power on — see planes within 60 seconds

### Option 2: Install Script

```bash
curl -fsSL https://raw.githubusercontent.com/skytracker/skytracker-device/main/scripts/install.sh | sudo bash
```

### Option 3: Manual Install

```bash
# Install system dependencies
sudo apt install dump1090-fa gpsd chromium-browser sqlite3

# Download the latest agent binary and UI
curl -L https://github.com/skytracker/skytracker-device/releases/latest/download/skytracker-agent-arm64 \
    -o /usr/local/bin/skytracker-agent
chmod +x /usr/local/bin/skytracker-agent

curl -L https://github.com/skytracker/skytracker-device/releases/latest/download/skytracker-ui.tar.gz \
    | tar xz -C /opt/skytracker/ui

# Start the agent
skytracker-agent --config /etc/skytracker/config.yaml
```

## Development Setup

No SBC or SDR hardware required. The agent includes a mock mode for development.

```bash
# Clone and build
git clone https://github.com/skytracker/skytracker-device.git
cd skytracker-device
go build -o skytracker-agent ./cmd/agent

# Run with synthetic aircraft data (no hardware needed)
./skytracker-agent --mock

# Or run just the mock dump1090 server
go run ./cmd/mock-dump1090

# Open the UI in a browser
open http://localhost:8080
```

### UI Development

The Display UI is plain HTML, CSS, and JavaScript — no framework, no build step.

```bash
cd ui/
python3 -m http.server 8080
# Edit files, refresh browser
```

### Cross-Compile for Raspberry Pi

```bash
GOOS=linux GOARCH=arm64 go build -o skytracker-agent-arm64 ./cmd/agent
```

## Project Structure

```
skytracker-device/
├── cmd/
│   ├── agent/             # Main agent entrypoint
│   └── mock-dump1090/     # Mock data generator for development
├── internal/              # Agent internals (adsb, config, geo, gpsd, etc.)
├── ui/                    # Display UI (static HTML/CSS/JS)
├── data/
│   └── enrichment.db      # SQLite: ICAO hex → type, callsign → airline
├── scripts/
│   ├── install.sh         # System setup script
│   └── kiosk.sh           # Chromium kiosk mode launcher
├── configs/
│   └── config.example.yaml
├── PRD.md                 # Full product requirements document
├── LICENSE                # MIT
└── README.md
```

## Hardware

| Component | Example | Cost |
|-----------|---------|------|
| RTL-SDR Blog V4 + Antenna Kit | R828D tuner, dipole antenna | ~$40 |
| Raspberry Pi 4 (2GB+) | CanaKit starter | ~$75 |
| 7" IPS Display (1024x600) | GeeekPi | ~$35 |
| USB GPS Dongle | Geekstory BN-808 | ~$18 |
| **Total** | | **~$168** |

Any Linux SBC with USB works. Raspberry Pi is recommended but not required.

## Contributing

See [PRD.md, Section 14](PRD.md#14-contributing) for development environment setup, code standards, and contribution guidelines.

## License

[MIT](LICENSE)
