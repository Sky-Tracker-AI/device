package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/skytracker/skytracker-device/internal/adsb"
	"github.com/skytracker/skytracker-device/internal/config"
	"github.com/skytracker/skytracker-device/internal/enrichment"
	"github.com/skytracker/skytracker-device/internal/geo"
	"github.com/skytracker/skytracker-device/internal/gpsd"
	"github.com/skytracker/skytracker-device/internal/platform"
	"github.com/skytracker/skytracker-device/internal/queue"
	"github.com/skytracker/skytracker-device/internal/routes"
	"github.com/skytracker/skytracker-device/internal/server"
	"github.com/skytracker/skytracker-device/internal/state"
	"github.com/skytracker/skytracker-device/internal/updater"
	"github.com/skytracker/skytracker-device/internal/wifi"
)

const version = "0.1.4"

func main() {
	var (
		mockMode   = flag.Bool("mock", false, "Run with synthetic aircraft data (no hardware required)")
		configPath = flag.String("config", "", "Path to config file (default: auto-detect)")
		rollback   = flag.Bool("rollback", false, "Rollback to previous agent version")
		showVer    = flag.Bool("version", false, "Print version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("skytracker-agent v%s\n", version)
		os.Exit(0)
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Printf("SkyTracker Agent v%s starting", version)

	if *mockMode {
		log.Printf("Running in MOCK mode — synthetic aircraft data")
	}

	// Handle rollback.
	if *rollback {
		exe, _ := os.Executable()
		u := updater.New(version, exe)
		if err := u.Rollback(); err != nil {
			log.Fatalf("Rollback failed: %v", err)
		}
		log.Printf("Rollback successful. Restart the agent to use the previous version.")
		os.Exit(0)
	}

	// Apply staged OTA update if one was downloaded before this restart.
	{
		exe, _ := os.Executable()
		u := updater.New(version, exe)
		if u.ApplyStaged() {
			log.Printf("OTA update applied — restarting with new binary")
			// Re-exec ourselves so the new binary takes over.
			if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
				log.Printf("Re-exec failed: %v — restart the service manually", err)
			}
		}
	}

	// Load configuration.
	var cfg *config.Config
	var err error
	if *configPath != "" {
		cfg, err = config.LoadFromPath(*configPath)
	} else {
		cfg, err = config.Load()
	}
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Load or create persistent agent state.
	statePath := "/var/lib/skytracker/state.json"
	if *mockMode || os.Getuid() != 0 {
		home, _ := os.UserHomeDir()
		statePath = filepath.Join(home, ".skytracker", "state.json")
	}
	agentState, err := state.Load(statePath)
	if err != nil {
		log.Fatalf("Failed to load state: %v", err)
	}
	log.Printf("Serial: %s (registered: %v, claimed: %v)", agentState.Serial, agentState.IsRegistered(), agentState.Claimed)

	log.Printf("Station: %s (sharing: %s)", cfg.Station.Name, cfg.Station.Sharing)
	log.Printf("Display port: %d", cfg.Display.Port)
	log.Printf("Poll interval: %dms, max range: %dnm", cfg.Advanced.PollIntervalMS, cfg.Advanced.MaxRangeNM)

	// Context for graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Default mock station location: Denver, CO.
	mockLat := 39.8561
	mockLon := -104.6737

	// Use configured station coordinates if available.
	if cfg.Station.Lat != 0 || cfg.Station.Lon != 0 {
		mockLat = cfg.Station.Lat
		mockLon = cfg.Station.Lon
	}

	// --- GPS ---
	var gpsProvider gpsInterface
	if *mockMode {
		gpsProvider = gpsd.NewMockClient(mockLat, mockLon)
		log.Printf("GPS: mock position %.4f, %.4f", mockLat, mockLon)
	} else if cfg.Station.Lat != 0 || cfg.Station.Lon != 0 {
		// Use configured station coordinates as a fixed position (no GPS dongle needed).
		gpsProvider = gpsd.NewMockClient(cfg.Station.Lat, cfg.Station.Lon)
		log.Printf("GPS: using configured position %.4f, %.4f", cfg.Station.Lat, cfg.Station.Lon)
	} else {
		gpsClient := gpsd.NewClient(cfg.Sources.GPSDHost, cfg.Sources.GPSDPort)
		go gpsClient.Run(ctx)
		gpsProvider = gpsClient
		log.Printf("GPS: connecting to %s:%d", cfg.Sources.GPSDHost, cfg.Sources.GPSDPort)
	}

	// --- ADS-B Poller ---
	var aircraftProvider aircraftInterface
	if *mockMode {
		pos := gpsProvider.Position()
		mockPoller := adsb.NewMockPoller(pos.Lat, pos.Lon, cfg.Advanced.PollIntervalMS)
		go mockPoller.Run(ctx)
		aircraftProvider = mockPoller
		log.Printf("ADS-B: mock poller with 15 synthetic aircraft")
	} else {
		poller := adsb.NewPoller(cfg.Sources.Dump1090URL, cfg.Advanced.PollIntervalMS)
		go poller.Run(ctx)
		aircraftProvider = poller
		log.Printf("ADS-B: polling %s every %dms", cfg.Sources.Dump1090URL, cfg.Advanced.PollIntervalMS)
	}

	// --- Enrichment ---
	var enrichAdapter *server.EnrichmentAdapter
	if *mockMode {
		mockEnrich := enrichment.NewMockEngine()
		enrichAdapter = &server.EnrichmentAdapter{
			LookupAircraftFn: func(icaoHex string) *server.AircraftTypeInfo {
				info := mockEnrich.LookupAircraft(icaoHex)
				if info == nil {
					return nil
				}
				return &server.AircraftTypeInfo{
					Registration: info.Registration,
					TypeCode:     info.TypeCode,
					TypeName:     info.TypeName,
					Manufacturer: info.Manufacturer,
					Operator:     info.Operator,
					Owner:        info.Owner,
				}
			},
			LookupAirlineFn: func(callsign string) *server.AirlineNameInfo {
				info := mockEnrich.LookupAirline(callsign)
				if info == nil {
					return nil
				}
				return &server.AirlineNameInfo{
					Name:    info.Name,
					ICAO:    info.ICAO,
					Country: info.Country,
				}
			},
		}
		log.Printf("Enrichment: mock engine with %d types", 15)
	} else {
		dbPath := findEnrichmentDB()
		engine := enrichment.NewEngine(dbPath)
		defer engine.Close()
		enrichAdapter = &server.EnrichmentAdapter{
			LookupAircraftFn: func(icaoHex string) *server.AircraftTypeInfo {
				info := engine.LookupAircraft(icaoHex)
				if info == nil {
					return nil
				}
				return &server.AircraftTypeInfo{
					Registration: info.Registration,
					TypeCode:     info.TypeCode,
					TypeName:     info.TypeName,
					Manufacturer: info.Manufacturer,
					Operator:     info.Operator,
					Owner:        info.Owner,
				}
			},
			LookupAirlineFn: func(callsign string) *server.AirlineNameInfo {
				info := engine.LookupAirline(callsign)
				if info == nil {
					return nil
				}
				return &server.AirlineNameInfo{
					Name:    info.Name,
					ICAO:    info.ICAO,
					Country: info.Country,
				}
			},
		}
		log.Printf("Enrichment: SQLite engine at %s", dbPath)
	}

	// --- WiFi ---
	if *mockMode {
		mockWifi := wifi.NewMockManager()
		go mockWifi.Run(ctx)
		log.Printf("WiFi: mock (simulated connected)")
	} else {
		wifiMgr := wifi.NewManager()
		go wifiMgr.Run(ctx)
		log.Printf("WiFi: manager started")
	}

	// --- Platform Client ---
	// Use API key from state (auto-registration) or config (manual).
	apiKey := agentState.APIKey
	if apiKey == "" {
		apiKey = cfg.Platform.APIKey
	}
	platformClient := platform.NewClient(cfg.Platform.Endpoint, apiKey)

	// Auto-register if not yet registered.
	if !agentState.IsRegistered() {
		pos := gpsProvider.Position()
		hwInfo := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
		regResp, err := platformClient.Register(ctx, platform.RegisterRequest{
			Serial:       agentState.Serial,
			HardwareInfo: hwInfo,
			AgentVersion: version,
			Lat:          pos.Lat,
			Lon:          pos.Lon,
		})
		if err != nil {
			log.Printf("[platform] registration failed (will retry): %v", err)
		} else {
			agentState.DeviceID = regResp.DeviceID
			agentState.APIKey = regResp.APIKey
			agentState.StationID = regResp.StationID
			agentState.ClaimCode = regResp.ClaimCode
			if err := agentState.Save(); err != nil {
				log.Printf("[platform] failed to save state: %v", err)
			}
			// Re-create client with new API key.
			platformClient = platform.NewClient(cfg.Platform.Endpoint, agentState.APIKey)
			log.Printf("[platform] registered: device=%s station=%s claim_code=%s",
				agentState.DeviceID, agentState.StationID, agentState.ClaimCode)
		}
	}
	platformClient.LogConnectivity()

	// --- Claim State Provider ---
	claimProv := &claimStateProviderImpl{
		state:    agentState,
		endpoint: cfg.Platform.Endpoint,
	}

	// --- Offline Queue ---
	queuePath := "/var/lib/skytracker/queue.db"
	if *mockMode || os.Getuid() != 0 {
		home, _ := os.UserHomeDir()
		queuePath = filepath.Join(home, ".skytracker", "queue.db")
	}
	dataQueue, err := queue.New(queuePath, cfg.Advanced.DataQueueMaxMB)
	if err != nil {
		log.Printf("Warning: offline queue unavailable: %v", err)
	} else {
		defer dataQueue.Close()
		log.Printf("Queue: %s (max %d MB)", queuePath, cfg.Advanced.DataQueueMaxMB)
	}

	// --- OTA Updater ---
	exe, _ := os.Executable()
	otaUpdater := updater.New(version, exe)
	if !*mockMode {
		go otaUpdater.Run(ctx)
		log.Printf("Updater: checking GitHub releases daily")
	}

	// --- Route Lookup ---
	routeLookup := routes.New()
	routeAdapter := &routeAdapterImpl{lookup: routeLookup}
	log.Printf("Routes: adsbdb.com lookup enabled")

	// --- HTTP + WebSocket Server ---
	uiDir := findUIDir()
	gpsAdapter := &gpsAdapterImpl{gps: gpsProvider}
	srv := server.NewServer(
		cfg.Display.Port,
		cfg.Station.Name,
		uiDir,
		cfg.Advanced.MaxRangeNM,
		cfg.Advanced.PollIntervalMS,
		aircraftProvider,
		gpsAdapter,
		enrichAdapter,
		routeAdapter,
		claimProv,
	)

	go func() {
		if err := srv.Run(ctx); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()

	// --- Platform Sync (background) ---
	if platformClient.IsConfigured() && dataQueue != nil {
		go runPlatformSync(ctx, platformClient, aircraftProvider, gpsProvider, dataQueue, cfg, agentState, claimProv, srv, enrichAdapter)
	}

	log.Printf("SkyTracker Agent ready — http://localhost:%d", cfg.Display.Port)

	// Wait for shutdown signal.
	sig := <-sigCh
	log.Printf("Received signal %v, shutting down...", sig)
	cancel()

	// Give goroutines time to clean up.
	time.Sleep(2 * time.Second)
	log.Printf("SkyTracker Agent stopped")
}

// aircraftInterface abstracts over Poller and MockPoller.
type aircraftInterface interface {
	Aircraft() []adsb.Aircraft
}

// gpsInterface abstracts over gpsd.Client and gpsd.MockClient.
type gpsInterface interface {
	Position() gpsd.Position
	Run(ctx context.Context)
}

// gpsAdapterImpl adapts gpsInterface to server.GPSProvider.
type gpsAdapterImpl struct {
	gps gpsInterface
}

func (g *gpsAdapterImpl) Position() server.StationPosition {
	pos := g.gps.Position()
	return server.StationPosition{
		Lat:    pos.Lat,
		Lon:    pos.Lon,
		HasFix: pos.HasFix,
	}
}

// findUIDir looks for the ui/ directory relative to the binary or working dir.
func findUIDir() string {
	candidates := []string{
		"ui",
		"./ui",
		"/opt/skytracker/ui",
	}

	// Also check relative to the binary location.
	if exe, err := os.Executable(); err == nil {
		candidates = append([]string{filepath.Join(filepath.Dir(exe), "ui")}, candidates...)
	}

	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			abs, _ := filepath.Abs(dir)
			log.Printf("UI directory: %s", abs)
			return dir
		}
	}

	log.Printf("Warning: ui/ directory not found, serving empty directory")
	return "ui"
}

