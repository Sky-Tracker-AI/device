package enrichment

import (
	"database/sql"
	"log"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

// AircraftInfo is the enriched information for an aircraft.
type AircraftInfo struct {
	Registration string // e.g. "N8674B"
	TypeCode     string // e.g. "B738"
	TypeName     string // e.g. "Boeing 737-8H4"
	Manufacturer string // e.g. "Boeing"
	Operator     string // e.g. "Southwest Airlines"
	Owner        string // e.g. "Wells Fargo Trust"
}

// AirlineInfo is the enriched information for an airline.
type AirlineInfo struct {
	Name    string // e.g. "Southwest Airlines"
	ICAO    string // e.g. "SWA"
	Country string // e.g. "United States"
}

// Engine provides aircraft type and airline lookups from a local SQLite
// database. It is safe for concurrent use.
type Engine struct {
	db *sql.DB

	// In-memory caches for fast lookups.
	mu             sync.RWMutex
	aircraftCache  map[string]*AircraftInfo // keyed by ICAO hex (lowercase)
	airlineCache   map[string]*AirlineInfo  // keyed by callsign prefix (uppercase)
}

// NewEngine opens the SQLite enrichment database at the given path.
// The database should have tables:
//   - aircraft_types (icao_hex TEXT, type_code TEXT, type_name TEXT, manufacturer TEXT)
//   - airlines (prefix TEXT, name TEXT, icao TEXT, country TEXT)
//
// If the database does not exist or tables are missing, the engine returns
// empty results (offline-first: enrichment is additive).
func NewEngine(dbPath string) *Engine {
	e := &Engine{
		aircraftCache: make(map[string]*AircraftInfo),
		airlineCache:  make(map[string]*AirlineInfo),
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Printf("[enrichment] cannot open db %s: %v (enrichment disabled)", dbPath, err)
		return e
	}

	// Test the connection.
	if err := db.Ping(); err != nil {
		log.Printf("[enrichment] cannot ping db: %v (enrichment disabled)", err)
		db.Close()
		return e
	}

	e.db = db
	e.loadCaches()
	return e
}

// Close releases the database connection.
func (e *Engine) Close() {
	if e.db != nil {
		e.db.Close()
	}
}

// LookupAircraft looks up aircraft type info by ICAO hex address.
func (e *Engine) LookupAircraft(icaoHex string) *AircraftInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.aircraftCache[strings.ToLower(icaoHex)]
}

