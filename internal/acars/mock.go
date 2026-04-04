package acars

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
)

// MockDecoder generates synthetic ACARS messages for development without hardware.
type MockDecoder struct {
	messages chan ACARSRawMessage
	mu       sync.Mutex
	running  bool
	stats    DecoderStats
}

// NewMockDecoder creates a mock decoder that produces synthetic Inmarsat ACARS traffic.
func NewMockDecoder() *MockDecoder {
	return &MockDecoder{
		messages: make(chan ACARSRawMessage, 100),
	}
}

// Run generates synthetic messages until ctx is cancelled.
func (m *MockDecoder) Run(ctx context.Context) {
	m.mu.Lock()
	m.running = true
	m.stats.Synced = true
	m.stats.PeakSNR = 14.2
	m.stats.CurrentSNR = 12.5
	m.stats.ViterbiBER = 0.00042
	startedAt := time.Now()
	m.mu.Unlock()

	log.Printf("[acars] mock decoder started")

	for {
		delay := time.Duration(2000+rand.Intn(3000)) * time.Millisecond
		select {
		case <-ctx.Done():
			m.mu.Lock()
			m.running = false
			m.mu.Unlock()
			log.Printf("[acars] mock decoder stopped")
			return
		case <-time.After(delay):
		}

		msg := m.generateMessage()
		line, err := json.Marshal(msg)
		if err != nil {
			continue
		}

		select {
		case m.messages <- ACARSRawMessage{Line: string(line)}:
			m.mu.Lock()
			m.stats.MessagesDecoded++
			m.stats.LastMessageAt = time.Now().UnixMilli()
			elapsed := time.Since(startedAt).Minutes()
			if elapsed > 0 {
				m.stats.MessageRate = float64(m.stats.MessagesDecoded) / elapsed
			}
			m.stats.UptimeSeconds = int64(time.Since(startedAt).Seconds())
			m.mu.Unlock()
		default:
		}
	}
}

// Messages returns the channel of raw messages.
func (m *MockDecoder) Messages() <-chan ACARSRawMessage {
	return m.messages
}

// Stats returns decoder statistics.
func (m *MockDecoder) Stats() DecoderStats {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stats
}

// IsRunning returns whether the mock decoder is active.
func (m *MockDecoder) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// mockAircraft represents a synthetic aircraft for mock data generation.
type mockAircraft struct {
	aesHex       string
	callsign     string
	registration string
	aircraftType string
	baseLat      float64
	baseLon      float64
	heading      float64
	altitude     int
	speed        float64
}

var mockFleet = []mockAircraft{
	{"AC0184", "BAW2156", "G-STBH", "B77W", 31.20, -47.88, 238, 40000, 512},
	{"ADB42F", "UAL955", "N77012", "B77W", 26.50, -82.30, 185, 37000, 498},
	{"A1C2D3", "DAL412", "N860DA", "A333", 28.10, -89.50, 270, 39000, 505},
	{"E40198", "AVA015", "N763AV", "A20N", 18.50, -78.20, 195, 36000, 478},
	{"AE07EA", "RCH240", "", "C17", 29.80, -86.40, 165, 32000, 450},
	{"A44B20", "FDX901", "N851FD", "B77L", 25.30, -71.60, 145, 41000, 520},
	{"A55C30", "UPS822", "N283UP", "B763", 22.70, -68.90, 210, 35000, 490},
	{"E50210", "LAN704", "CC-BGJ", "B789", 15.80, -55.30, 175, 40000, 515},
	{"3C0100", "AFR681", "F-GSQE", "B77W", 34.20, -42.50, 250, 39000, 508},
	{"3C4920", "DLH400", "D-AIMC", "A388", 38.50, -35.10, 235, 41000, 525},
}

func (m *MockDecoder) generateMessage() map[string]any {
	r := rand.Float64()
	switch {
	case r < 0.60:
		return m.generatePositionReport()
	case r < 0.85:
		return m.generateDispatch()
	case r < 0.95:
		return m.generateEGC()
	default:
		return m.generateWeather()
	}
}

