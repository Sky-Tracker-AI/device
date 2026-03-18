package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// Client communicates with the skytracker.ai platform API.
type Client struct {
	endpoint string
	apiKey   string
	client   *http.Client
}

// NewClient creates a new platform API client.
func NewClient(endpoint, apiKey string) *Client {
	return &Client{
		endpoint: endpoint,
		apiKey:   apiKey,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// IsConfigured returns true if the client has a valid API key.
func (c *Client) IsConfigured() bool {
	return c.apiKey != ""
}

// RegisterRequest is the payload for device registration.
type RegisterRequest struct {
	Serial       string  `json:"serial"`
	HardwareInfo string  `json:"hardware_info"`
	OSVersion    string  `json:"os_version"`
	AgentVersion string  `json:"agent_version"`
	Lat          float64 `json:"lat"`
	Lon          float64 `json:"lon"`
}

// RegisterResponse is the response from device registration.
type RegisterResponse struct {
	DeviceID  string `json:"device_id"`
	APIKey    string `json:"api_key"`
	StationID string `json:"station_id"`
	ClaimCode string `json:"claim_code"`
}

// Register sends a device registration request.
func (c *Client) Register(ctx context.Context, req RegisterRequest) (*RegisterResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/api/v1/devices/register", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("register: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result RegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &result, nil
}

// IngestSighting is a single aircraft sighting for batch upload.
type IngestSighting struct {
	StationID string  `json:"station_id"`
	Timestamp int64   `json:"timestamp"`
	ICAOHex   string  `json:"icao_hex"`
	Callsign  string  `json:"callsign"`
	Type      string  `json:"type"`
	Altitude  int     `json:"altitude"`
	Speed     float64 `json:"speed"`
	Heading   float64 `json:"heading"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	Distance  float64 `json:"distance"`
	VertRate  int     `json:"vert_rate"`
	Squawk    string  `json:"squawk"`
}

// IngestRequest is the payload for the batch ingest endpoint.
type IngestRequest struct {
	Sightings []IngestSighting `json:"sightings"`
}

// IngestResponse contains enrichment data returned from the platform.
type IngestResponse struct {
	RarityScores map[string]int `json:"rarity_scores"` // icao_hex → score
	Accepted     int            `json:"accepted"`
}

// Ingest sends a batch of aircraft sightings to the platform.
func (c *Client) Ingest(ctx context.Context, req IngestRequest) (*IngestResponse, error) {
	if !c.IsConfigured() {
		return nil, fmt.Errorf("not configured (no API key)")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/api/v1/ingest", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ingest: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result IngestResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &result, nil
}

// HealthRequest is the payload for device health reporting.
type HealthRequest struct {
	Uptime        int64   `json:"uptime_seconds"`
	GPSFix        bool    `json:"gps_fix"`
	Lat           float64 `json:"lat"`
	Lon           float64 `json:"lon"`
	AircraftCount int     `json:"aircraft_count"`
	AgentVersion  string  `json:"agent_version"`
	LastSync      int64   `json:"last_sync_timestamp"`
	QueueSize     int     `json:"queue_size"`
}

// HealthResponse contains any updates from the platform.
type HealthResponse struct {
	UpdateAvailable bool   `json:"update_available"`
	LatestVersion   string `json:"latest_version,omitempty"`
	DownloadURL     string `json:"download_url,omitempty"`
	Checksum        string `json:"checksum,omitempty"`
	SharingPref     string `json:"sharing_pref,omitempty"` // if user changed on website
	Claimed         bool   `json:"claimed"`
	StationName     string `json:"station_name,omitempty"`
}

// Health sends a health report to the platform.
func (c *Client) Health(ctx context.Context, req HealthRequest) (*HealthResponse, error) {
	if !c.IsConfigured() {
		return nil, fmt.Errorf("not configured (no API key)")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/api/v1/devices/health", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("health: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &result, nil
}

// LogConnectivity logs the platform connection status (for debugging).
func (c *Client) LogConnectivity() {
	if c.IsConfigured() {
		log.Printf("[platform] configured, endpoint=%s", c.endpoint)
	} else {
		log.Printf("[platform] not configured (no API key) — running in offline mode")
	}
}
