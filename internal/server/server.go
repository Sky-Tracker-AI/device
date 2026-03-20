package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/websocket"

	"github.com/skytracker/skytracker-device/internal/adsb"
	"github.com/skytracker/skytracker-device/internal/geo"
)

// Enricher is the interface for looking up aircraft type and airline info.
type Enricher interface {
	LookupAircraft(icaoHex string) *AircraftTypeInfo
	LookupAirline(callsign string) *AirlineNameInfo
}

// AircraftTypeInfo mirrors enrichment.AircraftInfo to avoid import cycles.
type AircraftTypeInfo struct {
	Registration string
	TypeCode     string
	TypeName     string
	Operator     string
	Owner        string
	Year         string
	LADD         bool
	PIA          bool
	Military     bool
}

// AirlineNameInfo mirrors enrichment.AirlineInfo to avoid import cycles.
type AirlineNameInfo struct {
	Name    string
	ICAO    string
	Country string
}

// AircraftProvider is the interface for getting the current aircraft list.
type AircraftProvider interface {
	Aircraft() []adsb.Aircraft
}

// GPSProvider is the interface for getting the station position.
type GPSProvider interface {
	Position() StationPosition
}

// StationPosition is a simplified GPS position.
type StationPosition struct {
	Lat    float64
	Lon    float64
	HasFix bool
}

// EnrichedAircraft is the JSON payload sent over WebSocket.
type EnrichedAircraft struct {
	ICAOHex      string   `json:"icao_hex"`
	Callsign     string   `json:"callsign"`
	Registration string   `json:"registration,omitempty"`
	Type         string   `json:"type"`
	TypeName     string   `json:"type_name,omitempty"`
	Airline      string   `json:"airline,omitempty"`
	Operator     string   `json:"operator,omitempty"`
	Origin       string   `json:"origin,omitempty"`
	Destination  string   `json:"destination,omitempty"`
	Altitude     int      `json:"altitude"`
	Speed        float64  `json:"speed"`
	Heading      float64  `json:"heading"`
	Lat          float64  `json:"lat"`
	Lon          float64  `json:"lon"`
	DistanceNM   float64  `json:"distance_nm"`
	Bearing      float64  `json:"bearing"`
	RarityScore  *int     `json:"rarity_score"`
	Military     bool     `json:"military,omitempty"`
}

// WSMessage is the top-level WebSocket message.
type WSMessage struct {
	Type       string             `json:"type"`
	Timestamp  int64              `json:"timestamp"`
	Station    WSStation          `json:"station"`
	Aircraft   []EnrichedAircraft `json:"aircraft"`
	Count      int                `json:"count"`
}

// WSStation includes station info in each broadcast.
type WSStation struct {
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
	Name string  `json:"name"`
}

// RouteLookup is the interface for looking up flight routes.
type RouteLookup interface {
	Get(callsign string) *RouteInfo
}

// RouteInfo holds origin/destination for a flight.
type RouteInfo struct {
	Origin      string
	Destination string
}

// ClaimState holds the current registration and claim status for the device.
type ClaimState struct {
	Registered  bool   `json:"registered"`
	ClaimCode   string `json:"claim_code,omitempty"`
	Claimed     bool   `json:"claimed"`
	StationName string `json:"station_name,omitempty"`
	ClaimURL    string `json:"claim_url,omitempty"`
}

// ClaimStateProvider returns the current claim state.
type ClaimStateProvider interface {
	ClaimState() ClaimState
}

// Server is the HTTP and WebSocket server for the SkyTracker Agent.
type Server struct {
	port         int
	stationName  string
	uiDir        string
	maxRangeNM   int
	pollInterval time.Duration

	aircraftProv AircraftProvider
	gpsProv      GPSProvider
	enricher     Enricher
	routes       RouteLookup
	claimProv    ClaimStateProvider

	upgrader websocket.Upgrader

	mu      sync.Mutex
	clients map[*websocket.Conn]bool
}

// NewServer creates a new server.
func NewServer(port int, stationName, uiDir string, maxRangeNM, pollIntervalMS int, ap AircraftProvider, gps GPSProvider, enricher Enricher, routes RouteLookup, claimProv ClaimStateProvider) *Server {
	return &Server{
		port:         port,
		stationName:  stationName,
		uiDir:        uiDir,
		maxRangeNM:   maxRangeNM,
		pollInterval: time.Duration(pollIntervalMS) * time.Millisecond,
		aircraftProv: ap,
		gpsProv:      gps,
		enricher:     enricher,
		routes:       routes,
		claimProv:    claimProv,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		clients: make(map[*websocket.Conn]bool),
	}
}

// SetStationName updates the station name displayed in broadcasts.
func (s *Server) SetStationName(name string) {
	s.mu.Lock()
	s.stationName = name
	s.mu.Unlock()
}

