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

// runPipeline starts dump978-fa and reads JSON output until exit.
func (d *UATDecoder) runPipeline(ctx context.Context) error {
	serial := d.sdrSerial
	if serial == "" {
		serial = "0"
	}

	args := []string{
		"--sdr",
		"--sdr-serial", serial,
		"--sdr-gain", strconv.Itoa(d.gain),
		"--json-stdout",
	}
	if d.biasT {
		args = append(args, "--sdr-device-settings", "biastee=true")
	}

	cmd := exec.CommandContext(ctx, d.dump978Bin, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	log.Printf("[uat] starting dump978-fa: %v", args)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start dump978-fa: %w", err)
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
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.readStdout(bufio.NewScanner(stdout))
	}()

	// Wait for dump978-fa to exit.
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

// readStdout reads dump978-fa JSON output line by line. Each line is a JSON
// object. We filter to ADS-B frames (those with an "address" field and
// optionally a "type" field starting with "adsb_").
func (d *UATDecoder) readStdout(scanner *bufio.Scanner) {
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] != '{' {
			continue
		}

		// Quick filter: must contain "address" to be an aircraft frame.
		if !strings.Contains(line, `"address"`) {
			continue
		}

		// Exclude non-ADS-B frames: if a "type" field is present, it must
		// start with "adsb_" (skip TIS-B trackfile, etc. for now).
		if idx := strings.Index(line, `"type":"`); idx >= 0 {
			after := line[idx+len(`"type":"`):]
			if !strings.HasPrefix(after, "adsb_") {
				continue
			}
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
