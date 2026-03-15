package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	defaultCheckInterval = 24 * time.Hour
	githubReleasesAPI    = "https://api.github.com/repos/Sky-Tracker-AI/device/releases/latest"
)

// Updater checks for and applies OTA updates from GitHub Releases.
type Updater struct {
	currentVersion string
	binaryPath     string
	stagingDir     string
	checkInterval  time.Duration
}

// GithubRelease represents the relevant fields from a GitHub release.
type GithubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []GithubAsset `json:"assets"`
}

// GithubAsset represents a downloadable asset in a release.
type GithubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// New creates a new updater.
func New(currentVersion, binaryPath string) *Updater {
	stagingDir := filepath.Join(filepath.Dir(binaryPath), ".skytracker-update")
	return &Updater{
		currentVersion: currentVersion,
		binaryPath:     binaryPath,
		stagingDir:     stagingDir,
		checkInterval:  defaultCheckInterval,
	}
}

// Run checks for updates on a schedule. Blocks until ctx is cancelled.
func (u *Updater) Run(ctx context.Context) {
	// Check once shortly after startup.
	select {
	case <-ctx.Done():
		return
	case <-time.After(30 * time.Second):
	}

	u.checkAndStage()

	ticker := time.NewTicker(u.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			u.checkAndStage()
		}
	}
}

func (u *Updater) checkAndStage() {
	release, err := u.fetchLatestRelease()
	if err != nil {
		log.Printf("[updater] check error: %v", err)
		return
	}

	if release.TagName == "" || release.TagName == u.currentVersion {
		log.Printf("[updater] up to date (current: %s)", u.currentVersion)
		return
	}

	// Compare versions (simple string comparison, versions are semver-like).
	if !isNewer(release.TagName, u.currentVersion) {
		return
	}

	log.Printf("[updater] new version available: %s (current: %s)", release.TagName, u.currentVersion)

	// Find the appropriate binary asset for this platform.
	assetName := fmt.Sprintf("skytracker-agent-%s-%s", runtime.GOOS, runtime.GOARCH)
	checksumName := assetName + ".sha256"

	var binaryAsset, checksumAsset *GithubAsset
	for i := range release.Assets {
		if release.Assets[i].Name == assetName {
			binaryAsset = &release.Assets[i]
		}
		if release.Assets[i].Name == checksumName {
			checksumAsset = &release.Assets[i]
		}
	}

	if binaryAsset == nil {
		log.Printf("[updater] no binary asset found for %s/%s", runtime.GOOS, runtime.GOARCH)
		return
	}

	// Download and verify.
	if err := u.downloadAndStage(binaryAsset, checksumAsset); err != nil {
		log.Printf("[updater] download error: %v", err)
		return
	}

	log.Printf("[updater] update staged successfully, will apply on next restart")
}

func (u *Updater) fetchLatestRelease() (*GithubRelease, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(githubReleasesAPI)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var release GithubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &release, nil
}

func (u *Updater) downloadAndStage(binary *GithubAsset, checksum *GithubAsset) error {
	if err := os.MkdirAll(u.stagingDir, 0755); err != nil {
		return fmt.Errorf("mkdir staging: %w", err)
	}

	// Download binary.
	binaryPath := filepath.Join(u.stagingDir, "skytracker-agent-new")
	if err := downloadFile(binary.BrowserDownloadURL, binaryPath); err != nil {
		return fmt.Errorf("download binary: %w", err)
	}

	// Download and verify checksum.
	if checksum != nil {
		checksumPath := filepath.Join(u.stagingDir, "checksum.sha256")
		if err := downloadFile(checksum.BrowserDownloadURL, checksumPath); err != nil {
			os.Remove(binaryPath)
			return fmt.Errorf("download checksum: %w", err)
		}

		expectedHash, err := os.ReadFile(checksumPath)
		if err != nil {
			os.Remove(binaryPath)
			return fmt.Errorf("read checksum: %w", err)
		}

		actualHash, err := fileSHA256(binaryPath)
		if err != nil {
			os.Remove(binaryPath)
			return fmt.Errorf("compute hash: %w", err)
		}

		expected := strings.TrimSpace(strings.Fields(string(expectedHash))[0])
		if actualHash != expected {
			os.Remove(binaryPath)
			return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actualHash)
		}

		log.Printf("[updater] checksum verified: %s", actualHash)
	}

	// Make the binary executable.
	if err := os.Chmod(binaryPath, 0755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	return nil
}

// ApplyStaged checks if a staged update exists and applies it by replacing
// the current binary. Returns true if an update was applied.
func (u *Updater) ApplyStaged() bool {
	newBinary := filepath.Join(u.stagingDir, "skytracker-agent-new")
	if _, err := os.Stat(newBinary); os.IsNotExist(err) {
		return false
	}

	// Back up the current binary.
	backupPath := u.binaryPath + ".prev"
	if err := copyFile(u.binaryPath, backupPath); err != nil {
		log.Printf("[updater] backup error: %v", err)
		return false
	}

	// Replace with new binary.
	if err := os.Rename(newBinary, u.binaryPath); err != nil {
		log.Printf("[updater] replace error: %v, rolling back", err)
		os.Rename(backupPath, u.binaryPath)
		return false
	}

	// Clean up staging directory.
	os.RemoveAll(u.stagingDir)

	log.Printf("[updater] update applied successfully")
	return true
}

// Rollback reverts to the previous version.
func (u *Updater) Rollback() error {
	backupPath := u.binaryPath + ".prev"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("no previous version available")
	}

	if err := os.Rename(backupPath, u.binaryPath); err != nil {
		return fmt.Errorf("rollback: %w", err)
	}

	log.Printf("[updater] rolled back to previous version")
	return nil
}

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func isNewer(tag, current string) bool {
	// Strip leading 'v' for comparison.
	tag = strings.TrimPrefix(tag, "v")
	current = strings.TrimPrefix(current, "v")
	// Simple lexicographic comparison works for semver with same-length parts.
	return tag > current
}
