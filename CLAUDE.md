# SkyTracker Device — Claude Instructions

## Releases

- **Always bump the version constant** in `cmd/agent/main.go` (`const version = "X.Y.Z"`) before creating a git tag. The version in the code must match the tag name (without the `v` prefix). The OTA updater compares this value against GitHub releases.
- Tag format: `vX.Y.Z` (e.g., `v0.4.0`)
- Pushing a tag triggers the GitHub Actions release workflow which builds the ARM64 binary.

## Build

```bash
# Native (dev/test)
go build ./...

# Cross-compile for Raspberry Pi
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o skytracker-agent-arm64 ./cmd/agent/

# Deploy to a Pi
scp skytracker-agent-arm64 pi@<host>:/tmp/skytracker-agent
ssh pi@<host> "sudo systemctl stop skytracker && sudo cp /tmp/skytracker-agent /usr/local/bin/skytracker-agent && sudo chmod +x /usr/local/bin/skytracker-agent && sudo systemctl start skytracker"
```

## Mock Mode

Mock mode (`--mock` flag) is **disabled from platform sync** — it will not register or send data to production. This is intentional to prevent test data from polluting the live database.
