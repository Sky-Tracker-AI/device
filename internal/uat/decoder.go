package uat

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/skytracker/skytracker-device/internal/config"
)

// UATDecoder manages a continuously-running dump978-fa process on a dedicated
// SDR tuned to 978 MHz. Unlike the ACARS decoder, this runs dump978-fa directly
// (no rtl_tcp intermediary, no SatDump). Each stdout line is a JSON object
// representing a decoded UAT frame.
type UATDecoder struct {
	dump978Bin string
	sdrSerial  string
	gain       int
	biasT      bool

	mu      sync.Mutex
	running bool
	frames  chan RawFrame
	stats   DecoderStats
	started time.Time
}

// NewUATDecoder creates a decoder for the given config and SDR serial.
func NewUATDecoder(cfg config.UATConfig, sdrSerial string) *UATDecoder {
	bin := cfg.Dump978Bin
	if bin == "" {
		bin = "dump978-fa"
	}
	return &UATDecoder{
		dump978Bin: bin,
		sdrSerial:  sdrSerial,
		gain:       cfg.Gain,
		biasT:      cfg.BiasT,
		frames:     make(chan RawFrame, 1000),
	}
}

// Run is the supervisory loop. It starts the decoder pipeline and restarts
// on failure with exponential backoff. Blocks until ctx is cancelled.
func (d *UATDecoder) Run(ctx context.Context) {
	backoff := 5 * time.Second
	maxBackoff := 60 * time.Second

	for {
		if ctx.Err() != nil {
			return
		}

		startTime := time.Now()
		err := d.runPipeline(ctx)
		if ctx.Err() != nil {
			return
		}

		runtime := time.Since(startTime)
		if err != nil {
			log.Printf("[uat] pipeline exited after %v: %v", runtime, err)
		} else {
			log.Printf("[uat] pipeline exited after %v", runtime)
		}

		// Reset backoff if the pipeline ran for a reasonable duration.
		if runtime > 60*time.Second {
			backoff = 5 * time.Second
		}

		log.Printf("[uat] restarting in %v", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		// Exponential backoff.
		backoff = time.Duration(math.Min(float64(backoff*2), float64(maxBackoff)))
	}
}

// Frames returns the read-only channel of decoded raw frames.
func (d *UATDecoder) Frames() <-chan RawFrame {
	return d.frames
}

// Stats returns a snapshot of decoder statistics.
func (d *UATDecoder) Stats() DecoderStats {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := d.stats
	if d.running {
		s.UptimeSeconds = int64(time.Since(d.started).Seconds())
	}
	return s
}

// IsRunning returns whether the decoder pipeline is active.
func (d *UATDecoder) IsRunning() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.running
}

// runPipeline starts rtl_sdr piped into dump978-fa and reads JSON output until exit.
// We use rtl_sdr instead of dump978-fa's built-in SoapySDR because the SoapySDR
// RTL-SDR module (0.3.3) has a tuning bug with the R828D chip (RTL-SDR Blog V4)
// that causes zero frames to be decoded at 978 MHz.
func (d *UATDecoder) runPipeline(ctx context.Context) error {
	serial := d.sdrSerial
	if serial == "" {
		serial = "0"
	}

	// Find the rtl_sdr device index for this serial number.
	devIndex, err := d.findDeviceIndex(serial)
	if err != nil {
		return fmt.Errorf("find SDR device: %w", err)
	}

	// Use bash to create a shell pipe between rtl_sdr and dump978-fa.
	// This avoids Go pipe plumbing issues and matches the working manual test.
	shellCmd := fmt.Sprintf(
		"rtl_sdr -d %d -f 978000000 -s 2083334 -g %d - 2>/dev/null | %s --stdin --format CU8 --json-stdout",
		devIndex, d.gain, d.dump978Bin,
	)
	cmd := exec.CommandContext(ctx, "bash", "-c", shellCmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	log.Printf("[uat] starting: bash -c %q", shellCmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start pipeline: %w", err)
	}

	d.mu.Lock()
	d.running = true
	d.started = time.Now()
	d.stats.FramesDecoded = 0
	d.stats.FrameRate = 0
	d.stats.LastFrameAt = 0
	d.mu.Unlock()

	// Read stdout (decoded frames) in a goroutine.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		d.readStdout(bufio.NewScanner(stdout))
	}()

	// Log stderr for diagnostics.
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Printf("[uat] pipe: %s", scanner.Text())
		}
	}()

	// Wait for the pipeline to exit.
	err = cmd.Wait()

	d.mu.Lock()
	d.running = false
	d.mu.Unlock()

	wg.Wait()

	if err != nil {
		return fmt.Errorf("dump978-fa exited: %w", err)
	}
	return nil
}

// findDeviceIndex maps an SDR serial number to an rtl_sdr device index by
// querying rtl_test for the device list.
func (d *UATDecoder) findDeviceIndex(serial string) (int, error) {
	out, err := exec.Command("rtl_test", "-t").CombinedOutput()
	if err != nil {
		// rtl_test returns non-zero even on success; parse output anyway.
	}
	// Output format: "  0:  RTLSDRBlog, Blog V4, SN: SKT-ADS-0"
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "SN: "+serial) {
			continue
		}
		// Extract leading index before ":"
		idx := strings.IndexByte(line, ':')
		if idx > 0 {
			n, err := strconv.Atoi(strings.TrimSpace(line[:idx]))
			if err == nil {
				return n, nil
			}
		}
	}
	return 0, fmt.Errorf("no RTL-SDR device with serial %q found", serial)
}

// readStdout reads dump978-fa JSON output line by line. Each line is a JSON
// object representing either an ADS-B aircraft frame or a FIS-B uplink frame.
// Frame classification happens downstream via ClassifyFrame.
func (d *UATDecoder) readStdout(scanner *bufio.Scanner) {
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] != '{' {
			continue
		}

		d.mu.Lock()
		d.stats.FramesDecoded++
		d.stats.LastFrameAt = time.Now().UnixMilli()
		elapsed := time.Since(d.started).Minutes()
		if elapsed > 0 {
			d.stats.FrameRate = float64(d.stats.FramesDecoded) / elapsed
		}
		d.mu.Unlock()

		select {
		case d.frames <- RawFrame{Line: line}:
		default:
			log.Printf("[uat] frame channel full, dropping frame")
		}
	}
}

// IncrementFISBStats updates FIS-B product statistics by the given count.
func (d *UATDecoder) IncrementFISBStats(count int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stats.FISBProductsDecoded += count
	d.stats.FISBLastProductAt = time.Now().UnixMilli()
	elapsed := time.Since(d.started).Minutes()
	if elapsed > 0 {
		d.stats.FISBProductRate = float64(d.stats.FISBProductsDecoded) / elapsed
	}
}
