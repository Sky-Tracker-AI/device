// mock-dump1090 serves synthetic aircraft data on /data/aircraft.json,
// mimicking the readsb/tar1090 HTTP interface. Useful for development
// and testing without SDR hardware.
//
// Usage:
//
//	go run ./cmd/mock-dump1090 [--port 8081] [--lat 30.2672] [--lon -97.7431]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// Aircraft represents a single aircraft in the readsb JSON format.
type Aircraft struct {
	Hex      string  `json:"hex"`
	Flight   string  `json:"flight,omitempty"`
	AltBaro  int     `json:"alt_baro,omitempty"`
	GS       float64 `json:"gs,omitempty"`
	Track    float64 `json:"track"`
	Lat      float64 `json:"lat,omitempty"`
	Lon      float64 `json:"lon,omitempty"`
	Seen     float64 `json:"seen"`
	SeenPos  float64 `json:"seen_pos,omitempty"`
	RSSI     float64 `json:"rssi"`
	Category string  `json:"category,omitempty"`
	AltGeom  int     `json:"alt_geom,omitempty"`
	BaroRate int     `json:"baro_rate,omitempty"`
	Squawk   string  `json:"squawk,omitempty"`
	NavQNH   float64 `json:"nav_qnh,omitempty"`
}

// Dump1090Response is the top-level response matching readsb JSON output.
type Dump1090Response struct {
	Now      float64    `json:"now"`
	Messages int        `json:"messages"`
	Aircraft []Aircraft `json:"aircraft"`
}

// aircraftTemplate defines a synthetic aircraft profile for generation.
type aircraftTemplate struct {
	callsignPrefix string
	flightNumMin   int
	flightNumMax   int
	hex            string
	typeCode       string
	altMin         int
	altMax         int
	speedMin       float64
	speedMax       float64
	category       string
}

// liveAircraft tracks a moving synthetic aircraft.
type liveAircraft struct {
	template  aircraftTemplate
	hex       string
	callsign  string
	lat       float64
	lon       float64
	alt       int
	speed     float64
	heading   float64
	rssi      float64
	squawk    string
	spawnTime time.Time
	lifespan  time.Duration
	category  string
	baroRate  int
}

var templates = []aircraftTemplate{
	{"SWA", 100, 9999, "a", "B738", 28000, 39000, 420, 510, "A3"},
	{"UAL", 100, 2500, "a", "A320", 30000, 41000, 440, 520, "A3"},
	{"DAL", 100, 2500, "a", "B763", 32000, 43000, 450, 530, "A5"},
	{"AAL", 100, 2500, "a", "A321", 30000, 40000, 440, 520, "A3"},
	{"JBU", 100, 2000, "a", "A320", 32000, 39000, 440, 510, "A3"},
	{"SKW", 3000, 6000, "a", "E75L", 24000, 37000, 380, 470, "A3"},
	{"RPA", 3000, 6000, "a", "E170", 22000, 35000, 370, 450, "A3"},
	{"ASA", 100, 1500, "a", "B739", 30000, 39000, 430, 510, "A3"},
	{"FDX", 100, 900, "a", "B763", 32000, 41000, 450, 530, "A5"},
	{"UPS", 100, 900, "a", "B748", 34000, 43000, 460, 540, "A5"},
	{"N", 100, 99999, "a", "C172", 1500, 8000, 90, 130, "A1"},
	{"N", 200, 99999, "a", "C182", 3000, 12000, 110, 160, "A1"},
	{"N", 300, 99999, "a", "SR22", 5000, 17000, 150, 200, "A1"},
	{"LXJ", 100, 900, "a", "C56X", 35000, 45000, 420, 500, "A2"},
	{"EJA", 100, 900, "a", "CL35", 37000, 45000, 430, 510, "A2"},
	{"RCH", 1000, 9999, "a", "C17", 18000, 32000, 380, 480, "A5"},
	{"CNV", 1000, 9999, "a", "C130", 15000, 28000, 250, 350, "A5"},
	{"VIR", 1, 200, "a", "A346", 34000, 43000, 460, 540, "A5"},
	{"BAW", 1, 300, "a", "B789", 34000, 43000, 460, 540, "A5"},
	{"AFR", 1, 300, "a", "A359", 35000, 43000, 460, 540, "A5"},
}

