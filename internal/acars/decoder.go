package acars

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/skytracker/skytracker-device/internal/config"
)

// Regex patterns for SatDump stderr parsing (shared with satellite decoder).
var (
	rePeakSNR = regexp.MustCompile(`Peak SNR\s*:\s*(-?\d+\.?\d*)dB`)
	reSNR     = regexp.MustCompile(`(?:^|,\s*)SNR\s*:\s*(-?\d+\.?\d*)\s*dB`)
	reSync    = regexp.MustCompile(`Deframer\s*:\s*SYNCED`)
	reAnsi    = regexp.MustCompile(`\x1b\[[0-9;]*m`)
)

// InmarsatDecoder manages a continuously-running SatDump Inmarsat Aero pipeline
// on a dedicated L-band SDR. Unlike the per-pass SatDumpDecoder, this runs 24/7
// with automatic restart on failure.
type InmarsatDecoder struct {
	satdumpBin string
	sdrSerial  string
	freqHz     int64
	sampleRate int
	tcpPort    string
	pipeline   string
	outputDir  string

	mu       sync.Mutex
	running  bool
	messages chan ACARSRawMessage
	stats    DecoderStats
	started  time.Time
}

// NewInmarsatDecoder creates a decoder for the given config and SDR serial.
func NewInmarsatDecoder(cfg config.ACARSConfig, satdumpBin, sdrSerial string) *InmarsatDecoder {
	return &InmarsatDecoder{
		satdumpBin: satdumpBin,
		sdrSerial:  sdrSerial,
		freqHz:     int64(cfg.FrequencyMHz * 1e6),
		sampleRate: cfg.SampleRate,
		tcpPort:    cfg.TCPPort,
		pipeline:   cfg.Pipeline,
		outputDir:  "/tmp/skytracker-acars",
		messages:   make(chan ACARSRawMessage, 1000),
	}
}

// Run is the supervisory loop. It starts the decoder pipeline and restarts
// on failure with exponential backoff. Blocks until ctx is cancelled.
func (d *InmarsatDecoder) Run(ctx context.Context) {
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
			log.Printf("[acars] pipeline exited after %v: %v", runtime, err)
		} else {
			log.Printf("[acars] pipeline exited after %v", runtime)
		}

		// Reset backoff if the pipeline ran for a reasonable duration.
		if runtime > 60*time.Second {
			backoff = 5 * time.Second
		}

		log.Printf("[acars] restarting in %v", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		// Exponential backoff.
		backoff = time.Duration(math.Min(float64(backoff*2), float64(maxBackoff)))
	}
}

// Messages returns the read-only channel of decoded raw messages.
func (d *InmarsatDecoder) Messages() <-chan ACARSRawMessage {
	return d.messages
}

// Stats returns a snapshot of decoder statistics.
func (d *InmarsatDecoder) Stats() DecoderStats {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := d.stats
	if d.running {
		s.UptimeSeconds = int64(time.Since(d.started).Seconds())
	}
	return s
}

// IsRunning returns whether the decoder pipeline is active.
func (d *InmarsatDecoder) IsRunning() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.running
}