// LookupAirline looks up airline info from a callsign. It extracts the
// alphabetic prefix from the callsign (e.g. "SWA" from "SWA1234").
func (e *Engine) LookupAirline(callsign string) *AirlineInfo {
	prefix := extractPrefix(callsign)
	if prefix == "" {
		return nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.airlineCache[prefix]
}

func (e *Engine) loadCaches() {
	if e.db == nil {
		return
	}

	// Load aircraft by ICAO hex.
	rows, err := e.db.Query("SELECT icao_hex, registration, type_code, type_name, manufacturer, operator, owner FROM aircraft")
	if err != nil {
		log.Printf("[enrichment] cannot query aircraft: %v", err)
	} else {
		defer rows.Close()
		count := 0
		for rows.Next() {
			var hex, reg, code, name, mfr, op, owner string
			if err := rows.Scan(&hex, &reg, &code, &name, &mfr, &op, &owner); err != nil {
				continue
			}
			e.aircraftCache[strings.ToLower(hex)] = &AircraftInfo{
				Registration: reg,
				TypeCode:     code,
				TypeName:     name,
				Manufacturer: mfr,
				Operator:     op,
				Owner:        owner,
			}
			count++
		}
		log.Printf("[enrichment] loaded %d aircraft records", count)
	}

	// Load airlines.
	rows2, err := e.db.Query("SELECT callsign_prefix, airline_name, country FROM airlines")
	if err != nil {
		log.Printf("[enrichment] cannot query airlines: %v", err)
	} else {
		defer rows2.Close()
		count := 0
		for rows2.Next() {
			var prefix, name, country string
			if err := rows2.Scan(&prefix, &name, &country); err != nil {
				continue
			}
			e.airlineCache[strings.ToUpper(prefix)] = &AirlineInfo{
				Name:    name,
				ICAO:    prefix,
				Country: country,
			}
			count++
		}
		log.Printf("[enrichment] loaded %d airline records", count)
	}
}

// extractPrefix returns the leading alphabetic characters of a callsign.
// For example, "SWA1234" → "SWA", "N172SP" → "" (no airline prefix for
// N-numbers).
func extractPrefix(callsign string) string {
	// N-numbers (US general aviation) start with N followed by digits.
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
			"a0b1c2": {TypeCode: "B738", TypeName: "Boeing 737-800", Manufacturer: "Boeing"},
			"a1d2e3": {TypeCode: "A320", TypeName: "Airbus A320-200", Manufacturer: "Airbus"},
			"a2f3g4": {TypeCode: "B763", TypeName: "Boeing 767-300ER", Manufacturer: "Boeing"},
			"a3h4i5": {TypeCode: "A321", TypeName: "Airbus A321-200", Manufacturer: "Airbus"},
			"a4j5k6": {TypeCode: "A320", TypeName: "Airbus A320-200", Manufacturer: "Airbus"},
			"a5l6m7": {TypeCode: "C172", TypeName: "Cessna 172 Skyhawk", Manufacturer: "Cessna"},
			"a6n7o8": {TypeCode: "E75L", TypeName: "Embraer E175LR", Manufacturer: "Embraer"},
			"a7p8q9": {TypeCode: "B77L", TypeName: "Boeing 777-200LRF", Manufacturer: "Boeing"},
			"a8r9s0": {TypeCode: "BE35", TypeName: "Beechcraft Bonanza", Manufacturer: "Beechcraft"},
			"a9t0u1": {TypeCode: "B739", TypeName: "Boeing 737-900ER", Manufacturer: "Boeing"},
			"b0v1w2": {TypeCode: "E45X", TypeName: "Embraer EMB 145XR", Manufacturer: "Embraer"},
			"b1x2y3": {TypeCode: "A20N", TypeName: "Airbus A320neo", Manufacturer: "Airbus"},
			"b2z3a4": {TypeCode: "C17", TypeName: "Boeing C-17 Globemaster III", Manufacturer: "Boeing"},
			"b3b4c5": {TypeCode: "PA28", TypeName: "Piper Cherokee", Manufacturer: "Piper"},
			"b4d5e6": {TypeCode: "B748", TypeName: "Boeing 747-8F", Manufacturer: "Boeing"},
		},
		airlines: map[string]*AirlineInfo{
			"SWA": {Name: "Southwest Airlines", ICAO: "SWA", Country: "United States"},
			"UAL": {Name: "United Airlines", ICAO: "UAL", Country: "United States"},
			"DAL": {Name: "Delta Air Lines", ICAO: "DAL", Country: "United States"},
			"AAL": {Name: "American Airlines", ICAO: "AAL", Country: "United States"},
			"JBU": {Name: "JetBlue Airways", ICAO: "JBU", Country: "United States"},
			"SKW": {Name: "SkyWest Airlines", ICAO: "SKW", Country: "United States"},
			"FDX": {Name: "FedEx Express", ICAO: "FDX", Country: "United States"},
			"ASA": {Name: "Alaska Airlines", ICAO: "ASA", Country: "United States"},
			"FFT": {Name: "Frontier Airlines", ICAO: "FFT", Country: "United States"},
			"NKS": {Name: "Spirit Airlines", ICAO: "NKS", Country: "United States"},
			"EVAC": {Name: "US Air Force", ICAO: "EVAC", Country: "United States"},
			"UPS": {Name: "UPS Airlines", ICAO: "UPS", Country: "United States"},
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
