package gpsd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// Position represents a GPS fix.
type Position struct {
	Lat       float64
	Lon       float64
	Altitude  float64 // meters
	Speed     float64 // m/s
	Timestamp time.Time
	HasFix    bool
}

// tpvReport is a gpsd TPV (time-position-velocity) JSON report.
type tpvReport struct {
	Class  string  `json:"class"`
	Mode   int     `json:"mode"`
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	Alt    float64 `json:"alt"`
	Speed  float64 `json:"speed"`
	Time   string  `json:"time"`
}

// Client connects to gpsd and reads GPS position updates.
type Client struct {
	host string
	port int

	mu  sync.RWMutex
	pos Position
}

// NewClient creates a new gpsd client.
func NewClient(host string, port int) *Client {
	return &Client{
		host: host,
		port: port,
	}
}

// Position returns the most recent GPS fix.
func (c *Client) Position() Position {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.pos
}

// Run connects to gpsd and reads position updates. Blocks until ctx is
// cancelled. Reconnects automatically on connection loss.
func (c *Client) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := c.connect(ctx)
		if err != nil {
			log.Printf("[gpsd] connection error: %v, retrying in 5s", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func (c *Client) connect(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", c.host, c.port)
	dialer := net.Dialer{Timeout: 5 * time.Second}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	log.Printf("[gpsd] connected to %s", addr)

	// Send the WATCH command to enable JSON reporting.
	_, err = fmt.Fprintf(conn, `?WATCH={"enable":true,"json":true}`)
	if err != nil {
		return fmt.Errorf("watch command: %w", err)
	}

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line := scanner.Bytes()
		var report tpvReport
		if err := json.Unmarshal(line, &report); err != nil {
			continue
		}

		if report.Class != "TPV" {
			continue
		}

		pos := Position{
			Lat:      report.Lat,
			Lon:      report.Lon,
			Altitude: report.Alt,
			Speed:    report.Speed,
			HasFix:   report.Mode >= 2,
		}
		if report.Time != "" {
			if t, err := time.Parse(time.RFC3339, report.Time); err == nil {
				pos.Timestamp = t
			}
		}

		c.mu.Lock()
		c.pos = pos
		c.mu.Unlock()
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read: %w", err)
	}
	return fmt.Errorf("connection closed")
}

// MockClient returns a fixed position for development.
type MockClient struct {
	pos Position
}

// NewMockClient creates a mock GPS client at the given coordinates.
func NewMockClient(lat, lon float64) *MockClient {
	return &MockClient{
		pos: Position{
			Lat:       lat,
			Lon:       lon,
			Altitude:  30.0,
			HasFix:    true,
			Timestamp: time.Now(),
		},
	}
}

// Position returns the fixed mock position.
func (m *MockClient) Position() Position {
	return m.pos
}

// Run is a no-op for the mock client.
func (m *MockClient) Run(ctx context.Context) {
	<-ctx.Done()
}
