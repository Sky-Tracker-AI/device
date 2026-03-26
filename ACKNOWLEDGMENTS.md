# Acknowledgments

SkyTracker stands on the shoulders of an extraordinary open source community. The ADS-B and satellite reception ecosystem has been built by hobbyists, researchers, and engineers over more than a decade. This project would not exist without their work.

## ADS-B Reception & Decoding

### readsb
- **Project:** [github.com/wiedehopf/readsb](https://github.com/wiedehopf/readsb)
- **Author:** wiedehopf
- **License:** GPL-2.0
- **Usage:** SkyTracker runs readsb as a standalone process to decode Mode S / ADS-B signals from 1090 MHz. The agent reads its JSON output over HTTP. readsb is the core of all aircraft reception on the device.
- **History:** readsb descends from Mutability's fork of [dump1090](https://github.com/MalcolmRobb/dump1090) by Malcolm Robb, itself based on the original [dump1090](https://github.com/antirez/dump1090) by Salvatore Sanfilippo (antirez). The community has iterated on this decoder for over a decade.

### tar1090-db
- **Project:** [github.com/wiedehopf/tar1090-db](https://github.com/wiedehopf/tar1090-db)
- **Author:** wiedehopf
- **License:** GPL-3.0
- **Usage:** Aircraft enrichment database — ICAO hex to type, registration, operator, and LADD/PIA flags. SkyTracker downloads this CSV and auto-updates weekly. The device does not modify or redistribute the database; it fetches it at runtime from the upstream source.

### adsbdb
- **Project:** [adsbdb.com](https://www.adsbdb.com/)
- **Usage:** Flight route lookup API used to resolve callsign to origin/destination when the platform SWIM feed is unavailable.

## Satellite Reception & Decoding

### SatDump
- **Project:** [github.com/SatDump/SatDump](https://github.com/SatDump/SatDump)
- **License:** GPL-3.0
- **Usage:** SkyTracker launches SatDump as a standalone process to decode weather satellite imagery (METEOR-M LRPT, NOAA APT). The agent manages SatDump's lifecycle and reads its decoded output files.

### rtl_tcp / librtlsdr
- **Project:** [github.com/osmocom/rtl-sdr](https://github.com/osmocom/rtl-sdr)
- **License:** GPL-2.0
- **Usage:** rtl_tcp provides a TCP interface to RTL-SDR dongles. SkyTracker uses it as an intermediary between the SDR hardware and SatDump to work around gain-setting issues on RTL-SDR Blog V4 devices.

### CelesTrak
- **Project:** [celestrak.org](https://celestrak.org/)
- **Maintainer:** Dr. T.S. Kelso
- **Usage:** Two-Line Element (TLE) sets for satellite orbit prediction. The agent periodically fetches updated TLEs to compute pass predictions.

## GPS

### gpsd
- **Project:** [gpsd.gitlab.io/gpsd](https://gpsd.gitlab.io/gpsd/)
- **License:** BSD-2-Clause
- **Usage:** Reads NMEA data from USB GPS dongles to determine station position.

## Go Dependencies

| Package | License | Purpose |
|---------|---------|---------|
| [chi](https://github.com/go-chi/chi) | MIT | HTTP router for the local web server |
| [gorilla/websocket](https://github.com/gorilla/websocket) | BSD-2-Clause | WebSocket broadcast to display UI |
| [go-satellite](https://github.com/joshuaferrara/go-satellite) | MIT | SGP4/SDP4 orbital propagation for pass prediction |
| [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) | BSD-3-Clause | Pure-Go SQLite for offline sighting queue |
| [tinygo.org/x/bluetooth](https://github.com/tinygo-org/bluetooth) | BSD-2-Clause | Bluetooth LE for device setup |
| [yaml.v3](https://github.com/go-yaml/yaml) | MIT/Apache-2.0 | Configuration file parsing |

## RTL-SDR Hardware Ecosystem

SkyTracker is designed to work with RTL-SDR dongles — inexpensive software-defined radio receivers originally intended for DVB-T television. The RTL-SDR community repurposed these devices for ADS-B reception, weather satellite decoding, and countless other radio applications. We are grateful to the [RTL-SDR Blog](https://www.rtl-sdr.com/) team and the broader SDR community for making this hardware accessible and well-documented.

---

### A note on GPL and process boundaries

SkyTracker runs readsb, SatDump, and rtl_tcp as **separate standalone processes** and communicates with them over HTTP, TCP, or the filesystem. The SkyTracker agent does not link against, modify, or redistribute any GPL-licensed code. The agent's MIT license applies only to the SkyTracker agent and display UI source code in this repository.

---

If we have missed crediting your project or if any license information is incorrect, please [open an issue](https://github.com/Sky-Tracker-AI/device/issues) and we will fix it promptly.
