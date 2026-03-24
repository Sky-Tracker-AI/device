package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/skytracker/skytracker-device/internal/adsb"
	"github.com/skytracker/skytracker-device/internal/ble"
	"github.com/skytracker/skytracker-device/internal/config"
	"github.com/skytracker/skytracker-device/internal/enrichment"
	"github.com/skytracker/skytracker-device/internal/geo"
	"github.com/skytracker/skytracker-device/internal/gpsd"
	"github.com/skytracker/skytracker-device/internal/platform"
	"github.com/skytracker/skytracker-device/internal/queue"
	"github.com/skytracker/skytracker-device/internal/routes"
	"github.com/skytracker/skytracker-device/internal/sat"
	"github.com/skytracker/skytracker-device/internal/satellite"
	"github.com/skytracker/skytracker-device/internal/scheduler"
	"github.com/skytracker/skytracker-device/internal/sdr"
	"github.com/skytracker/skytracker-device/internal/server"
	"github.com/skytracker/skytracker-device/internal/state"
	"github.com/skytracker/skytracker-device/internal/updater"
	"github.com/skytracker/skytracker-device/internal/wifi"
)

const version = "0.6.0"

func main() {
	var (
		mockMode   = flag.Bool("mock", false, "Run with synthetic aircraft data (no hardware required)")
		configPath = flag.String("config", "", "Path to config file (default: auto-detect)")
		rollback   = flag.Bool("rollback", false, "Rollback to previous agent version")
		showVer    = flag.Bool("version", false, "Print version and exit")
		pairMode   = flag.Bool("pair", false, "Trigger BLE pairing mode on running agent")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("skytracker-agent v%s\n", version)
		os.Exit(0)
	}

	// --pair: send SIGUSR1 to the running agent and exit.
	if *pairMode {
		out, err := exec.Command("pidof", "skytracker-agent").Output()
		if err != nil {
			fmt.Fprintln(os.Stderr, "No running skytracker-agent found")
			os.Exit(1)
		}
		pidStr := strings.TrimSpace(string(out))
		// pidof may return multiple PIDs; take the first that isn't us.
		for _, p := range strings.Fields(pidStr) {
			pid := 0
			fmt.Sscanf(p, "%d", &pid)
			if pid > 0 && pid != os.Getpid() {
				proc, err := os.FindProcess(pid)
				if err == nil {
					proc.Signal(syscall.SIGUSR1)
					fmt.Printf("BLE pairing mode activated (PID %d) — advertising for 5 minutes\n", pid)
					os.Exit(0)
				}
			}
		}
		fmt.Fprintln(os.Stderr, "Could not signal running agent")
		os.Exit(1)
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Printf("SkyTracker Agent v%s starting", version)

	if *mockMode {
		log.Printf("Running in MOCK mode — synthetic aircraft data")
	}

	// Resolve our own binary path (needed for OTA and rollback).
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("Cannot determine executable path: %v", err)
	}

	// Handle rollback.
	if *rollback {
		u := updater.New(version, exe)
		if err := u.Rollback(); err != nil {
			log.Fatalf("Rollback failed: %v", err)
		}
		log.Printf("Rollback successful. Restart the agent to use the previous version.")
		os.Exit(0)
	}

	// Apply staged OTA update if one was downloaded before this restart.
	{
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
	log.Printf("Serial: %s (registered: %v, claimed: %v)", agentState.GetSerial(), agentState.IsRegistered(), agentState.GetClaimed())

	log.Printf("Station: %s (sharing: %s)", cfg.Station.Name, cfg.Station.Sharing)
	log.Printf("Display port: %d", cfg.Display.Port)
	log.Printf("Poll interval: %dms, max range: %dnm", cfg.Advanced.PollIntervalMS, cfg.Advanced.MaxRangeNM)

	// Context for graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)

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
	var enrichEngine interface {
		LookupAircraft(string) *enrichment.AircraftInfo
		LookupAirline(string) *enrichment.AirlineInfo
		Close()
	}
	if *mockMode {
		enrichEngine = enrichment.NewMockEngine()
		log.Printf("Enrichment: mock engine")
	} else {
		csvPath := findAircraftCSV()
		engine := enrichment.NewEngine(csvPath)
		defer engine.Close()
		enrichEngine = engine

		// Start background CSV updater.
		csvUpdater := enrichment.NewCSVUpdater(engine, filepath.Dir(csvPath))
		go csvUpdater.Run(ctx)
		log.Printf("Enrichment: tar1090-db CSV at %s", csvPath)
	}
	enrichAdapter := &server.EnrichmentAdapter{
		LookupAircraftFn: func(icaoHex string) *server.AircraftTypeInfo {
			info := enrichEngine.LookupAircraft(icaoHex)
			if info == nil {
				return nil
			}
			return &server.AircraftTypeInfo{
				Registration: info.Registration,
				TypeCode:     info.TypeCode,
				TypeName:     info.TypeName,
				Operator:     info.Operator,
				Owner:        info.Owner,
				Year:         info.Year,
				LADD:         info.LADD,
				PIA:          info.PIA,
				Military:     info.Military,
			}
		},
		LookupAirlineFn: func(callsign string) *server.AirlineNameInfo {
			info := enrichEngine.LookupAirline(callsign)
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

	// --- WiFi ---
	var wifiMgr *wifi.Manager
	if *mockMode {
		mockWifi := wifi.NewMockManager()
		go mockWifi.Run(ctx)
		log.Printf("WiFi: mock (simulated connected)")
	} else {
		wifiMgr = wifi.NewManager()
		go wifiMgr.Run(ctx)
		log.Printf("WiFi: manager started")
	}

	// --- Platform Client ---
	// Use API key from state (auto-registration) or config (manual).
	apiKey := agentState.GetAPIKey()
	if apiKey == "" {
		apiKey = cfg.Platform.APIKey
	}
	platHolder := &platformClientHolder{client: platform.NewClient(cfg.Platform.Endpoint, apiKey)}

	// Auto-register if not yet registered. Never register mock stations with production.
	if *mockMode {
		log.Printf("[platform] mock mode — skipping registration and platform sync")
		cfg.Platform.Endpoint = ""
	}
	if !*mockMode && !agentState.IsRegistered() {
		pos := gpsProvider.Position()
		hwInfo := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
		regResp, err := platHolder.Get().Register(ctx, platform.RegisterRequest{
			Serial:       agentState.GetSerial(),
			HardwareInfo: hwInfo,
			AgentVersion: version,
			Lat:          pos.Lat,
			Lon:          pos.Lon,
		})
		if err != nil {
			log.Printf("[platform] registration failed (will retry): %v", err)
		} else {
			agentState.SetRegistration(regResp.DeviceID, regResp.APIKey, regResp.StationID, regResp.ClaimCode)
			if err := agentState.Save(); err != nil {
				log.Printf("[platform] failed to save state: %v", err)
			}
			// Re-create client with new API key.
			platHolder.Set(platform.NewClient(cfg.Platform.Endpoint, agentState.GetAPIKey()))
			log.Printf("[platform] registered: device=%s station=%s claim_code=%s",
				agentState.GetDeviceID(), agentState.GetStationID(), agentState.GetClaimCode())
		}
	}
	platHolder.Get().LogConnectivity()

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
	otaUpdater := updater.New(version, exe)
	if !*mockMode {
		go otaUpdater.Run(ctx)
		log.Printf("Updater: checking GitHub releases daily")
	}

	// --- Route Cache (fed by platform ingest response) ---
	routeCache := routes.New()
	routeAdapter := &routeAdapterImpl{cache: routeCache}
	log.Printf("Routes: platform SWIM feed")

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

	// --- BLE Provisioning ---
	var bleService *ble.Service
	if !*mockMode && cfg.BLE.Enabled {
		bleService = ble.NewService(cfg.BLE, wifiMgr, agentState, version)

		// Wire up registration callback for first-boot scenario.
		bleService.SetRegisterFunc(func(regCtx context.Context) error {
			pos := gpsProvider.Position()
			hwInfo := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
			regResp, err := platHolder.Get().Register(regCtx, platform.RegisterRequest{
				Serial:       agentState.GetSerial(),
				HardwareInfo: hwInfo,
				AgentVersion: version,
				Lat:          pos.Lat,
				Lon:          pos.Lon,
			})
			if err != nil {
				return err
			}
			agentState.SetRegistration(regResp.DeviceID, regResp.APIKey, regResp.StationID, regResp.ClaimCode)
			if err := agentState.Save(); err != nil {
				return err
			}
			platHolder.Set(platform.NewClient(cfg.Platform.Endpoint, agentState.GetAPIKey()))
			log.Printf("[ble] registration complete: device=%s claim_code=%s",
				agentState.GetDeviceID(), agentState.GetClaimCode())
			return nil
		})

		go bleService.Run(ctx)

		if cfg.BLE.AutoPairOnBoot && !agentState.GetClaimed() {
			bleService.StartAdvertising()
			log.Printf("[ble] auto-advertising (device unclaimed)")
		}
		log.Printf("[ble] service started (window=%ds)", cfg.BLE.WindowSeconds)
	} else {
		log.Printf("[ble] disabled (mock=%v, enabled=%v)", *mockMode, cfg.BLE.Enabled)
	}

	// --- Omni Subsystem (SDR + Satellite + Scheduler) ---
	var omniMode sdr.Mode
	var satService *sat.Service
	var sched *scheduler.Scheduler
	var omniSDRs []sdr.SDRDevice

	// Build decoder factory: SatDump if available, NoopDecoder otherwise.
	var decoderFn func(noradID int, satName string) scheduler.Decoder
	satdumpBin, satdumpErr := exec.LookPath(cfg.Omni.SatDumpBin)
	if satdumpErr != nil {
		log.Printf("[omni] satdump not found, using noop decoders")
		decoderFn = func(noradID int, satName string) scheduler.Decoder {
			return scheduler.NewNoopDecoder(satName)
		}
	} else {
		log.Printf("[omni] satdump found: %s", satdumpBin)
		outputDir := cfg.Omni.DecoderOutputDir
		decoderFn = func(noradID int, satName string) scheduler.Decoder {
			if satellite.GetPipeline(noradID) == nil {
				return scheduler.NewNoopDecoder(satName)
			}
			return satellite.NewSatDumpDecoder(noradID, satName, satdumpBin, outputDir)
		}
	}

	if cfg.Omni.Enabled {
		// Detect SDR hardware.
		if *mockMode {
			// In mock mode, create synthetic SDRs for testing.
			mockHandles := []sdr.SDRHandle{
				&sdr.MockSDRHandle{MockID: "mock-sdr-0", MockSerial: "SKT-MOCK-0", MockTuner: "R820T"},
			}
			omniMode = sdr.ModeOmniOnly
			log.Printf("[omni] mock mode: 1 synthetic SDR, mode=%s", omniMode)

			// Start satellite service (real TLE fetch even in mock mode).
			satService = sat.NewService(cfg.Omni.MinElevation, cfg.Omni.TLERefreshHrs)
			go satService.Start(ctx)

			// Set station position from GPS.
			pos := gpsProvider.Position()
			if pos.Lat != 0 || pos.Lon != 0 {
				satService.SetStation(pos.Lat, pos.Lon, 0)
			}

			// Start scheduler with mock SDRs.
			if cfg.Omni.SchedulerEnabled {
				sched = scheduler.NewScheduler(mockHandles, satService, decoderFn)
				go sched.Run(ctx)
				log.Printf("[omni] scheduler started with %d mock SDR(s)", len(mockHandles))
			}
		} else {
			// Real hardware detection.
			allSDRs := sdr.Detect()
			readsbSerial, readsbActive := sdr.DetectReadsbSerial()

			if readsbActive {
				log.Printf("[omni] readsb active, using device serial=%s", readsbSerial)
				omniSDRs = sdr.FilterAvailable(allSDRs, readsbSerial)
			} else {
				omniSDRs = allSDRs
			}

			omniMode = sdr.DetermineMode(readsbActive, len(omniSDRs))
			log.Printf("[omni] detected %d total SDR(s), %d available for omni, mode=%s",
				len(allSDRs), len(omniSDRs), omniMode)

			// Program serial numbers on unconfigured dongles.
			if len(omniSDRs) > 0 {
				programmed := sdr.ProgramSerials(omniSDRs)
				if programmed > 0 {
					log.Printf("[omni] programmed serial numbers on %d SDR(s)", programmed)
				}
			}

			// Start satellite service.
			satService = sat.NewService(cfg.Omni.MinElevation, cfg.Omni.TLERefreshHrs)
			go satService.Start(ctx)

			// Set station position.
			pos := gpsProvider.Position()
			if pos.Lat != 0 || pos.Lon != 0 {
				satService.SetStation(pos.Lat, pos.Lon, 0)
			}

			// Start scheduler if we have available SDRs.
			if len(omniSDRs) > 0 && cfg.Omni.SchedulerEnabled {
				handles := make([]sdr.SDRHandle, len(omniSDRs))
				for i := range omniSDRs {
					handles[i] = sdr.NewHandle(omniSDRs[i])
				}
				sched = scheduler.NewScheduler(handles, satService, decoderFn)
				go sched.Run(ctx)
				log.Printf("[omni] scheduler started with %d SDR(s)", len(handles))
			}
		}

		// Wire up post-pass reporter.
		if sched != nil && !*mockMode {
			reporter := satellite.NewReporter(platHolder.Get, agentState.GetStationID())
			sched.SetOnComplete(func(task *scheduler.Task, outputDir string) {
				reporter.ReportPass(ctx, task, outputDir)
			})
			log.Printf("[omni] post-pass reporter enabled")
		}
	} else {
		omniMode = sdr.ModeNone
		log.Printf("[omni] disabled by config")
	}

	// --- Platform Sync (background) ---
	if !*mockMode && platHolder.IsConfigured() && dataQueue != nil {
		go runPlatformSync(ctx, platHolder, aircraftProvider, gpsProvider, dataQueue, cfg, agentState, claimProv, srv, enrichAdapter, routeCache, bleService, omniMode, satService, sched, omniSDRs)
	}

	log.Printf("SkyTracker Agent ready — http://localhost:%d", cfg.Display.Port)

	// Signal handling loop: SIGUSR1 triggers BLE, INT/TERM shuts down.
	go func() {
		for sig := range sigCh {
			switch sig {
			case syscall.SIGUSR1:
				if bleService != nil {
					bleService.StartAdvertising()
					log.Printf("[ble] SIGUSR1 received — advertising triggered")
				}
			case syscall.SIGINT, syscall.SIGTERM:
				log.Printf("Received signal %v, shutting down...", sig)
				cancel()
				return
			}
		}
	}()

	// Wait for context cancellation.
	<-ctx.Done()

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

// findAircraftCSV looks for the tar1090-db aircraft CSV.
func findAircraftCSV() string {
	candidates := []string{
		"data/aircraft.csv.gz",
		"./data/aircraft.csv.gz",
		"/opt/skytracker/data/aircraft.csv.gz",
	}

	if exe, err := os.Executable(); err == nil {
		candidates = append([]string{filepath.Join(filepath.Dir(exe), "data", "aircraft.csv.gz")}, candidates...)
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			abs, _ := filepath.Abs(path)
			return abs
		}
	}

	return "data/aircraft.csv.gz"
}

// routeAdapterImpl adapts routes.Cache to server.RouteLookup.
type routeAdapterImpl struct {
	cache *routes.Cache
}

func (r *routeAdapterImpl) Get(callsign string) *server.RouteInfo {
	route := r.cache.Get(callsign)
	if route == nil {
		return nil
	}
	return &server.RouteInfo{
		Origin:      route.Origin,
		Destination: route.Destination,
	}
}

// platformClientHolder provides thread-safe access to the platform client,
// which may be replaced after BLE registration completes.
type platformClientHolder struct {
	mu     sync.RWMutex
	client *platform.Client
}

func (h *platformClientHolder) Get() *platform.Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.client
}

func (h *platformClientHolder) Set(c *platform.Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.client = c
}

func (h *platformClientHolder) IsConfigured() bool {
	c := h.Get()
	return c != nil && c.IsConfigured()
}

// claimStateProviderImpl wraps agent state to implement server.ClaimStateProvider.
type claimStateProviderImpl struct {
	state    *state.State
	endpoint string
}

func (c *claimStateProviderImpl) ClaimState() server.ClaimState {
	claimCode := c.state.GetClaimCode()
	cs := server.ClaimState{
		Registered: c.state.IsRegistered(),
		ClaimCode:  claimCode,
		Claimed:    c.state.GetClaimed(),
	}
	if claimCode != "" {
		// Derive frontend URL from API endpoint (api.skytracker.ai → skytracker.ai).
		frontendURL := strings.Replace(c.endpoint, "://api.", "://", 1)
		cs.ClaimURL = frontendURL + "/claim?code=" + url.QueryEscape(claimCode)
	}
	return cs
}

// runPlatformSync periodically syncs aircraft data to skytracker.ai.
func runPlatformSync(ctx context.Context, holder *platformClientHolder, ap aircraftInterface, gps gpsInterface, q *queue.Queue, cfg *config.Config, agentState *state.State, claimProv *claimStateProviderImpl, srv *server.Server, enricher *server.EnrichmentAdapter, routeCache *routes.Cache, bleService *ble.Service, omniMode sdr.Mode, satService *sat.Service, sched *scheduler.Scheduler, omniSDRs []sdr.SDRDevice) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Registration retry helper — retries with exponential backoff on each health tick.
	regHelper := newRegistrationHelper(holder, agentState, cfg.Platform.Endpoint)

	// Poll health more frequently while unclaimed (30s vs 5m).
	healthInterval := 30 * time.Second
	if agentState.GetClaimed() {
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
				isLADD := false
				if enricher != nil {
					if info := enricher.LookupAircraft(a.Hex); info != nil {
						typeCode = info.TypeCode
						isLADD = info.LADD
					}
				}

				s := platform.IngestSighting{
					Timestamp: time.Now().UnixMilli(),
					ICAOHex:   a.Hex,
					Altitude:  a.Altitude(),
					Speed:     a.Speed(),
					Heading:   a.Heading(),
					Lat:       *a.Lat,
					Lon:       *a.Lon,
					Distance:  dist,
					VertRate:  a.VertRate(),
				}
				if !isLADD {
					// LADD: send position for coverage stats but strip identifying fields.
					s.Callsign = a.Callsign()
					s.Type = typeCode
					s.Squawk = a.Squawk
				}
				sightings = append(sightings, s)
			}

			// Try to ingest.
			ingestResp, err := holder.Get().Ingest(ctx, platform.IngestRequest{Sightings: sightings})
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

			// Feed SWIM routes from the ingest response into the local cache.
			if ingestResp != nil && len(ingestResp.Routes) > 0 {
				routeData := make(map[string]routes.RouteData, len(ingestResp.Routes))
				for cs, r := range ingestResp.Routes {
					routeData[cs] = routes.RouteData{Origin: r.Origin, Destination: r.Destination}
				}
				routeCache.Update(routeData)
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
				holder.Get().Ingest(ctx, platform.IngestRequest{Sightings: queuedSightings})
			}

		case <-healthTicker.C:
			// Retry registration if still unregistered.
			if !agentState.IsRegistered() {
				regHelper.tryRegister(ctx, gps)
				if !agentState.IsRegistered() {
					continue // skip health report until registered
				}
			}
			aircraft := ap.Aircraft()
			pos := gps.Position()
			healthReq := platform.HealthRequest{
				Uptime:        int64(time.Since(startTime).Seconds()),
				GPSFix:        pos.HasFix,
				Lat:           pos.Lat,
				Lon:           pos.Lon,
				AircraftCount: len(aircraft),
				AgentVersion:  version,
				LastSync:      time.Now().Unix(),
				QueueSize:     q.Count(),
				OmniMode:      string(omniMode),
			}

			// Populate omni health fields.
			if len(omniSDRs) > 0 {
				healthReq.SDRCount = len(omniSDRs)
				serials := make([]string, 0, len(omniSDRs))
				for _, d := range omniSDRs {
					if d.SerialNumber != "" {
						serials = append(serials, d.SerialNumber)
					}
				}
				healthReq.SDRSerials = serials
			}
			if satService != nil {
				healthReq.TLECount = satService.TLECount()
				if age := satService.TLEAge(); age > 0 {
					healthReq.TLEAge = int64(age.Seconds())
				}
				upcoming := satService.GetUpcomingPasses()
				healthReq.SatellitePasses24h = len(upcoming)
				// Send up to 10 structured upcoming passes for the platform dashboard.
				limit := len(upcoming)
				if limit > 10 {
					limit = 10
				}
				passes := make([]platform.UpcomingPass, 0, limit)
				for _, p := range upcoming[:limit] {
					freq := 0.0
					if len(p.Frequencies) > 0 {
						freq = p.Frequencies[0]
					}
					passes = append(passes, platform.UpcomingPass{
						NoradID:      p.NoradID,
						SatName:      p.Name,
						Frequency:    freq,
						AOSPredicted: p.AOS.Unix(),
						LOSPredicted: p.LOS.Unix(),
						MaxElevation: p.MaxElevation,
					})
				}
				healthReq.UpcomingPasses = passes
			}
			if sched != nil {
				healthReq.SchedulerState = sched.State()
				healthReq.ActiveDecoder = sched.ActiveDecoder()
			}

			resp, err := holder.Get().Health(ctx, healthReq)
			if err != nil {
				continue
			}

			// Sync station name from platform (picks up renames too).
			if resp.StationName != "" {
				srv.SetStationName(resp.StationName)
			}

			// Detect when device gets claimed.
			if resp.Claimed && !agentState.GetClaimed() {
				log.Printf("[platform] device claimed! station_name=%s", resp.StationName)
				agentState.MarkClaimed()

				if err := agentState.Save(); err != nil {
					log.Printf("[platform] failed to save claim state: %v", err)
				}

				if bleService != nil {
					bleService.OnClaimed()
				}

				// Switch to slower health polling now that we're claimed.
				healthTicker.Reset(5 * time.Minute)
			}
		}
	}
}