// findEnrichmentDB looks for the enrichment database.
func findEnrichmentDB() string {
	candidates := []string{
		"data/enrichment.db",
		"./data/enrichment.db",
		"/opt/skytracker/data/enrichment.db",
	}

	if exe, err := os.Executable(); err == nil {
		candidates = append([]string{filepath.Join(filepath.Dir(exe), "data", "enrichment.db")}, candidates...)
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			abs, _ := filepath.Abs(path)
			return abs
		}
	}

	return "data/enrichment.db"
}

// routeAdapterImpl adapts routes.Lookup to server.RouteLookup.
type routeAdapterImpl struct {
	lookup *routes.Lookup
}

func (r *routeAdapterImpl) Get(callsign string) *server.RouteInfo {
	route := r.lookup.Get(callsign)
	if route == nil {
		return nil
	}
	return &server.RouteInfo{
		Origin:      route.Origin,
		Destination: route.Destination,
	}
}

// claimStateProviderImpl wraps agent state to implement server.ClaimStateProvider.
type claimStateProviderImpl struct {
	mu       sync.RWMutex
	state    *state.State
	endpoint string
}

func (c *claimStateProviderImpl) ClaimState() server.ClaimState {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cs := server.ClaimState{
		Registered:  c.state.IsRegistered(),
		ClaimCode:   c.state.ClaimCode,
		Claimed:     c.state.Claimed,
	}
	if c.state.ClaimCode != "" {
		// Derive frontend URL from API endpoint (api.skytracker.ai → skytracker.ai).
		frontendURL := strings.Replace(c.endpoint, "://api.", "://", 1)
		cs.ClaimURL = frontendURL + "/claim?code=" + c.state.ClaimCode
	}
	return cs
}