func (m *MockDecoder) generatePositionReport() map[string]any {
	ac := mockFleet[rand.Intn(len(mockFleet))]
	// Add some position jitter to simulate movement.
	lat := ac.baseLat + (rand.Float64()-0.5)*2.0
	lon := ac.baseLon + (rand.Float64()-0.5)*2.0

	return map[string]any{
		"aes_id":        ac.aesHex,
		"callsign":      ac.callsign,
		"registration":  ac.registration,
		"aircraft_type": ac.aircraftType,
		"lat":           lat,
		"lon":           lon,
		"altitude":      float64(ac.altitude + (rand.Intn(2000) - 1000)),
		"heading":       ac.heading + (rand.Float64()-0.5)*10,
		"speed":         ac.speed + (rand.Float64()-0.5)*20,
		"eta_airport":   mockDestinations[rand.Intn(len(mockDestinations))],
		"text": fmt.Sprintf("AES:%s .%s LAT %s %.3f LON %s %.3f ALT %d HDG %.0f SPD %.0f",
			ac.aesHex, ac.registration,
			latDirStr(lat), absFloat(lat),
			lonDirStr(lon), absFloat(lon),
			ac.altitude, ac.heading, ac.speed),
		"frequency":       1545.0,
		"signal_strength": 10 + rand.Float64()*6,
		"sat_id":          "4F3",
		"source":          "aero",
	}
}

func (m *MockDecoder) generateDispatch() map[string]any {
	ac := mockFleet[rand.Intn(len(mockFleet))]
	msgs := []string{
		fmt.Sprintf("FI %s/AN %s GATE CHANGE: ARRIVAL GATE C%d CHANGED TO B%d.",
			ac.callsign, ac.registration, rand.Intn(40)+1, rand.Intn(30)+1),
		fmt.Sprintf("FI %s/AN %s LOADSHEET FINAL: PAX %d/0/%d CARGO %dKG FUEL %dKG",
			ac.callsign, ac.registration, 100+rand.Intn(180), rand.Intn(20), 1000+rand.Intn(5000), 10000+rand.Intn(15000)),
		fmt.Sprintf("FI %s SELCAL CHECK REQUESTED. SELCAL CODE: %s",
			ac.callsign, randomSELCAL()),
	}

	return map[string]any{
		"aes_id":          ac.aesHex,
		"callsign":        ac.callsign,
		"registration":    ac.registration,
		"aircraft_type":   ac.aircraftType,
		"label":           []string{"H1", "SA", "5Z", "Q0"}[rand.Intn(4)],
		"text":            msgs[rand.Intn(len(msgs))],
		"frequency":       1545.0,
		"signal_strength": 10 + rand.Float64()*6,
		"sat_id":          "4F3",
		"source":          "aero",
	}
}

func (m *MockDecoder) generateEGC() map[string]any {
	msgs := []string{
		"USCG NAVAREA IV 0312/26 GULF OF MEXICO. UNMANNED DRILLING PLATFORM ADRIFT. 27-14.3N 091-42.7W. MARINERS EXERCISE CAUTION.",
		"HURRICANE WARNING NR 023 TROPICAL CYCLONE 04L 041500 UTC. POSITION 25.2N 087.6W MOVEMENT NW 12KT MAX SUSTAINED WINDS 75KT.",
		"NAVAREA IV 0298/26 FLORIDA STRAIT. UNLIT BUOY REPORTED ADRIFT 24-30N 081-15W.",
		"HYDROLANT 1042/26 NORTH ATLANTIC. SUBMARINE CABLE OPERATIONS IN PROGRESS. WIDE BERTH REQUESTED.",
	}
	msg := msgs[rand.Intn(len(msgs))]

	return map[string]any{
		"text":            msg,
		"frequency":       1537.0,
		"signal_strength": 8 + rand.Float64()*4,
		"sat_id":          "4F3",
		"source":          "stdc",
	}
}

func (m *MockDecoder) generateWeather() map[string]any {
	ac := mockFleet[rand.Intn(len(mockFleet))]
	airports := []string{"KATL", "KJFK", "KORD", "KLAX", "KIAH", "KMIA", "KDFW"}
	apt := airports[rand.Intn(len(airports))]
	wind := 100 + rand.Intn(260)
	wspd := 5 + rand.Intn(25)

	return map[string]any{
		"aes_id":          ac.aesHex,
		"callsign":        ac.callsign,
		"aircraft_type":   ac.aircraftType,
		"text":            fmt.Sprintf("METAR %s 261430Z %03d%02dKT 10SM SCT045 BKN070 28/18 A2992", apt, wind, wspd),
		"frequency":       1545.0,
		"signal_strength": 10 + rand.Float64()*6,
		"sat_id":          "4F3",
		"source":          "aero",
	}
}

var mockDestinations = []string{"SBGR", "KJFK", "EGLL", "KMIA", "KIAH", "MMMX", "SKBO", "SEQM", "SPJC", "SCEL"}

func latDirStr(lat float64) string {
	if lat >= 0 {
		return "N"
	}
	return "S"
}

func lonDirStr(lon float64) string {
	if lon >= 0 {
		return "E"
	}
	return "W"
}

func randomSELCAL() string {
	chars := "ABCDEFGHJKLMPQRS"
	b := make([]byte, 4)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return fmt.Sprintf("%c%c-%c%c", b[0], b[1], b[2], b[3])
}