// Run starts the HTTP server and WebSocket broadcaster. Blocks until ctx
// is cancelled.
func (s *Server) Run(ctx context.Context) error {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)

	// WebSocket endpoint.
	r.Get("/ws", s.handleWS)

	// Health endpoint.
	r.Get("/api/health", s.handleHealth)

	// Claim status endpoint (used by setup.js).
	r.Get("/api/status", s.handleStatus)

	// Serve static UI files.
	fs := http.FileServer(http.Dir(s.uiDir))
	r.Handle("/*", fs)

	addr := fmt.Sprintf(":%d", s.port)
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	// Start the WebSocket broadcaster.
	go s.broadcast(ctx)

	// Start the HTTP server.
	go func() {
		log.Printf("[server] listening on %s", addr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("[server] listen error: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s.closeAllClients()
	return srv.Shutdown(shutdownCtx)
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[server] ws upgrade error: %v", err)
		return
	}

	s.mu.Lock()
	s.clients[conn] = true
	s.mu.Unlock()

	log.Printf("[server] ws client connected (%d total)", len(s.clients))

	// Read loop (handles pong, detects disconnect).
	go func() {
		defer func() {
			s.mu.Lock()
			if _, ok := s.clients[conn]; ok {
				delete(s.clients, conn)
				conn.Close()
				log.Printf("[server] ws client disconnected (%d remaining)", len(s.clients))
			}
			s.mu.Unlock()
		}()

		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			return nil
		})

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	pos := s.gpsProv.Position()
	aircraft := s.aircraftProv.Aircraft()

	resp := map[string]interface{}{
		"status":         "ok",
		"station":        s.stationName,
		"aircraft_count": len(aircraft),
		"gps_fix":        pos.HasFix,
		"gps_lat":        pos.Lat,
		"gps_lon":        pos.Lon,
		"timestamp":      time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	var cs ClaimState
	if s.claimProv != nil {
		cs = s.claimProv.ClaimState()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cs)
}

func (s *Server) broadcast(ctx context.Context) {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sendUpdate()
		}
	}
}

func (s *Server) sendUpdate() {
	pos := s.gpsProv.Position()
	rawAircraft := s.aircraftProv.Aircraft()

	enriched := make([]EnrichedAircraft, 0, len(rawAircraft))

	for _, a := range rawAircraft {
		if !a.HasPosition() {
			continue
		}

		distNM := geo.HaversineNM(pos.Lat, pos.Lon, *a.Lat, *a.Lon)
		if distNM > float64(s.maxRangeNM) {
			continue
		}

		bearing := geo.Bearing(pos.Lat, pos.Lon, *a.Lat, *a.Lon)
		callsign := a.Callsign()

		ea := EnrichedAircraft{
			ICAOHex:    a.Hex,
			Callsign:   callsign,
			Altitude:   a.Altitude(),
			Speed:      a.Speed(),
			Heading:    a.Heading(),
			Lat:        *a.Lat,
			Lon:        *a.Lon,
			DistanceNM: round2(distNM),
			Bearing:    round1(bearing),
		}

		// Enrich with type info.
		if s.enricher != nil {
			if info := s.enricher.LookupAircraft(a.Hex); info != nil {
				ea.Type = info.TypeCode
				ea.TypeName = info.TypeName
				ea.Military = info.Military
				if !info.LADD {
					// LADD aircraft: show on radar but suppress identifying info.
					ea.Registration = info.Registration
					ea.Operator = info.Operator
				}
			}
			if info := s.enricher.LookupAirline(callsign); info != nil {
				ea.Airline = info.Name
			}
		}

		if s.routes != nil && callsign != "" {
			if route := s.routes.Get(callsign); route != nil {
				ea.Origin = route.Origin
				ea.Destination = route.Destination
			}
		}

		enriched = append(enriched, ea)
	}

	// Sort by distance.
	sort.Slice(enriched, func(i, j int) bool {
		return enriched[i].DistanceNM < enriched[j].DistanceNM
	})

	s.mu.Lock()
	defer s.mu.Unlock()

	msg := WSMessage{
		Type:      "aircraft_update",
		Timestamp: time.Now().UnixMilli(),
		Station: WSStation{
			Lat:  pos.Lat,
			Lon:  pos.Lon,
			Name: s.stationName,
		},
		Aircraft: enriched,
		Count:    len(enriched),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[server] marshal error: %v", err)
		return
	}

	for conn := range s.clients {
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[server] write error: %v", err)
			conn.Close()
			delete(s.clients, conn)
		}
	}
}

func (s *Server) closeAllClients() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for conn := range s.clients {
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutting down"))
		conn.Close()
		delete(s.clients, conn)
	}
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}

func round1(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
}

// EnrichmentAdapter wraps the enrichment package types to satisfy the
// Enricher interface without creating import cycles.
type EnrichmentAdapter struct {
	LookupAircraftFn func(icaoHex string) *AircraftTypeInfo
	LookupAirlineFn  func(callsign string) *AirlineNameInfo
}

func (a *EnrichmentAdapter) LookupAircraft(icaoHex string) *AircraftTypeInfo {
	if a.LookupAircraftFn != nil {
		return a.LookupAircraftFn(icaoHex)
	}
	return nil
}

func (a *EnrichmentAdapter) LookupAirline(callsign string) *AirlineNameInfo {
	if a.LookupAirlineFn != nil {
		return a.LookupAirlineFn(callsign)
	}
	return nil
}

