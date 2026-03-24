package satellite

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/skytracker/skytracker-device/internal/platform"
	"github.com/skytracker/skytracker-device/internal/scheduler"
	"github.com/skytracker/skytracker-device/internal/sdr"
)

func TestReporterReportPass(t *testing.T) {
	var mu sync.Mutex
	var receivedRequests []map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		receivedRequests = append(receivedRequests, body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"accepted": 1})
	}))
	defer srv.Close()

	client := platform.NewClient(srv.URL, "test-api-key")
	reporter := NewReporter(func() *platform.Client { return client }, "station-123")

	// Create a mock output directory with a PNG image.
	outputDir := t.TempDir()
	imagesDir := filepath.Join(outputDir, "IMAGES")
	os.MkdirAll(imagesDir, 0755)
	createMockPNG(t, filepath.Join(imagesDir, "avhrr_composite.png"), 2080, 1500)

	task := &scheduler.Task{
		ID:           "task-1",
		NoradID:      33591,
		SatName:      "NOAA 19",
		Priority:     scheduler.PriorityHigh,
		FreqHz:       137100000,
		AOS:          time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC),
		LOS:          time.Date(2024, 3, 1, 12, 15, 0, 0, time.UTC),
		MaxElevation: 45.2,
		State:        scheduler.TaskCompleted,
		SDR:          &sdr.MockSDRHandle{MockID: "sdr-0", MockSerial: "SKT-0", MockTuner: "R820T"},
	}

	reporter.ReportPass(context.Background(), task, outputDir)

	mu.Lock()
	defer mu.Unlock()

	// Should have received 2 requests: satellite observation + weather image.
	if len(receivedRequests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(receivedRequests))
	}

	// Check satellite observation request.
	satReq := receivedRequests[0]
	if satReq["source_type"] != "satellite" {
		t.Errorf("first request source_type = %v, want satellite", satReq["source_type"])
	}
	sat, ok := satReq["satellite"].(map[string]interface{})
	if !ok {
		t.Fatal("satellite field missing or wrong type")
	}
	if int(sat["norad_id"].(float64)) != 33591 {
		t.Errorf("norad_id = %v, want 33591", sat["norad_id"])
	}
	if sat["sat_name"] != "NOAA 19" {
		t.Errorf("sat_name = %v, want NOAA 19", sat["sat_name"])
	}
	if sat["has_imagery"] != true {
		t.Errorf("has_imagery = %v, want true", sat["has_imagery"])
	}

	// Check weather image request.
	imgReq := receivedRequests[1]
	if imgReq["source_type"] != "weather_image" {
		t.Errorf("second request source_type = %v, want weather_image", imgReq["source_type"])
	}
}

func TestReporterReportPassNoImages(t *testing.T) {
	var mu sync.Mutex
	var requestCount int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"accepted": 1})
	}))
	defer srv.Close()

	client := platform.NewClient(srv.URL, "test-api-key")
	reporter := NewReporter(func() *platform.Client { return client }, "station-123")

	// Empty output directory — no images.
	outputDir := t.TempDir()

	task := &scheduler.Task{
		ID:           "task-1",
		NoradID:      25338,
		SatName:      "NOAA 15",
		Priority:     scheduler.PriorityHigh,
		FreqHz:       137620000,
		AOS:          time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC),
		LOS:          time.Date(2024, 3, 1, 12, 10, 0, 0, time.UTC),
		MaxElevation: 30.0,
		State:        scheduler.TaskCompleted,
		SDR:          &sdr.MockSDRHandle{MockID: "sdr-0", MockSerial: "SKT-0", MockTuner: "R820T"},
	}

	reporter.ReportPass(context.Background(), task, outputDir)

	mu.Lock()
	defer mu.Unlock()

	// Should have 1 request (satellite observation only, no images).
	if requestCount != 1 {
		t.Errorf("expected 1 request (observation only), got %d", requestCount)
	}
}