// runPipeline starts rtl_tcp + SatDump and reads output until exit.
func (d *InmarsatDecoder) runPipeline(ctx context.Context) error {
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Kill any stale rtl_tcp on our port from a previous crash.
	exec.Command("pkill", "-f", "rtl_tcp.*-p "+d.tcpPort).Run()
	time.Sleep(500 * time.Millisecond)

	// Start rtl_tcp as intermediary (RTL-SDR Blog V4 gain workaround).
	serial := d.sdrSerial
	if serial == "" {
		serial = "0"
	}
	// Offset the tuner by 200 kHz to move the RTL-SDR DC spike away from the
	// signal. SatDump's --freq_shift compensates so the demodulator still finds
	// the real carrier.
	const dcOffset int64 = 200000
	tcpArgs := []string{
		"-d", serial,
		"-f", strconv.FormatInt(d.freqHz-dcOffset, 10),
		"-g", "40",
		"-s", strconv.Itoa(d.sampleRate),
		"-p", d.tcpPort,
		"-T",
	}

	tcpCmd := exec.CommandContext(childCtx, "rtl_tcp", tcpArgs...)
	tcpCmd.Stdout = nil
	tcpCmd.Stderr = nil

	log.Printf("[acars] starting rtl_tcp: %v", tcpArgs)
	if err := tcpCmd.Start(); err != nil {
		return fmt.Errorf("start rtl_tcp: %w", err)
	}
	defer func() {
		if tcpCmd.Process != nil {
			tcpCmd.Process.Kill()
		}
	}()

	// Give rtl_tcp time to initialize hardware and bind port.
	time.Sleep(2 * time.Second)

	// Clean stale output from previous runs.
	os.RemoveAll(d.outputDir)
	os.MkdirAll(d.outputDir, 0755)

	// Start SatDump Inmarsat pipeline (2.0 moved "live" under "legacy" subcommand).
	args := []string{
		"legacy",
		"live",
		d.pipeline,
		d.outputDir,
		"--source", "rtltcp",
		"--ip_address", "127.0.0.1",
		"--port", d.tcpPort,
		"--samplerate", strconv.Itoa(d.sampleRate),
		"--frequency", strconv.FormatInt(d.freqHz-dcOffset, 10),
		"--freq_shift", strconv.FormatInt(dcOffset, 10),
	}

	cmd := exec.CommandContext(childCtx, d.satdumpBin, args...)
	cmd.Dir = "/usr/share/satdump" // SatDump 2.0 needs its resource directory as cwd.

	// SatDump crashes if HOME is unset (null std::string in config path).
	if os.Getenv("HOME") == "" {
		cmd.Env = append(os.Environ(), "HOME=/root")
	}

	// Capture stdout for decoded ACARS messages (JSON lines).
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	// Capture stderr for SNR/sync metrics.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	log.Printf("[acars] starting satdump: %v", args)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start satdump: %w", err)
	}

	d.mu.Lock()
	d.running = true
	d.started = time.Now()
	d.stats.MessagesDecoded = 0
	d.stats.MessageRate = 0
	d.stats.PeakSNR = 0
	d.stats.CurrentSNR = 0
	d.stats.Synced = false
	d.mu.Unlock()

	// Parse stdout (decoded messages) in a goroutine.
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		d.readStdout(bufio.NewScanner(stdout))
	}()

	go func() {
		defer wg.Done()
		d.readStderr(bufio.NewScanner(stderr))
	}()

	// Wait for SatDump to exit.
	err = cmd.Wait()

	d.mu.Lock()
	d.running = false
	d.mu.Unlock()

	wg.Wait()

	if err != nil {
		return fmt.Errorf("satdump exited: %w", err)
	}
	return nil
}

// readStdout reads SatDump's decoded ACARS message output line by line.
func (d *InmarsatDecoder) readStdout(scanner *bufio.Scanner) {
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] != '{' {
			continue
		}

		d.mu.Lock()
		d.stats.MessagesDecoded++
		d.stats.LastMessageAt = time.Now().UnixMilli()
		elapsed := time.Since(d.started).Minutes()
		if elapsed > 0 {
			d.stats.MessageRate = float64(d.stats.MessagesDecoded) / elapsed
		}
		d.mu.Unlock()

		select {
		case d.messages <- ACARSRawMessage{Line: line}:
		default:
			log.Printf("[acars] message channel full, dropping message")
		}
	}
}

// readStderr parses SatDump's stderr for SNR and sync status metrics.
func (d *InmarsatDecoder) readStderr(scanner *bufio.Scanner) {
	for scanner.Scan() {
		line := reAnsi.ReplaceAllString(scanner.Text(), "")

		// Check for Peak SNR.
		if m := rePeakSNR.FindStringSubmatch(line); len(m) > 1 {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				d.mu.Lock()
				d.stats.CurrentSNR = v
				if v > d.stats.PeakSNR {
					d.stats.PeakSNR = v
				}
				d.mu.Unlock()
			}
			continue
		}

		// Check for regular SNR.
		if m := reSNR.FindStringSubmatch(line); len(m) > 1 {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				d.mu.Lock()
				d.stats.CurrentSNR = v
				if v > d.stats.PeakSNR {
					d.stats.PeakSNR = v
				}
				d.mu.Unlock()
			}
			continue
		}

		// Check for deframer sync.
		if reSync.MatchString(line) {
			d.mu.Lock()
			d.stats.Synced = true
			d.mu.Unlock()
		}
	}
}
