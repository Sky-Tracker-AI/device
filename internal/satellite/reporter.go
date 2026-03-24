package satellite

import (
	"context"
	"encoding/binary"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/skytracker/skytracker-device/internal/platform"
	"github.com/skytracker/skytracker-device/internal/scheduler"
)

// Reporter handles post-pass observation reporting and image upload.
type Reporter struct {
	platformFn func() *platform.Client
	stationID  string
}

// NewReporter creates a Reporter. platformFn is called on each report to get the
// current platform client (which may change after BLE registration).
func NewReporter(platformFn func() *platform.Client, stationID string) *Reporter {
	return &Reporter{
		platformFn: platformFn,
		stationID:  stationID,
	}
}

// ReportPass is called by the scheduler's OnComplete callback after a pass ends.
func (r *Reporter) ReportPass(ctx context.Context, task *scheduler.Task, outputDir string) {
	client := r.platformFn()
	if client == nil || !client.IsConfigured() {
		log.Printf("[reporter] platform not configured, skipping report for %s", task.SatName)
		return
	}

	pipeline := GetPipeline(task.NoradID)
	protocol := ""
	if pipeline != nil {
		protocol = pipeline.Protocol
	}

	// Read signal metrics from the decoder.
	var signalStrength float64
	var framesDecoded int
	if task.Decoder != nil {
		signalStrength = task.Decoder.SignalStrength()
		framesDecoded = task.Decoder.FramesDecoded()
	}

	// Scan for output images.
	images := scanImages(outputDir)
	hasImagery := len(images) > 0

	// 1. Report satellite observation.
	obs := platform.SatelliteObservation{
		NoradID:        task.NoradID,
		SatName:        task.SatName,
		SatCategory:    "weather",
		Frequency:      float64(task.FreqHz) / 1e6,
		Protocol:       protocol,
		AOS:            task.AOS.UnixMilli(),
		LOS:            task.LOS.UnixMilli(),
		MaxElevation:   task.MaxElevation,
		SignalStrength: signalStrength,
		FramesDecoded:  framesDecoded,
		HasImagery:     hasImagery,
	}

	if err := client.IngestSatellite(ctx, obs); err != nil {
		log.Printf("[reporter] failed to report observation for %s: %v", task.SatName, err)
	} else {
		log.Printf("[reporter] reported observation: %s (NORAD %d, maxEl=%.1f, snr=%.1f dB, frames=%d, imagery=%v)",
			task.SatName, task.NoradID, task.MaxElevation, signalStrength, framesDecoded, hasImagery)
	}

	// 2. Upload weather images if any.
	for _, img := range images {
		width, height := readPNGDimensions(img.path)
		info, err := os.Stat(img.path)
		if err != nil {
			log.Printf("[reporter] stat image %s: %v", img.path, err)
			continue
		}

		upload := platform.WeatherImageUpload{
			NoradID:       task.NoradID,
			SatName:       task.SatName,
			Channels:      img.channels,
			ResolutionKm:  img.resolutionKm,
			ImageWidth:    width,
			ImageHeight:   height,
			FileSizeBytes: int(info.Size()),
			CapturedAt:    task.AOS.UnixMilli(),
		}

		imageID, uploadURL, err := client.IngestWeatherImage(ctx, upload)
		if err != nil {
			log.Printf("[reporter] failed to report image for %s: %v", task.SatName, err)
			continue
		}

		// Upload file if platform provided a presigned URL.
		if uploadURL != "" {
			if err := client.UploadToPresignedURL(ctx, uploadURL, img.path); err != nil {
				log.Printf("[reporter] failed to upload image to presigned URL: %v", err)
			} else {
				log.Printf("[reporter] uploaded image: %s (%dx%d, %d bytes)", filepath.Base(img.path), width, height, info.Size())
				// Confirm upload so platform marks the record as complete.
				if err := client.ConfirmWeatherImageUpload(ctx, imageID); err != nil {
					log.Printf("[reporter] failed to confirm image upload: %v", err)
				}
			}
		} else {
			log.Printf("[reporter] image metadata reported (no upload URL): %s", filepath.Base(img.path))
		}
	}

	// 3. Cleanup output directory.
	if outputDir != "" {
		if err := os.RemoveAll(outputDir); err != nil {
			log.Printf("[reporter] cleanup failed: %v", err)
		}
	}
}

// imageInfo holds metadata about a decoded image.
type imageInfo struct {
	path         string
	channels     []string
	resolutionKm float64
}

// scanImages looks for decoded PNG files in the output directory.
func scanImages(outputDir string) []imageInfo {
	if outputDir == "" {
		return nil
	}

	var images []imageInfo
	filepath.WalkDir(outputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		name := strings.ToLower(d.Name())
		if !strings.HasSuffix(name, ".png") {
			return nil
		}
		// Skip thumbnails.
		if strings.Contains(name, "thumb") || strings.Contains(name, "icon") {
			return nil
		}

		img := imageInfo{
			path:         path,
			resolutionKm: 4.0, // Default for APT/LRPT
		}

		// Derive channel info from filename.
		img.channels = deriveChannels(name)

		images = append(images, img)
		return nil
	})
	return images
}

// deriveChannels infers channel names from the image filename.
func deriveChannels(filename string) []string {
	lower := strings.ToLower(filename)
	switch {
	case strings.Contains(lower, "avhrr"):
		return []string{"AVHRR"}
	case strings.Contains(lower, "msu-mr"):
		return []string{"MSU-MR"}
	case strings.Contains(lower, "channel_a") || strings.Contains(lower, "apt-a"):
		return []string{"APT-A"}
	case strings.Contains(lower, "channel_b") || strings.Contains(lower, "apt-b"):
		return []string{"APT-B"}
	case strings.Contains(lower, "composite") || strings.Contains(lower, "rgb"):
		return []string{"Composite"}
	default:
		return nil
	}
}

// readPNGDimensions reads width and height from a PNG file header.
func readPNGDimensions(path string) (width, height int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	// PNG header: 8-byte signature, then IHDR chunk.
	// IHDR starts at offset 8: 4-byte length + 4-byte "IHDR" + 4-byte width + 4-byte height.
	header := make([]byte, 24)
	if _, err := f.Read(header); err != nil {
		return 0, 0
	}

	// Verify PNG signature.
	pngSig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	for i := range pngSig {
		if header[i] != pngSig[i] {
			return 0, 0
		}
	}

	// Verify IHDR chunk type at offset 12.
	if string(header[12:16]) != "IHDR" {
		return 0, 0
	}

	// Width at offset 16, height at offset 20 (big-endian uint32).
	width = int(binary.BigEndian.Uint32(header[16:20]))
	height = int(binary.BigEndian.Uint32(header[20:24]))
	return width, height
}

// FormatObservationSummary returns a human-readable summary for logging.
func FormatObservationSummary(task *scheduler.Task) string {
	return fmt.Sprintf("%s (NORAD %d) maxEl=%.1f AOS=%s LOS=%s",
		task.SatName, task.NoradID, task.MaxElevation,
		task.AOS.Format("15:04:05"), task.LOS.Format("15:04:05"))
}
