package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
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

// IngestRoute is a SWIM-sourced route returned in the ingest response.
type IngestRoute struct {
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
}

// IngestResponse contains enrichment data returned from the platform.
type IngestResponse struct {
	Accepted     int                       `json:"accepted"`
	RarityScores map[string]int            `json:"rarity_scores,omitempty"`
	Routes       map[string]IngestRoute    `json:"routes,omitempty"` // callsign → route
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
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
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
	Uptime         int64    `json:"uptime_seconds"`
	GPSFix         bool     `json:"gps_fix"`
	Lat            float64  `json:"lat"`
	Lon            float64  `json:"lon"`
	AircraftCount  int      `json:"aircraft_count"`
	AgentVersion   string   `json:"agent_version"`
	LastSync       int64    `json:"last_sync_timestamp"`
	QueueSize      int      `json:"queue_size"`

	// Omni extensions (omitted by ADS-B-only devices).
	SDRCount           int              `json:"sdr_count,omitempty"`
	SDRSerials         []string         `json:"sdr_serials,omitempty"`
	TLEAge             int64            `json:"tle_age_seconds,omitempty"`
	TLECount           int              `json:"tle_count,omitempty"`
	SatellitePasses24h int              `json:"satellite_passes_24h,omitempty"`
	UpcomingPasses     []UpcomingPass   `json:"upcoming_passes,omitempty"`
	SchedulerState     string           `json:"scheduler_state,omitempty"`
	ActiveDecoder      string           `json:"active_decoder,omitempty"`
	OmniMode           string           `json:"omni_mode,omitempty"`

	// Signal types declares active capabilities (e.g. ["adsb", "satellite", "acars"]).
	SignalTypes []string `json:"signal_types,omitempty"`

	// ACARS extensions (omitted when ACARS is not enabled).
	ACARSEnabled      bool    `json:"acars_enabled,omitempty"`
	ACARSMessageCount int     `json:"acars_message_count,omitempty"`
	ACARSMessageRate  float64 `json:"acars_message_rate,omitempty"`
	ACARSDecoderState string  `json:"acars_decoder_state,omitempty"` // "running", "stopped", "restarting"
	ACARSSNR          float64 `json:"acars_snr,omitempty"`
	ACARSSatellite    string  `json:"acars_satellite,omitempty"`
	ACARSFrequency    float64 `json:"acars_frequency,omitempty"`

	// GOES HRIT extensions (omitted when GOES is not enabled).
	GOESEnabled       bool    `json:"goes_enabled,omitempty"`
	GOESSatellite     string  `json:"goes_satellite,omitempty"`     // "GOES-16" or "GOES-18"
	GOESSNR           float64 `json:"goes_snr_db,omitempty"`
	GOESViterbiBER    float64 `json:"goes_viterbi_ber,omitempty"`
	GOESRSCorrections int     `json:"goes_rs_corrections,omitempty"`
	GOESFramesDecoded int     `json:"goes_frames_decoded,omitempty"`
	GOESDecoderState  string  `json:"goes_decoder_state,omitempty"` // "running", "stopped", "restarting"
	GOESImages24h     int     `json:"goes_images_24h,omitempty"`
}

