package uat

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"
)

// MockUATDecoder generates synthetic GA aircraft frames for development
// without hardware. Implements the same interface as UATDecoder.
type MockUATDecoder struct {
	frames chan RawFrame
	mu     sync.Mutex
	running bool
	stats   DecoderStats
}

// NewMockUATDecoder creates a mock decoder that produces synthetic 978 MHz UAT traffic.
func NewMockUATDecoder() *MockUATDecoder {
	return &MockUATDecoder{
		frames: make(chan RawFrame, 100),
	}
}

// mockUATAircraft represents a synthetic GA aircraft for mock data generation.
type mockUATAircraft struct {
	address  string
	flight   string
	typeCode string
	baseLat  float64
	baseLon  float64
	heading  float64
	altitude int
	speed    float64
}

var mockUATFleet = []mockUATAircraft{
	{"A00001", "N172SP", "C172", 30.27, -97.74, 270, 3500, 110},
	{"A00002", "N28PK", "PA28", 30.35, -97.68, 180, 4500, 125},
	{"A00003", "N922CD", "SR22", 30.20, -97.80, 90, 6000, 170},
	{"A00004", "N36BE", "BE36", 30.40, -97.60, 315, 8000, 165},
	{"A00005", "N208EX", "C208", 30.15, -97.90, 45, 10000, 155},
	{"A00006", "N44RH", "R44", 30.30, -97.70, 200, 1500, 90},
	{"A00007", "N7RV", "RV7", 30.25, -97.85, 135, 5500, 160},
	{"A00008", "N46PM", "PA46", 30.45, -97.55, 225, 12000, 195},
}

// Run generates synthetic UAT frames until ctx is cancelled.
func (m *MockUATDecoder) Run(ctx context.Context) {
	m.mu.Lock()
	m.running = true
	startedAt := time.Now()
	m.mu.Unlock()

	log.Printf("[uat] mock decoder started")

	for {
		delay := time.Duration(1000+rand.Intn(1000)) * time.Millisecond
		select {
		case <-ctx.Done():
			m.mu.Lock()
			m.running = false
			m.mu.Unlock()
			log.Printf("[uat] mock decoder stopped")
			return
		case <-time.After(delay):
		}

		frame := m.generateFrame()
		line, err := json.Marshal(frame)
		if err != nil {
			continue
		}

		select {
		case m.frames <- RawFrame{Line: string(line)}:
			m.mu.Lock()
			m.stats.FramesDecoded++
			m.stats.LastFrameAt = time.Now().UnixMilli()
			elapsed := time.Since(startedAt).Minutes()
			if elapsed > 0 {
				m.stats.FrameRate = float64(m.stats.FramesDecoded) / elapsed
			}
			m.stats.UptimeSeconds = int64(time.Since(startedAt).Seconds())
			m.mu.Unlock()
		default:
		}
	}
}

// Frames returns the channel of raw frames.
func (m *MockUATDecoder) Frames() <-chan RawFrame {
	return m.frames
}

// Stats returns decoder statistics.
func (m *MockUATDecoder) Stats() DecoderStats {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stats
}

// IsRunning returns whether the mock decoder is active.
func (m *MockUATDecoder) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// generateFrame builds a dump978-style JSON object for a random mock aircraft.
func (m *MockUATDecoder) generateFrame() map[string]any {
	ac := mockUATFleet[rand.Intn(len(mockUATFleet))]

	// Orbit around base position with random jitter.
	elapsed := float64(time.Now().UnixMilli()) / 60000.0
	orbitRadius := 0.02 + rand.Float64()*0.01
	lat := ac.baseLat + orbitRadius*math.Sin(elapsed+float64(ac.altitude))
	lon := ac.baseLon + orbitRadius*math.Cos(elapsed+float64(ac.altitude))

	// Vary altitude slightly.
	alt := ac.altitude + (rand.Intn(400) - 200)

	// Vary heading slightly.
	track := ac.heading + (rand.Float64()-0.5)*10
	if track < 0 {
		track += 360
	}
	if track >= 360 {
		track -= 360
	}

	// Vary ground speed slightly.
	gs := ac.speed + (rand.Float64()-0.5)*10

	// Vertical rate: mostly level, occasionally climbing/descending.
	vertRate := 0
	if rand.Float64() < 0.3 {
		vertRate = (rand.Intn(10) - 5) * 100
	}

	return map[string]any{
		"type":      "adsb_icao",
		"address":   ac.address,
		"lat":       math.Round(lat*1e6) / 1e6,
		"lon":       math.Round(lon*1e6) / 1e6,
		"alt_baro":  alt,
		"gs":        math.Round(gs*10) / 10,
		"track":     math.Round(track*10) / 10,
		"vert_rate": vertRate,
		"squawk":    "1200",
		"flight":    ac.flight,
		"category":  "A1",
		"timestamp":  float64(time.Now().UnixMilli()) / 1000.0,
	}
}
