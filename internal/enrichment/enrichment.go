package enrichment

import (
	"bufio"
	"compress/gzip"
	"log"
	"os"
	"strings"
	"sync"
)

// AircraftInfo is the enriched information for an aircraft.
type AircraftInfo struct {
	Registration string
	TypeCode     string
	TypeName     string
	Operator     string
	Owner        string
	Year         string
	LADD         bool // Limiting Aircraft Data Displayed
	PIA          bool // Privacy ICAO Address (FAA-reassigned hex)
	Military     bool
}

// AirlineInfo is the enriched information for an airline.
type AirlineInfo struct {
	Name    string
	ICAO    string
	Country string
}

// Engine provides aircraft type and airline lookups from a tar1090-db
// CSV file. It is safe for concurrent use.
type Engine struct {
	csvPath string

	mu            sync.RWMutex
	aircraftCache map[string]*AircraftInfo
	airlineCache  map[string]*AirlineInfo
}

// NewEngine loads aircraft data from a tar1090-db aircraft.csv.gz file
// and airline data from the built-in airline table.
func NewEngine(csvPath string) *Engine {
	e := &Engine{
		csvPath:       csvPath,
		aircraftCache: make(map[string]*AircraftInfo, 650000),
		airlineCache:  make(map[string]*AirlineInfo, len(defaultAirlines)),
	}

	if err := e.loadAircraft(); err != nil {
		log.Printf("[enrichment] cannot load %s: %v (enrichment disabled)", csvPath, err)
	}

	e.loadAirlines()
	return e
}

// Close is a no-op (retained for interface compatibility).
func (e *Engine) Close() {}

// LookupAircraft looks up aircraft type info by ICAO hex address.
func (e *Engine) LookupAircraft(icaoHex string) *AircraftInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.aircraftCache[strings.ToLower(icaoHex)]
}

