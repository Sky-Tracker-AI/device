package goes

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
	"sync"
	"time"

	"github.com/skytracker/skytracker-device/internal/config"
)

// Regex patterns for SatDump GOES HRIT stderr parsing.
var (
	reViterbiBER   = regexp.MustCompile(`Viterbi\s*:\s*SYNCED,?\s*BER\s*:\s*([0-9.eE+-]+)`)
	reRSCorrections = regexp.MustCompile(`RS.*?Corrections?\s*:\s*(\d+)`)
	rePeakSNR      = regexp.MustCompile(`Peak SNR\s*:\s*(-?\d+\.?\d*)dB`)
	reSNR          = regexp.MustCompile(`(?:^|,\s*)SNR\s*:\s*(-?\d+\.?\d*)\s*dB`)
	reFrameCount   = regexp.MustCompile(`(?:CADU|Frame)s?\s*:\s*(\d+)`)
	reDeframerSync = regexp.MustCompile(`Deframer\s*:\s*SYNCED`)
	reAnsi         = regexp.MustCompile(`\x1b\[[0-9;]*m`)
)

// DecoderStats holds thread-safe signal quality metrics for the GOES decoder.
type DecoderStats struct {
	PeakSNR        float64 `json:"peak_snr"`
	CurrentSNR     float64 `json:"current_snr"`
	ViterbiBER     float64 `json:"viterbi_ber"`
	RSCorrections  int     `json:"rs_corrections"`
	FramesDecoded  int     `json:"frames_decoded"`
	Synced         bool    `json:"synced"`
	UptimeSeconds  int64   `json:"uptime_seconds"`
}

// Decoder manages a continuously-running SatDump GOES HRIT pipeline
// on a dedicated L-band SDR. Follows the same supervisor pattern as
// the Inmarsat ACARS decoder.
type Decoder struct {
	satdumpBin string
	sdrSerial  string
	freqHz     int64
	sampleRate int
	tcpPort    string
	pipeline   string
	outputDir  string

	mu      sync.Mutex
	running bool
	stats   DecoderStats
	started time.Time
}

// NewDecoder creates a GOES HRIT decoder for the given config and SDR serial.
func NewDecoder(cfg config.GOESConfig, satdumpBin, sdrSerial string) *Decoder {
	return &Decoder{
		satdumpBin: satdumpBin,
		sdrSerial:  sdrSerial,
		freqHz:     int64(cfg.FrequencyMHz * 1e6),
		sampleRate: cfg.SampleRate,
		tcpPort:    cfg.TCPPort,
		pipeline:   cfg.Pipeline,
		outputDir:  "/tmp/skytracker-goes",
	}
}

// Run is the supervisory loop. It starts the HRIT pipeline and restarts
// on failure with exponential backoff. Blocks until ctx is cancelled.
func (d *Decoder) Run(ctx context.Context) {
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
			log.Printf("[goes] pipeline exited after %v: %v", runtime, err)
		} else {
			log.Printf("[goes] pipeline exited after %v", runtime)
		}

		if runtime > 60*time.Second {
			backoff = 5 * time.Second
		}

		log.Printf("[goes] restarting in %v", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		backoff = time.Duration(math.Min(float64(backoff*2), float64(maxBackoff)))
	}
}

// Stats returns a snapshot of decoder statistics.
func (d *Decoder) Stats() DecoderStats {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := d.stats
	if d.running {
		s.UptimeSeconds = int64(time.Since(d.started).Seconds())
	}
	return s
}

// IsRunning returns whether the HRIT pipeline is active.
func (d *Decoder) IsRunning() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.running
}

// OutputDir returns the path where SatDump writes decoded GOES products.
func (d *Decoder) OutputDir() string {
	return d.outputDir
}