// UpcomingPass is a device-predicted satellite pass sent in the health report.
type UpcomingPass struct {
	NoradID      int     `json:"norad_id"`
	SatName      string  `json:"sat_name"`
	Frequency    float64 `json:"frequency"`
	AOSPredicted int64   `json:"aos_predicted"`
	LOSPredicted int64   `json:"los_predicted"`
	MaxElevation float64 `json:"max_elevation"`
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
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("health: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &result, nil
}

// SatelliteObservation matches platform models.SatelliteObservation.
type SatelliteObservation struct {
	NoradID        int     `json:"norad_id"`
	SatName        string  `json:"sat_name"`
	SatCategory    string  `json:"sat_category,omitempty"`
	Frequency      float64 `json:"frequency"`
	Protocol       string  `json:"protocol,omitempty"`
	AOS            int64   `json:"aos"`
	LOS            int64   `json:"los"`
	MaxElevation   float64 `json:"max_elevation"`
	SignalStrength float64 `json:"signal_strength,omitempty"`
	FramesDecoded  int     `json:"frames_decoded,omitempty"`
	HasImagery     bool    `json:"has_imagery,omitempty"`
}

// WeatherImageUpload matches platform models.WeatherImageUpload.
type WeatherImageUpload struct {
	NoradID       int      `json:"norad_id"`
	SatName       string   `json:"sat_name"`
	Channels      []string `json:"channels,omitempty"`
	ResolutionKm  float64  `json:"resolution_km,omitempty"`
	ImageWidth    int      `json:"image_width,omitempty"`
	ImageHeight   int      `json:"image_height,omitempty"`
	FileSizeBytes int      `json:"file_size_bytes"`
	CapturedAt    int64    `json:"captured_at"`

	// GOES HRIT extensions (omitted for LEO imagery).
	SourceType    string `json:"source_type,omitempty"`    // "lrpt", "apt", or "hrit"
	ProductType   string `json:"product_type,omitempty"`   // GOES: "full_disk", "conus", "mesoscale"
	CompositeName string `json:"composite_name,omitempty"` // e.g. "true_color", "ir_enhanced"
}

// satelliteIngestRequest wraps a satellite observation for the ingest endpoint.
type satelliteIngestRequest struct {
	SourceType string               `json:"source_type"`
	Satellite  *SatelliteObservation `json:"satellite"`
}

// weatherImageIngestRequest wraps a weather image upload for the ingest endpoint.
type weatherImageIngestRequest struct {
	SourceType   string              `json:"source_type"`
	WeatherImage *WeatherImageUpload `json:"weather_image"`
}

// weatherImageIngestResponse holds the platform response for weather image ingest.
type weatherImageIngestResponse struct {
	Accepted  int    `json:"accepted"`
	ImageID   string `json:"image_id,omitempty"`
	UploadURL string `json:"upload_url,omitempty"`
}

// IngestSatellite reports a decoded satellite pass.
func (c *Client) IngestSatellite(ctx context.Context, obs SatelliteObservation) error {
	if !c.IsConfigured() {
		return fmt.Errorf("not configured (no API key)")
	}

	req := satelliteIngestRequest{
		SourceType: "satellite",
		Satellite:  &obs,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/api/v1/ingest", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("ingest satellite: status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// IngestWeatherImage reports decoded weather imagery metadata.
// Returns the image ID and a presigned upload URL if the platform provides one.
func (c *Client) IngestWeatherImage(ctx context.Context, img WeatherImageUpload) (imageID, uploadURL string, err error) {
	if !c.IsConfigured() {
		return "", "", fmt.Errorf("not configured (no API key)")
	}

	req := weatherImageIngestRequest{
		SourceType:   "weather_image",
		WeatherImage: &img,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", "", fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/api/v1/ingest", bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", "", fmt.Errorf("ingest weather image: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result weatherImageIngestResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("decode: %w", err)
	}
	return result.ImageID, result.UploadURL, nil
}

// UploadToPresignedURL PUTs a file to a presigned R2 URL.
func (c *Client) UploadToPresignedURL(ctx context.Context, url string, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "PUT", url, f)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	httpReq.ContentLength = stat.Size()
	httpReq.Header.Set("Content-Type", "image/png")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("upload: status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// ConfirmWeatherImageUpload tells the platform that the image upload to R2 is complete.
func (c *Client) ConfirmWeatherImageUpload(ctx context.Context, imageID string) error {
	if !c.IsConfigured() {
		return fmt.Errorf("not configured (no API key)")
	}

	body, err := json.Marshal(map[string]string{
		"source_type": "weather_image_confirm",
		"image_id":    imageID,
	})
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/api/v1/ingest", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("confirm weather image: status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// ACARSIngestMessage is a single ACARS message for batch upload.
type ACARSIngestMessage struct {
	Timestamp      int64   `json:"timestamp"`
	Source         string  `json:"source"`                    // "aero" or "stdc"
	MessageType    string  `json:"message_type"`              // pos, acars, wx, oooi, mil, sar, nav, egc
	AESHex         string  `json:"aes_hex,omitempty"`
	ICAOHex        string  `json:"icao_hex,omitempty"`
	Callsign       string  `json:"callsign,omitempty"`
	Registration   string  `json:"registration,omitempty"`
	AircraftType   string  `json:"aircraft_type,omitempty"`
	Lat            float64 `json:"lat,omitempty"`
	Lon            float64 `json:"lon,omitempty"`
	Altitude       int     `json:"altitude,omitempty"`
	Heading        float64 `json:"heading,omitempty"`
	Speed          float64 `json:"speed,omitempty"`
	ETAAirport     string  `json:"eta_airport,omitempty"`
	ETATime        int64   `json:"eta_time,omitempty"`
	Label          string  `json:"label,omitempty"`
	Sublabel       string  `json:"sublabel,omitempty"`
	RawText        string  `json:"raw_text"`
	DecodedSummary string  `json:"decoded_summary,omitempty"`
	Frequency      float64 `json:"frequency,omitempty"`
	SignalStrength float64 `json:"signal_strength,omitempty"`
	SatID          string  `json:"sat_id,omitempty"`
	Channel        string  `json:"channel,omitempty"`
	OOOIEvent      string  `json:"oooi_event,omitempty"`
	OOOIAirport    string  `json:"oooi_airport,omitempty"`
}

// acarsIngestRequest wraps ACARS messages for the ingest endpoint.
type acarsIngestRequest struct {
	SourceType    string               `json:"source_type"`
	ACARSMessages []ACARSIngestMessage `json:"acars_messages"`
}

// IngestACARSMessages sends a batch of ACARS messages to the platform.
func (c *Client) IngestACARSMessages(ctx context.Context, msgs []ACARSIngestMessage) error {
	if !c.IsConfigured() {
		return fmt.Errorf("not configured (no API key)")
	}

	req := acarsIngestRequest{
		SourceType:    "acars",
		ACARSMessages: msgs,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/api/v1/ingest", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("ingest acars: status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// LogConnectivity logs the platform connection status (for debugging).
func (c *Client) LogConnectivity() {
	if c.IsConfigured() {
		log.Printf("[platform] configured, endpoint=%s", c.endpoint)
	} else {
		log.Printf("[platform] not configured (no API key) — running in offline mode")
	}
}