// runPlatformSync periodically syncs aircraft data to skytracker.ai.
func runPlatformSync(ctx context.Context, client *platform.Client, ap aircraftInterface, gps gpsInterface, q *queue.Queue, cfg *config.Config, agentState *state.State, claimProv *claimStateProviderImpl, srv *server.Server, enricher *server.EnrichmentAdapter) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Poll health more frequently while unclaimed (30s vs 5m).
	healthInterval := 30 * time.Second
	if agentState.Claimed {
		healthInterval = 5 * time.Minute
	}
	healthTicker := time.NewTicker(healthInterval)
	defer healthTicker.Stop()

	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Build sightings from current aircraft.
			aircraft := ap.Aircraft()
			pos := gps.Position()

			if len(aircraft) == 0 {
				continue
			}

			sightings := make([]platform.IngestSighting, 0, len(aircraft))
			for _, a := range aircraft {
				if !a.HasPosition() {
					continue
				}
				dist := geo.HaversineNM(pos.Lat, pos.Lon, *a.Lat, *a.Lon)
				typeCode := ""
				if enricher != nil {
					if info := enricher.LookupAircraft(a.Hex); info != nil {
						typeCode = info.TypeCode
					}
				}
				sightings = append(sightings, platform.IngestSighting{
					Timestamp: time.Now().UnixMilli(),
					ICAOHex:   a.Hex,
					Callsign:  a.Callsign(),
					Type:      typeCode,
					Altitude:  a.Altitude(),
					Speed:     a.Speed(),
					Heading:   a.Heading(),
					Lat:       *a.Lat,
					Lon:       *a.Lon,
					Distance:  dist,
					VertRate:  a.VertRate(),
					Squawk:    a.Squawk,
				})
			}

			// Try to ingest.
			_, err := client.Ingest(ctx, platform.IngestRequest{Sightings: sightings})
			if err != nil {
				// Queue for later.
				queueSightings := make([]queue.Sighting, 0, len(sightings))
				for _, s := range sightings {
					queueSightings = append(queueSightings, queue.Sighting{
						Timestamp: time.Now(),
						ICAOHex:   s.ICAOHex,
						Callsign:  s.Callsign,
						Type:      s.Type,
						Altitude:  s.Altitude,
						Speed:     s.Speed,
						Heading:   s.Heading,
						Lat:       s.Lat,
						Lon:       s.Lon,
						Distance:  s.Distance,
					})
				}
				q.Enqueue(queueSightings)
				continue
			}

			// If ingest succeeded, also try to drain the queue.
			queued, err := q.Dequeue(500)
			if err == nil && len(queued) > 0 {
				queuedSightings := make([]platform.IngestSighting, 0, len(queued))
				for _, s := range queued {
					queuedSightings = append(queuedSightings, platform.IngestSighting{
						Timestamp: s.Timestamp.UnixMilli(),
						ICAOHex:   s.ICAOHex,
						Callsign:  s.Callsign,
						Type:      s.Type,
						Altitude:  s.Altitude,
						Speed:     s.Speed,
						Heading:   s.Heading,
						Lat:       s.Lat,
						Lon:       s.Lon,
						Distance:  s.Distance,
					})
				}
				client.Ingest(ctx, platform.IngestRequest{Sightings: queuedSightings})
			}

		case <-healthTicker.C:
			aircraft := ap.Aircraft()
			pos := gps.Position()
			resp, err := client.Health(ctx, platform.HealthRequest{
				Uptime:        int64(time.Since(startTime).Seconds()),
				GPSFix:        pos.HasFix,
				Lat:           pos.Lat,
				Lon:           pos.Lon,
				AircraftCount: len(aircraft),
				AgentVersion:  version,
				LastSync:      time.Now().Unix(),
				QueueSize:     q.Count(),
			})
			if err != nil {
				continue
			}

			// Sync station name from platform (picks up renames too).
			if resp.StationName != "" {
				srv.SetStationName(resp.StationName)
			}

			// Detect when device gets claimed.
			if resp.Claimed && !agentState.Claimed {
				log.Printf("[platform] device claimed! station_name=%s", resp.StationName)
				claimProv.mu.Lock()
				agentState.Claimed = true
				agentState.ClaimCode = "" // no longer needed
				agentState.Save()
				claimProv.mu.Unlock()

				// Switch to slower health polling now that we're claimed.
				healthTicker.Reset(5 * time.Minute)
			}
		}
	}
}

