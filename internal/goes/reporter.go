package goes

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/skytracker/skytracker-device/internal/platform"
)

// NORAD catalog numbers for GOES satellites.
const (
	NoradGOES16 = 41866
	NoradGOES18 = 51850
	NoradGOES19 = 61861
)

// Reporter uploads GOES HRIT imagery to the platform.
type Reporter struct {
	platformFn func() *platform.Client
	stationID  func() string
	satellite  string
}

// NewReporter creates a Reporter for GOES image uploads.
func NewReporter(platformFn func() *platform.Client, stationID func() string, satellite string) *Reporter {
	return &Reporter{
		platformFn: platformFn,
		stationID:  stationID,
		satellite:  satellite,
	}
}

// Upload handles the 3-step upload for a single GOES product:
// 1. Ingest metadata → get presigned URL
// 2. PUT file to R2
// 3. Confirm upload
func (r *Reporter) Upload(ctx context.Context, product ProductInfo) {
	client := r.platformFn()
	if client == nil || !client.IsConfigured() {
		return
	}

	info, err := os.Stat(product.Path)
	if err != nil {
		log.Printf("[goes] stat image %s: %v", product.Path, err)
		return
	}

	width, height := readPNGDimensions(product.Path)

	noradID := NoradGOES18
	switch r.satellite {
	case "GOES-16":
		noradID = NoradGOES16
	case "GOES-19":
		noradID = NoradGOES19
	}

	upload := platform.WeatherImageUpload{
		NoradID:       noradID,
		SatName:       r.satellite,
		SourceType:    "hrit",
		ProductType:   string(product.ProductType),
		CompositeName: product.CompositeName,
		ImageWidth:    width,
		ImageHeight:   height,
		FileSizeBytes: int(info.Size()),
		CapturedAt:    time.Now().UnixMilli(),
	}

	imageID, uploadURL, err := client.IngestWeatherImage(ctx, upload)
	if err != nil {
		log.Printf("[goes] ingest image failed: %v", err)
		return
	}

	if uploadURL == "" {
		log.Printf("[goes] image metadata reported (no upload URL): %s", filepath.Base(product.Path))
		return
	}

	if err := client.UploadToPresignedURL(ctx, uploadURL, product.Path); err != nil {
		log.Printf("[goes] upload failed: %v", err)
		return
	}

	if err := client.ConfirmWeatherImageUpload(ctx, imageID); err != nil {
		log.Printf("[goes] confirm upload failed: %v", err)
		return
	}

	log.Printf("[goes] uploaded %s %s (%dx%d, %d bytes)",
		product.ProductType, product.CompositeName, width, height, info.Size())
}

// readPNGDimensions reads width and height from a PNG file header.
// Duplicated from satellite/reporter.go to avoid cross-package dependency.
func readPNGDimensions(path string) (width, height int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	header := make([]byte, 24)
	if _, err := f.Read(header); err != nil {
		return 0, 0
	}

	pngSig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	for i := range pngSig {
		if header[i] != pngSig[i] {
			return 0, 0
		}
	}

	if len(header) < 24 || string(header[12:16]) != "IHDR" {
		return 0, 0
	}

	width = int(uint32(header[16])<<24 | uint32(header[17])<<16 | uint32(header[18])<<8 | uint32(header[19]))
	height = int(uint32(header[20])<<24 | uint32(header[21])<<16 | uint32(header[22])<<8 | uint32(header[23]))
	return width, height
}
