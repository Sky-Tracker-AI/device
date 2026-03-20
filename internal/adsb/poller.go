package adsb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"sync"
	"time"
)

// Poller polls a dump1090-fa endpoint at a configured interval and keeps
// a snapshot of the latest aircraft data.
type Poller struct {
	url      string
	interval time.Duration
	client   *http.Client

	mu       sync.RWMutex
	aircraft []Aircraft
	lastPoll time.Time
}

// NewPoller creates a new dump1090-fa poller.
func NewPoller(url string, intervalMS int) *Poller {
	return &Poller{
		url:      url,
		interval: time.Duration(intervalMS) * time.Millisecond,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Run starts the polling loop. It blocks until ctx is cancelled.
func (p *Poller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	p.poll()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll()
		}
	}
}

// Aircraft returns the latest snapshot of aircraft data.
func (p *Poller) Aircraft() []Aircraft {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]Aircraft, len(p.aircraft))
	copy(out, p.aircraft)
	return out
}

// LastPoll returns the time of the last successful poll.
func (p *Poller) LastPoll() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastPoll
}

func (p *Poller) poll() {
	resp, err := p.client.Get(p.url)
	if err != nil {
		log.Printf("[adsb] poll error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[adsb] poll: unexpected status %d", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[adsb] read error: %v", err)
		return
	}

	var data Dump1090Response
	if err := json.Unmarshal(body, &data); err != nil {
		log.Printf("[adsb] parse error: %v", err)
		return
	}

	p.mu.Lock()
	p.aircraft = data.Aircraft
	p.lastPoll = time.Now()
	p.mu.Unlock()
}

// MockPoller generates synthetic aircraft data for development without
// real hardware.
type MockPoller struct {
	interval   time.Duration
	stationLat float64
	stationLon float64

	mu       sync.RWMutex
	aircraft []Aircraft
	mocks    []mockAircraft
}

type mockAircraft struct {
	hex       string
	callsign  string
	lat       float64
	lon       float64
	altitude  int
	speed     float64
	heading   float64
	turnRate  float64 // degrees per second
	climbRate int     // feet per minute
}

// NewMockPoller creates a poller that generates synthetic data.
func NewMockPoller(stationLat, stationLon float64, intervalMS int) *MockPoller {
	mp := &MockPoller{
		interval:   time.Duration(intervalMS) * time.Millisecond,
		stationLat: stationLat,
		stationLon: stationLon,
	}
	mp.mocks = mp.generateMockFleet()
	return mp
}

func (mp *MockPoller) generateMockFleet() []mockAircraft {
	fleet := []struct {
		hex      string
		callsign string
		altMin   int
		altMax   int
		speed    float64
	}{
		{"A0B1C2", "SWA1234", 28000, 38000, 450},
		{"A1D2E3", "UAL445", 30000, 40000, 470},
		{"A2F3A4", "DAL88", 33000, 39000, 480},
		{"A3B4C5", "AAL2201", 25000, 37000, 460},
		{"A4D5E6", "JBU614", 29000, 36000, 440},
		{"A5A6B7", "N172SP", 2500, 5500, 110},
		{"A6C7D8", "SKW5432", 18000, 28000, 380},
		{"A7E8F9", "FDX1021", 34000, 41000, 490},
		{"A8A9B0", "N524JT", 8000, 12000, 180},
		{"A9C0D1", "ASA331", 31000, 37000, 460},
		{"B0E1F2", "FFT402", 15000, 22000, 320},
		{"B1A2B3", "NKS917", 27000, 35000, 440},
		{"B2C3A4", "EVAC01", 16000, 20000, 350},
		{"B3B4C5", "N78ND", 3500, 6000, 130},
		{"B4D5E6", "UPS234", 35000, 42000, 500},
	}

	mocks := make([]mockAircraft, len(fleet))
	for i, f := range fleet {
		angle := float64(i) * (360.0 / float64(len(fleet)))
		dist := 5.0 + float64(i%10)*15.0

		angleRad := angle * math.Pi / 180.0
		stationLatRad := mp.stationLat * math.Pi / 180.0

		latOff := dist * math.Cos(angleRad) / 60.0
		lonOff := dist * math.Sin(angleRad) / (60.0 * math.Cos(stationLatRad))

		alt := f.altMin + (i*1231)%(f.altMax-f.altMin)

		mocks[i] = mockAircraft{
			hex:       f.hex,
			callsign:  f.callsign,
			lat:       mp.stationLat + latOff,
			lon:       mp.stationLon + lonOff,
			altitude:  alt,
			speed:     f.speed + float64(i%5)*5,
			heading:   math.Mod(angle+90, 360),
			turnRate:  float64(i%3) * 0.5,
			climbRate: (i%5 - 2) * 200,
		}
	}
	return mocks
}

// Run starts generating synthetic data. Blocks until ctx is cancelled.
func (mp *MockPoller) Run(ctx context.Context) {
	ticker := time.NewTicker(mp.interval)
	defer ticker.Stop()

	mp.update()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mp.update()
		}
	}
}

func (mp *MockPoller) update() {
	dtSec := mp.interval.Seconds()

	result := make([]Aircraft, 0, len(mp.mocks))
	for i := range mp.mocks {
		m := &mp.mocks[i]

		headingRad := m.heading * math.Pi / 180.0
		latRad := m.lat * math.Pi / 180.0

		nmPerSec := m.speed / 3600.0
		dist := nmPerSec * dtSec

		latDelta := dist * math.Cos(headingRad) / 60.0
		lonDelta := dist * math.Sin(headingRad) / (60.0 * math.Cos(latRad))
		m.lat += latDelta
		m.lon += lonDelta

		m.heading += m.turnRate * dtSec
		if m.heading >= 360 {
			m.heading -= 360
		}
		if m.heading < 0 {
			m.heading += 360
		}

		m.altitude += int(float64(m.climbRate) * dtSec / 60.0)
		if m.altitude < 1000 {
			m.altitude = 1000
			m.climbRate = intAbs(m.climbRate)
		}
		if m.altitude > 45000 {
			m.altitude = 45000
			m.climbRate = -intAbs(m.climbRate)
		}

		// Wrap aircraft that go too far from the station.
		dLat := (m.lat - mp.stationLat) * 60
		stationLatRad := mp.stationLat * math.Pi / 180.0
		dLon := (m.lon - mp.stationLon) * 60 * math.Cos(stationLatRad)
		distNM := math.Sqrt(dLat*dLat + dLon*dLon)
		if distNM > 250 {
			m.heading = math.Mod(m.heading+180, 360)
		}

		alt := m.altitude
		speed := m.speed
		heading := m.heading
		lat := m.lat
		lon := m.lon
		callsign := fmt.Sprintf("%-8s", m.callsign)

		result = append(result, Aircraft{
			Hex:     m.hex,
			Flight:  callsign,
			AltBaro: &FlexInt{Value: &alt},
			GS:      &speed,
			Track:   &heading,
			Lat:     &lat,
			Lon:     &lon,
			Seen:    0.5,
			SeenPos: 0.5,
		})
	}

	mp.mu.Lock()
	mp.aircraft = result
	mp.mu.Unlock()
}

// Aircraft returns the latest mock aircraft data.
func (mp *MockPoller) Aircraft() []Aircraft {
	mp.mu.RLock()
	defer mp.mu.RUnlock()
	out := make([]Aircraft, len(mp.aircraft))
	copy(out, mp.aircraft)
	return out
}

func intAbs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