// runPipeline starts rtl_tcp + SatDump and reads stderr until exit.
func (d *Decoder) runPipeline(ctx context.Context) error {
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Kill any stale rtl_tcp on our port from a previous crash.
	exec.Command("pkill", "-f", "rtl_tcp.*-p "+d.tcpPort).Run()
	time.Sleep(500 * time.Millisecond)

	serial := d.sdrSerial
	if serial == "" {
		serial = "0"
	}

	// Tune directly to the HRIT frequency. The E4000 tuner's narrow IF
	// bandwidth cannot tolerate the 200 kHz DC offset used for R820T tuners.
	// The DC spike at center is handled by SatDump's demodulator.
	tcpArgs := []string{
		"-d", serial,
		"-f", strconv.FormatInt(d.freqHz, 10),
		"-g", "42",
		"-s", strconv.Itoa(d.sampleRate),
		"-p", d.tcpPort,
		"-T",
	}

	tcpCmd := exec.CommandContext(childCtx, "rtl_tcp", tcpArgs...)
	tcpCmd.Stdout = nil
	tcpCmd.Stderr = nil

	log.Printf("[goes] starting rtl_tcp: %v", tcpArgs)
	if err := tcpCmd.Start(); err != nil {
		return fmt.Errorf("start rtl_tcp: %w", err)
	}
	defer func() {
		if tcpCmd.Process != nil {
			tcpCmd.Process.Kill()
		}
	}()

	time.Sleep(2 * time.Second)

	// Ensure output directory exists.
	os.MkdirAll(d.outputDir, 0755)

	// Start SatDump GOES HRIT pipeline.
	args := []string{
		"legacy",
		"live",
		d.pipeline,
		d.outputDir,
		"--source", "rtltcp",
		"--ip_address", "127.0.0.1",
		"--port", d.tcpPort,
		"--samplerate", strconv.Itoa(d.sampleRate),
		"--frequency", strconv.FormatInt(d.freqHz, 10),
		"--gain", "42",
		"--bias",
	}

	cmd := exec.CommandContext(childCtx, d.satdumpBin, args...)
	cmd.Dir = "/usr/share/satdump"

	// Build environment for SatDump. Include the plugins directory in
	// LD_LIBRARY_PATH so that libgoes_support.so can resolve its
	// libxrit_support.so dependency. Without this, SatDump's double
	// plugin-directory scan fails to deduplicate the GOES plugin and
	// crashes with a duplicate CLI subcommand error.
	env := os.Environ()
	pluginDir := "/usr/share/satdump/plugins"
	if existing := os.Getenv("LD_LIBRARY_PATH"); existing != "" {
		env = append(env, "LD_LIBRARY_PATH="+pluginDir+":"+existing)
	} else {
		env = append(env, "LD_LIBRARY_PATH="+pluginDir)
	}
	if os.Getenv("HOME") == "" {
		env = append(env, "HOME=/root")
	}
	cmd.Env = env

	cmd.Stdout = nil // GOES HRIT outputs files, not JSON on stdout.

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	log.Printf("[goes] starting satdump: %v", args)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start satdump: %w", err)
	}

	d.mu.Lock()
	d.running = true
	d.started = time.Now()
	d.stats = DecoderStats{}
	d.mu.Unlock()

	d.readStderr(bufio.NewScanner(stderr))

	err = cmd.Wait()

	d.mu.Lock()
	d.running = false
	d.mu.Unlock()

	if err != nil {
		return fmt.Errorf("satdump exited: %w", err)
	}
	return nil
}

// readStderr parses SatDump's stderr for signal quality metrics.
func (d *Decoder) readStderr(scanner *bufio.Scanner) {
	for scanner.Scan() {
		line := reAnsi.ReplaceAllString(scanner.Text(), "")

		// Peak SNR.
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

		// Regular SNR.
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

		// Viterbi BER.
		if m := reViterbiBER.FindStringSubmatch(line); len(m) > 1 {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				d.mu.Lock()
				d.stats.ViterbiBER = v
				d.mu.Unlock()
			}
		}

		// Reed-Solomon corrections.
		if m := reRSCorrections.FindStringSubmatch(line); len(m) > 1 {
			if v, err := strconv.Atoi(m[1]); err == nil {
				d.mu.Lock()
				d.stats.RSCorrections = v
				d.mu.Unlock()
			}
		}

		// Frame count.
		if m := reFrameCount.FindStringSubmatch(line); len(m) > 1 {
			if v, err := strconv.Atoi(m[1]); err == nil {
				d.mu.Lock()
				if v > d.stats.FramesDecoded {
					d.stats.FramesDecoded = v
				}
				d.mu.Unlock()
			}
		}

		// Deframer sync.
		if reDeframerSync.MatchString(line) {
			d.mu.Lock()
			d.stats.Synced = true
			d.mu.Unlock()
		}
	}
}
