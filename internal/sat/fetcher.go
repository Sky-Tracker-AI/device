package sat

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/skytracker/skytracker-device/internal/omni"
)

// Fetcher periodically retrieves TLE data from CelesTrak and caches to disk.
type Fetcher struct {
	mu        sync.RWMutex
	tles      map[int]*TLESet
	catalog   map[int]*omni.CatalogEntry
	client    *http.Client
	cachePath string
	refreshH  int
	onRefresh func() // called after successful TLE fetch
}

// NewFetcher creates a Fetcher that retains TLEs only for catalog satellites.
// cachePath is the file where TLEs are persisted to disk.
func NewFetcher(catalog map[int]*omni.CatalogEntry, cachePath string, refreshHours int) *Fetcher {
	if refreshHours <= 0 {
		refreshHours = 12
	}
	return &Fetcher{
		tles:      make(map[int]*TLESet),
		catalog:   catalog,
		cachePath: cachePath,
		refreshH:  refreshHours,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// Start loads TLEs from disk, fetches if needed, then refreshes periodically.
func (f *Fetcher) Start(ctx context.Context) {
	// Load from disk first.
	loaded, cacheAge := f.loadFromDisk()
	if loaded > 0 {
		log.Printf("[sat] loaded %d TLEs from disk cache (age: %v)", loaded, cacheAge.Round(time.Second))
	}

	// Only fetch immediately if cache is old or empty.
	if cacheAge > time.Duration(f.refreshH)*time.Hour || loaded == 0 {
		if err := f.FetchAll(ctx); err != nil {
			log.Printf("[sat] initial fetch error: %v", err)
		}
	} else {
		log.Printf("[sat] disk cache fresh enough, skipping immediate fetch")
	}

	ticker := time.NewTicker(time.Duration(f.refreshH) * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := f.FetchAll(ctx); err != nil {
				log.Printf("[sat] refresh error: %v", err)
			} else if f.onRefresh != nil {
				f.onRefresh()
			}
		}
	}
}

// FetchAll fetches TLE data from all CelesTrak groups and persists to disk.
func (f *Fetcher) FetchAll(ctx context.Context) error {
	groups := omni.CelesTrakGroupURLs()
	now := time.Now()
	totalFetched := 0

	for group, url := range groups {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		entries, err := f.fetchGroupTLE(ctx, url)
		if err != nil {
			log.Printf("[sat] failed to fetch group %q: %v", group, err)
			continue
		}

		kept := 0
		f.mu.Lock()
		for _, entry := range entries {
			if _, inCatalog := f.catalog[entry.NoradID]; !inCatalog {
				continue
			}
			entry.FetchedAt = now
			f.tles[entry.NoradID] = &entry
			kept++
		}
		f.mu.Unlock()

		if kept > 0 {
			log.Printf("[sat] group %q: %d/%d matched catalog", group, kept, len(entries))
		}
		totalFetched += kept
	}

	f.mu.RLock()
	total := len(f.tles)
	f.mu.RUnlock()
	log.Printf("[sat] fetch complete: %d new, %d total cached", totalFetched, total)

	// Persist to disk.
	if err := f.saveToDisk(); err != nil {
		log.Printf("[sat] failed to save TLE cache: %v", err)
	}

	return nil
}

// fetchGroupTLE retrieves TLE data in 3-line text format from CelesTrak.
func (f *Fetcher) fetchGroupTLE(ctx context.Context, url string) ([]TLESet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SkyTracker-Device/1.0")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return nil, fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, url, string(body))
	}

	return parseTLEText(resp.Body)
}

// parseTLEText parses 3-line TLE text format into TLESet entries.
func parseTLEText(r io.Reader) ([]TLESet, error) {
	var entries []TLESet
	scanner := bufio.NewScanner(r)
	var lines []string

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n ")
		if line == "" {
			continue
		}
		lines = append(lines, line)

		if len(lines) == 3 {
			name := strings.TrimSpace(lines[0])
			line1 := lines[1]
			line2 := lines[2]

			// Validate: line 1 starts with "1 ", line 2 starts with "2 ".
			if !strings.HasPrefix(line1, "1 ") || !strings.HasPrefix(line2, "2 ") {
				lines = nil
				continue
			}

			// Extract NORAD ID from line 1 (columns 3-7).
			noradStr := strings.TrimSpace(line1[2:7])
			noradID, err := strconv.Atoi(noradStr)
			if err != nil {
				lines = nil
				continue
			}

			entries = append(entries, TLESet{
				NoradID: noradID,
				Name:    name,
				Line1:   line1,
				Line2:   line2,
			})
			lines = nil
		}
	}

	if err := scanner.Err(); err != nil {
		return entries, fmt.Errorf("scanner error: %w", err)
	}
	return entries, nil
}

// GetTLE returns the cached TLE for a NORAD ID, or nil if unavailable.
func (f *Fetcher) GetTLE(noradID int) *TLESet {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.tles[noradID]
}

// GetAllTLEs returns a shallow copy of the TLE cache.
func (f *Fetcher) GetAllTLEs() map[int]*TLESet {
	f.mu.RLock()
	defer f.mu.RUnlock()

	out := make(map[int]*TLESet, len(f.tles))
	for k, v := range f.tles {
		out[k] = v
	}
	return out
}

// Count returns the number of cached TLEs.
func (f *Fetcher) Count() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.tles)
}

// CacheAge returns the age of the oldest TLE in the cache.
func (f *Fetcher) CacheAge() time.Duration {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var oldest time.Time
	for _, tle := range f.tles {
		if oldest.IsZero() || tle.FetchedAt.Before(oldest) {
			oldest = tle.FetchedAt
		}
	}
	if oldest.IsZero() {
		return 0
	}
	return time.Since(oldest)
}

// saveToDisk persists the TLE cache to the cache file as JSON.
func (f *Fetcher) saveToDisk() error {
	f.mu.RLock()
	data, err := json.Marshal(f.tles)
	n := len(f.tles)
	f.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("marshal TLEs: %w", err)
	}

	// Ensure directory exists.
	dir := filepath.Dir(f.cachePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	if err := os.WriteFile(f.cachePath, data, 0644); err != nil {
		return fmt.Errorf("write cache: %w", err)
	}

	log.Printf("[sat] saved %d TLEs to %s", n, f.cachePath)
	return nil
}

// loadFromDisk loads TLEs from the cache file. Returns count loaded and cache age.
func (f *Fetcher) loadFromDisk() (int, time.Duration) {
	data, err := os.ReadFile(f.cachePath)
	if err != nil {
		return 0, 0
	}

	var tles map[int]*TLESet
	if err := json.Unmarshal(data, &tles); err != nil {
		log.Printf("[sat] corrupt cache file %s: %v", f.cachePath, err)
		return 0, 0
	}

	// Only keep entries that are in our catalog.
	f.mu.Lock()
	loaded := 0
	var oldest time.Time
	for id, tle := range tles {
		if _, inCatalog := f.catalog[id]; !inCatalog {
			continue
		}
		f.tles[id] = tle
		loaded++
		if oldest.IsZero() || tle.FetchedAt.Before(oldest) {
			oldest = tle.FetchedAt
		}
	}
	f.mu.Unlock()

	age := time.Duration(0)
	if !oldest.IsZero() {
		age = time.Since(oldest)
	}
	return loaded, age
}