var (
	stationLat float64
	stationLon float64
	mu         sync.RWMutex
	aircraft   []*liveAircraft
	msgCount   int
)

func main() {
	port := flag.Int("port", 8081, "HTTP server port")
	flag.Float64Var(&stationLat, "lat", 30.2672, "Station latitude (default: Austin, TX)")
	flag.Float64Var(&stationLon, "lon", -97.7431, "Station longitude (default: Austin, TX)")
	flag.Parse()

	// Seed initial aircraft
	for i := 0; i < 12+rand.Intn(6); i++ {
		ac := spawnAircraft()
		// Randomize spawn time so not all aircraft are brand new
		ac.spawnTime = time.Now().Add(-time.Duration(rand.Intn(int(ac.lifespan.Seconds()))) * time.Second)
		aircraft = append(aircraft, ac)
	}

	// Update positions every second
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			updateAircraft()
		}
	}()

	http.HandleFunc("/data/aircraft.json", handleAircraftJSON)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("mock-dump1090: serving /data/aircraft.json on http://localhost%s", addr)
	log.Printf("mock-dump1090: station at %.4f, %.4f", stationLat, stationLon)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func handleAircraftJSON(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	defer mu.RUnlock()

	acList := make([]Aircraft, 0, len(aircraft))
	for _, ac := range aircraft {
		acList = append(acList, Aircraft{
			Hex:      ac.hex,
			Flight:   fmt.Sprintf("%-8s", ac.callsign),
			AltBaro:  ac.alt,
			AltGeom:  ac.alt + rand.Intn(200) - 100,
			GS:       math.Round(ac.speed*10) / 10,
			Track:    math.Round(ac.heading*10) / 10,
			Lat:      math.Round(ac.lat*1e6) / 1e6,
			Lon:      math.Round(ac.lon*1e6) / 1e6,
			Seen:     math.Round(rand.Float64()*20) / 10,
			SeenPos:  math.Round(rand.Float64()*30) / 10,
			RSSI:     ac.rssi,
			Category: ac.category,
			BaroRate: ac.baroRate,
			Squawk:   ac.squawk,
			NavQNH:   1013.2,
		})
	}

	resp := Dump1090Response{
		Now:      float64(time.Now().UnixNano()) / 1e9,
		Messages: msgCount,
		Aircraft: acList,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(resp)
	msgCount += rand.Intn(50) + 10
}

func updateAircraft() {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()
	alive := make([]*liveAircraft, 0, len(aircraft))

	for _, ac := range aircraft {
		// Remove expired aircraft
		if now.Sub(ac.spawnTime) > ac.lifespan {
			continue
		}
		moveAircraft(ac)
		alive = append(alive, ac)
	}

	aircraft = alive

	// Spawn new aircraft to maintain 10-20 count
	for len(aircraft) < 10+rand.Intn(5) {
		aircraft = append(aircraft, spawnAircraft())
	}
}

func moveAircraft(ac *liveAircraft) {
	// Convert speed from knots to degrees per second (very rough approximation)
	// 1 knot ≈ 1.852 km/h, 1 degree latitude ≈ 111.32 km
	speedDegPerSec := (ac.speed * 1.852) / (111320.0 * 3600.0)

	headingRad := ac.heading * math.Pi / 180.0
	ac.lat += speedDegPerSec * math.Cos(headingRad)
	ac.lon += speedDegPerSec * math.Sin(headingRad) / math.Cos(ac.lat*math.Pi/180.0)

	// Small random heading drift (simulating real-world flight path changes)
	ac.heading += (rand.Float64() - 0.5) * 0.5
	if ac.heading < 0 {
		ac.heading += 360
	}
	if ac.heading >= 360 {
		ac.heading -= 360
	}

	// Small altitude changes
	ac.alt += rand.Intn(101) - 50
	if ac.alt < 500 {
		ac.alt = 500
	}

	// Baro rate reflects altitude changes
	ac.baroRate = (rand.Intn(5) - 2) * 64

	// RSSI varies based on distance from station
	dist := haversineNm(stationLat, stationLon, ac.lat, ac.lon)
	ac.rssi = -5.0 - dist*0.1 + (rand.Float64()-0.5)*2
	if ac.rssi < -30 {
		ac.rssi = -30
	}
}

func spawnAircraft() *liveAircraft {
	tmpl := templates[rand.Intn(len(templates))]

	// Generate hex code (6 hex digits)
	hex := fmt.Sprintf("%s%05x", tmpl.hex[:1], rand.Intn(0xFFFFF))

	// Generate callsign
	var callsign string
	if tmpl.callsignPrefix == "N" {
		// GA tail number format
		callsign = fmt.Sprintf("N%d%s", tmpl.flightNumMin+rand.Intn(tmpl.flightNumMax-tmpl.flightNumMin),
			string(rune('A'+rand.Intn(26))))
	} else {
		callsign = fmt.Sprintf("%s%d", tmpl.callsignPrefix, tmpl.flightNumMin+rand.Intn(tmpl.flightNumMax-tmpl.flightNumMin))
	}

	// Spawn at a random bearing and distance from station (20-200 nm)
	bearing := rand.Float64() * 360
	distNm := 20 + rand.Float64()*180
	lat, lon := destinationPoint(stationLat, stationLon, bearing, distNm)

	alt := tmpl.altMin + rand.Intn(tmpl.altMax-tmpl.altMin)
	speed := tmpl.speedMin + rand.Float64()*(tmpl.speedMax-tmpl.speedMin)
	heading := rand.Float64() * 360

	// Lifespan: 2-15 minutes
	lifespan := time.Duration(120+rand.Intn(780)) * time.Second

	squawks := []string{"1200", "4512", "0363", "5234", "7301", "2615", "1456", "0100"}

	return &liveAircraft{
		template:  tmpl,
		hex:       hex,
		callsign:  callsign,
		lat:       lat,
		lon:       lon,
		alt:       alt,
		speed:     speed,
		heading:   heading,
		rssi:      -10 - rand.Float64()*15,
		squawk:    squawks[rand.Intn(len(squawks))],
		spawnTime: time.Now(),
		lifespan:  lifespan,
		category:  tmpl.category,
		baroRate:  0,
	}
}

// haversineNm returns distance between two points in nautical miles.
func haversineNm(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusNm = 3440.065

	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	lat1R := lat1 * math.Pi / 180
	lat2R := lat2 * math.Pi / 180

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1R)*math.Cos(lat2R)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusNm * c
}

// destinationPoint returns the lat/lon of a point at the given bearing and
// distance (nautical miles) from the start point.
func destinationPoint(lat, lon, bearingDeg, distNm float64) (float64, float64) {
	const earthRadiusNm = 3440.065

	latR := lat * math.Pi / 180
	lonR := lon * math.Pi / 180
	brng := bearingDeg * math.Pi / 180
	d := distNm / earthRadiusNm

	newLat := math.Asin(math.Sin(latR)*math.Cos(d) +
		math.Cos(latR)*math.Sin(d)*math.Cos(brng))
	newLon := lonR + math.Atan2(
		math.Sin(brng)*math.Sin(d)*math.Cos(latR),
		math.Cos(d)-math.Sin(latR)*math.Sin(newLat))

	return newLat * 180 / math.Pi, newLon * 180 / math.Pi
}