func TestReporterPlatformError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer srv.Close()

	client := platform.NewClient(srv.URL, "test-api-key")
	reporter := NewReporter(func() *platform.Client { return client }, "station-123")

	task := &scheduler.Task{
		ID:      "task-1",
		NoradID: 25338,
		SatName: "NOAA 15",
		FreqHz:  137620000,
		AOS:     time.Now().Add(-10 * time.Minute),
		LOS:     time.Now(),
		SDR:     &sdr.MockSDRHandle{MockID: "sdr-0", MockSerial: "SKT-0", MockTuner: "R820T"},
	}

	// Should not panic — just log errors.
	reporter.ReportPass(context.Background(), task, "")
}

func TestScanImages(t *testing.T) {
	dir := t.TempDir()
	imagesDir := filepath.Join(dir, "IMAGES")
	os.MkdirAll(imagesDir, 0755)

	// Create test files.
	createMockPNG(t, filepath.Join(imagesDir, "avhrr_composite.png"), 100, 100)
	createMockPNG(t, filepath.Join(imagesDir, "channel_a.png"), 100, 100)
	os.WriteFile(filepath.Join(imagesDir, "thumb_small.png"), []byte("not a real png"), 0644)
	os.WriteFile(filepath.Join(imagesDir, "data.bin"), []byte("binary data"), 0644)

	images := scanImages(dir)

	// Should find 2 images (avhrr_composite + channel_a, skip thumb).
	if len(images) != 2 {
		t.Errorf("found %d images, want 2", len(images))
	}
}

func TestScanImagesEmptyDir(t *testing.T) {
	images := scanImages("")
	if len(images) != 0 {
		t.Errorf("expected 0 images for empty dir, got %d", len(images))
	}
}

func TestDeriveChannels(t *testing.T) {
	tests := []struct {
		filename string
		want     []string
	}{
		{"avhrr_123_rgb.png", []string{"AVHRR"}},
		{"msu-mr_output.png", []string{"MSU-MR"}},
		{"channel_a_enhanced.png", []string{"APT-A"}},
		{"apt-b_ir.png", []string{"APT-B"}},
		{"composite_final.png", []string{"Composite"}},
		{"rgb_image.png", []string{"Composite"}},
		{"unknown_output.png", nil},
	}

	for _, tt := range tests {
		got := deriveChannels(tt.filename)
		if len(got) != len(tt.want) {
			t.Errorf("deriveChannels(%q) = %v, want %v", tt.filename, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("deriveChannels(%q)[%d] = %q, want %q", tt.filename, i, got[i], tt.want[i])
			}
		}
	}
}

func TestReadPNGDimensions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.png")
	createMockPNG(t, path, 2080, 1500)

	w, h := readPNGDimensions(path)
	if w != 2080 || h != 1500 {
		t.Errorf("readPNGDimensions = (%d, %d), want (2080, 1500)", w, h)
	}
}

func TestReadPNGDimensionsInvalidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not_a_png.png")
	os.WriteFile(path, []byte("not a png file"), 0644)

	w, h := readPNGDimensions(path)
	if w != 0 || h != 0 {
		t.Errorf("readPNGDimensions invalid = (%d, %d), want (0, 0)", w, h)
	}
}

// createMockPNG creates a minimal valid PNG file with the given dimensions.
func createMockPNG(t *testing.T, path string, width, height int) {
	t.Helper()

	// Minimal PNG: signature + IHDR chunk (no image data needed for header parsing).
	var data []byte
	// PNG signature.
	data = append(data, 0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A)
	// IHDR chunk: length (13 bytes).
	lenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBytes, 13)
	data = append(data, lenBytes...)
	// IHDR type.
	data = append(data, 'I', 'H', 'D', 'R')
	// Width.
	wBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(wBytes, uint32(width))
	data = append(data, wBytes...)
	// Height.
	hBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(hBytes, uint32(height))
	data = append(data, hBytes...)
	// Bit depth, color type, compression, filter, interlace.
	data = append(data, 8, 2, 0, 0, 0)
	// CRC (fake).
	data = append(data, 0, 0, 0, 0)

	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}
