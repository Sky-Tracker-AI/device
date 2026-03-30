package goes

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/skytracker/skytracker-device/internal/config"
)

// ProductType identifies a GOES scan mode.
type ProductType string

const (
	ProductFullDisk  ProductType = "full_disk"
	ProductCONUS     ProductType = "conus"
	ProductMesoscale ProductType = "mesoscale"
)

// ProductInfo describes a decoded GOES image product.
type ProductInfo struct {
	Path          string      `json:"path"`
	ProductType   ProductType `json:"product_type"`
	CompositeName string      `json:"composite_name"`
	FileSizeBytes int64       `json:"file_size_bytes"`
	DetectedAt    time.Time   `json:"detected_at"`
}

// ProductWatcher monitors the SatDump GOES output directory for new image
// products and triggers uploads according to configured cadence limits.
type ProductWatcher struct {
	outputDir         string
	maxStorageBytes   int64
	cadence           map[ProductType]time.Duration
	composites        map[ProductType][]string
	lastUpload        map[ProductType]time.Time
	seen              map[string]bool // filenames already processed
	pendingCh         chan ProductInfo
	onProduct         func(ProductInfo) // called for eligible new products

	mu             sync.Mutex
	latestProducts map[ProductType]ProductInfo
}

// NewProductWatcher creates a watcher for the given GOES output directory.
// The onProduct callback is invoked for each product eligible for upload.
func NewProductWatcher(cfg config.GOESConfig, outputDir string, onProduct func(ProductInfo)) *ProductWatcher {
	cadence := make(map[ProductType]time.Duration)
	composites := make(map[ProductType][]string)

	for pt, entry := range map[ProductType]config.GOESProductEntry{
		ProductFullDisk:  cfg.Products.FullDisk,
		ProductCONUS:     cfg.Products.CONUS,
		ProductMesoscale: cfg.Products.Mesoscale,
	} {
		if !entry.Decode {
			continue
		}
		d, err := time.ParseDuration(entry.UploadInterval)
		if err != nil || d <= 0 {
			continue // "0" or invalid = don't auto-upload
		}
		cadence[pt] = d
		composites[pt] = entry.Composites
	}

	// Pre-populate seen map with existing files so we don't re-upload
	// stale images from a previous run on restart.
	seen := make(map[string]bool)
	filepath.WalkDir(filepath.Join(outputDir, "IMAGES"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(path), ".png") {
			seen[path] = true
		}
		return nil
	})

	return &ProductWatcher{
		outputDir:         outputDir,
		maxStorageBytes:   int64(cfg.MaxLocalStorageGB) * 1024 * 1024 * 1024,
		cadence:           cadence,
		composites:        composites,
		lastUpload:        make(map[ProductType]time.Time),
		seen:              seen,
		pendingCh:         make(chan ProductInfo, 100),
		onProduct:         onProduct,
		latestProducts:    make(map[ProductType]ProductInfo),
	}
}

// Run polls the output directory for new product files. Blocks until ctx is cancelled.
func (w *ProductWatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.scan()
			w.enforceStorageLimit()
		}
	}
}

// LatestProducts returns the most recent product detected per type.
func (w *ProductWatcher) LatestProducts() map[ProductType]ProductInfo {
	w.mu.Lock()
	defer w.mu.Unlock()
	result := make(map[ProductType]ProductInfo, len(w.latestProducts))
	for k, v := range w.latestProducts {
		result[k] = v
	}
	return result
}

// scan walks the IMAGES subdirectory looking for new PNG files.
// Only scans IMAGES/ to avoid picking up EMWIN/NWS weather charts
// and other non-imagery products that SatDump outputs.
func (w *ProductWatcher) scan() {
	imagesDir := filepath.Join(w.outputDir, "IMAGES")

	var files []string
	filepath.WalkDir(imagesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(path), ".png") {
			files = append(files, path)
		}
		return nil
	})

	for _, path := range files {
		if w.seen[path] {
			continue
		}
		w.seen[path] = true

		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		product := ProductInfo{
			Path:          path,
			ProductType:   classifyProductType(path),
			CompositeName: classifyComposite(path),
			FileSizeBytes: info.Size(),
			DetectedAt:    time.Now(),
		}

		w.mu.Lock()
		w.latestProducts[product.ProductType] = product
		w.mu.Unlock()

		if w.shouldUpload(product) {
			w.lastUpload[product.ProductType] = time.Now()
			if w.onProduct != nil {
				w.onProduct(product)
			}
		}
	}
}

// shouldUpload checks whether a product is eligible for upload based on
// cadence limits and configured composite types.
func (w *ProductWatcher) shouldUpload(p ProductInfo) bool {
	interval, ok := w.cadence[p.ProductType]
	if !ok {
		return false // product type not configured for upload
	}

	// Check if composite is in the configured list.
	allowed := w.composites[p.ProductType]
	if len(allowed) > 0 && p.CompositeName != "" {
		found := false
		for _, c := range allowed {
			if c == p.CompositeName {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Enforce upload cadence.
	last := w.lastUpload[p.ProductType]
	return time.Since(last) >= interval
}

// enforceStorageLimit removes the oldest files when total size exceeds the limit.
func (w *ProductWatcher) enforceStorageLimit() {
	if w.maxStorageBytes <= 0 {
		return
	}

	type fileEntry struct {
		path    string
		size    int64
		modTime time.Time
	}

	var files []fileEntry
	var totalSize int64

	filepath.WalkDir(w.outputDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		files = append(files, fileEntry{path: path, size: info.Size(), modTime: info.ModTime()})
		totalSize += info.Size()
		return nil
	})

	if totalSize <= w.maxStorageBytes {
		return
	}

	// Sort oldest first.
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	for _, f := range files {
		if totalSize <= w.maxStorageBytes {
			break
		}
		os.Remove(f.path)
		totalSize -= f.size
		log.Printf("[goes] storage cleanup: removed %s (%d bytes)", filepath.Base(f.path), f.size)
	}
}

// classifyProductType determines the GOES product type from the file path.
// SatDump names output directories/files with identifiable patterns.
func classifyProductType(path string) ProductType {
	lower := strings.ToLower(path)
	switch {
	case strings.Contains(lower, "conus"):
		return ProductCONUS
	case strings.Contains(lower, "meso"):
		return ProductMesoscale
	default:
		// Full Disk is the default/most common product.
		return ProductFullDisk
	}
}

// classifyComposite extracts the composite name from a SatDump output filename.
// Composites are named like "abi_ABI_False_Color.png" or "abi_Cloud_Top_IR.png".
// Raw channel files are named like "G19_13_20260330T150021Z.png".
func classifyComposite(path string) string {
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, filepath.Ext(base))

	// Named composites: strip "abi_" prefix.
	if strings.HasPrefix(name, "abi_") {
		return strings.TrimPrefix(name, "abi_")
	}

	// Raw channel files: "G19_13_..." → "channel_13"
	if len(name) > 3 && (name[0] == 'G') {
		parts := strings.SplitN(name, "_", 3)
		if len(parts) >= 2 {
			return "channel_" + parts[1]
		}
	}

	return name
}