// LookupAirline looks up airline info from a callsign.
func (e *Engine) LookupAirline(callsign string) *AirlineInfo {
	prefix := extractPrefix(callsign)
	if prefix == "" {
		return nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.airlineCache[prefix]
}

// Reload re-reads the aircraft CSV and swaps the in-memory cache.
func (e *Engine) Reload() error {
	newCache, err := parseAircraftCSV(e.csvPath)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.aircraftCache = newCache
	e.mu.Unlock()
	log.Printf("[enrichment] reloaded %d aircraft records", len(newCache))
	return nil
}

func (e *Engine) loadAircraft() error {
	cache, err := parseAircraftCSV(e.csvPath)
	if err != nil {
		return err
	}
	e.aircraftCache = cache
	log.Printf("[enrichment] loaded %d aircraft records", len(cache))
	return nil
}

func (e *Engine) loadAirlines() {
	for prefix, info := range defaultAirlines {
		e.airlineCache[strings.ToUpper(prefix)] = info
	}
	log.Printf("[enrichment] loaded %d airline records", len(e.airlineCache))
}

// parseAircraftCSV reads a gzipped tar1090-db aircraft CSV.
// Format: icao_hex;registration;type_code;flags;type_name;year;owner;
// Flags: position 0=military, 1=interesting, 2=PIA, 3=LADD
func parseAircraftCSV(path string) (map[string]*AircraftInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	cache := make(map[string]*AircraftInfo, 650000)
	scanner := bufio.NewScanner(gz)

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.SplitN(line, ";", 8)
		if len(fields) < 7 {
			continue
		}

		hex := strings.ToLower(fields[0])
		if hex == "" {
			continue
		}

		flags := fields[3]
		military := len(flags) > 0 && flags[0] == '1'
		pia := len(flags) > 2 && flags[2] == '1'
		ladd := len(flags) > 3 && flags[3] == '1'

		reg := fields[1]
		if pia {
			reg = "" // PIA: registration must not be exposed
		}

		cache[hex] = &AircraftInfo{
			Registration: reg,
			TypeCode:     fields[2],
			TypeName:     fields[4],
			Owner:        fields[6],
			Operator:     fields[6], // CSV has no separate operator field
			Year:         fields[5],
			Military:     military,
			PIA:          pia,
			LADD:         ladd,
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return cache, nil
}

// extractPrefix returns the leading alphabetic characters of a callsign.
func extractPrefix(callsign string) string {
	if len(callsign) > 0 && callsign[0] == 'N' {
		if len(callsign) > 1 && callsign[1] >= '0' && callsign[1] <= '9' {
			return ""
		}
	}

	var prefix strings.Builder
	for _, c := range callsign {
		if c >= 'A' && c <= 'Z' {
			prefix.WriteRune(c)
		} else {
			break
		}
	}
	return prefix.String()
}

// MockEngine provides hardcoded enrichment data for development.
type MockEngine struct {
	aircraft map[string]*AircraftInfo
	airlines map[string]*AirlineInfo
}

// NewMockEngine creates an engine with built-in test data.
func NewMockEngine() *MockEngine {
	m := &MockEngine{
		aircraft: map[string]*AircraftInfo{
			"a0b1c2": {TypeCode: "B738", TypeName: "Boeing 737-800"},
			"a1d2e3": {TypeCode: "A320", TypeName: "Airbus A320-200"},
			"a2f3g4": {TypeCode: "B763", TypeName: "Boeing 767-300ER"},
			"a3h4i5": {TypeCode: "A321", TypeName: "Airbus A321-200"},
			"a4j5k6": {TypeCode: "A320", TypeName: "Airbus A320-200"},
			"a5l6m7": {TypeCode: "C172", TypeName: "Cessna 172 Skyhawk"},
			"a6n7o8": {TypeCode: "E75L", TypeName: "Embraer E175LR"},
			"a7p8q9": {TypeCode: "B77L", TypeName: "Boeing 777-200LRF"},
			"a8r9s0": {TypeCode: "BE35", TypeName: "Beechcraft Bonanza"},
			"a9t0u1": {TypeCode: "B739", TypeName: "Boeing 737-900ER"},
			"b0v1w2": {TypeCode: "E45X", TypeName: "Embraer EMB 145XR"},
			"b1x2y3": {TypeCode: "A20N", TypeName: "Airbus A320neo"},
			"b2z3a4": {TypeCode: "C17", TypeName: "Boeing C-17 Globemaster III", Military: true},
			"b3b4c5": {TypeCode: "PA28", TypeName: "Piper Cherokee"},
			"b4d5e6": {TypeCode: "B748", TypeName: "Boeing 747-8F"},
			"c0ladd": {TypeCode: "GLF6", TypeName: "Gulfstream G650", Owner: "LADD Test Owner", LADD: true},
			"c1pia0": {TypeCode: "B738", TypeName: "Boeing 737-800", PIA: true},
		},
		airlines: map[string]*AirlineInfo{
			"SWA":  {Name: "Southwest Airlines", ICAO: "SWA", Country: "United States"},
			"UAL":  {Name: "United Airlines", ICAO: "UAL", Country: "United States"},
			"DAL":  {Name: "Delta Air Lines", ICAO: "DAL", Country: "United States"},
			"AAL":  {Name: "American Airlines", ICAO: "AAL", Country: "United States"},
			"JBU":  {Name: "JetBlue Airways", ICAO: "JBU", Country: "United States"},
			"SKW":  {Name: "SkyWest Airlines", ICAO: "SKW", Country: "United States"},
			"FDX":  {Name: "FedEx Express", ICAO: "FDX", Country: "United States"},
			"ASA":  {Name: "Alaska Airlines", ICAO: "ASA", Country: "United States"},
			"FFT":  {Name: "Frontier Airlines", ICAO: "FFT", Country: "United States"},
			"NKS":  {Name: "Spirit Airlines", ICAO: "NKS", Country: "United States"},
			"EVAC": {Name: "US Air Force", ICAO: "EVAC", Country: "United States"},
			"UPS":  {Name: "UPS Airlines", ICAO: "UPS", Country: "United States"},
		},
	}
	return m
}

func (m *MockEngine) LookupAircraft(icaoHex string) *AircraftInfo {
	return m.aircraft[strings.ToLower(icaoHex)]
}

func (m *MockEngine) LookupAirline(callsign string) *AirlineInfo {
	prefix := extractPrefix(callsign)
	if prefix == "" {
		return nil
	}
	return m.airlines[prefix]
}

func (m *MockEngine) Close() {}
