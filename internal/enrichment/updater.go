package enrichment

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	csvURL         = "https://raw.githubusercontent.com/wiedehopf/tar1090-db/csv/aircraft.csv.gz"
	updateInterval = 7 * 24 * time.Hour
	maxDownloadSize = 50 * 1024 * 1024 // 50 MB
)

// CSVUpdater periodically downloads the latest aircraft database from
// tar1090-db and reloads the enrichment engine.
type CSVUpdater struct {
	engine  *Engine
	dataDir string
}

// NewCSVUpdater creates an updater that keeps the aircraft CSV current.
func NewCSVUpdater(engine *Engine, dataDir string) *CSVUpdater {
	return &CSVUpdater{
		engine:  engine,
		dataDir: dataDir,
	}
}

// Run checks for updates on a schedule. Blocks until ctx is cancelled.
func (u *CSVUpdater) Run(ctx context.Context) {
	// If the CSV doesn't exist yet, download immediately.
	csvPath := filepath.Join(u.dataDir, "aircraft.csv.gz")
	if _, err := os.Stat(csvPath); os.IsNotExist(err) {
		u.download(csvPath)
	}

	ticker := time.NewTicker(updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			u.download(csvPath)
		}
	}
}

func (u *CSVUpdater) download(csvPath string) {
	log.Printf("[enrichment] checking for aircraft database update")

	client := &http.Client{Timeout: 5 * time.Minute}
	req, err := http.NewRequest("GET", csvURL, nil)
	if err != nil {
		log.Printf("[enrichment] update request error: %v", err)
		return
	}

	// Use If-Modified-Since to avoid unnecessary downloads.
	if info, err := os.Stat(csvPath); err == nil {
		req.Header.Set("If-Modified-Since", info.ModTime().UTC().Format(http.TimeFormat))
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[enrichment] update download error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		log.Printf("[enrichment] aircraft database is up to date")
		return
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("[enrichment] update: unexpected status %d", resp.StatusCode)
		return
	}

	// Download to temp file, then atomic rename.
	tmpPath := csvPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		log.Printf("[enrichment] update create error: %v", err)
		return
	}

	n, err := io.Copy(out, io.LimitReader(resp.Body, maxDownloadSize))
	if syncErr := out.Sync(); syncErr != nil && err == nil {
		err = syncErr
	}
	out.Close()

	if err != nil {
		os.Remove(tmpPath)
		log.Printf("[enrichment] update write error: %v", err)
		return
	}

	if err := os.Rename(tmpPath, csvPath); err != nil {
		os.Remove(tmpPath)
		log.Printf("[enrichment] update rename error: %v", err)
		return
	}

	log.Printf("[enrichment] downloaded aircraft database (%s)", formatBytes(n))

	if err := u.engine.Reload(); err != nil {
		log.Printf("[enrichment] reload error after update: %v", err)
	}
}

func formatBytes(b int64) string {
	if b >= 1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	}
	return fmt.Sprintf("%.1f KB", float64(b)/1024)
}
