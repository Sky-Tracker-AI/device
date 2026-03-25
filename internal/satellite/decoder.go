package satellite

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/skytracker/skytracker-device/internal/sdr"
)

var (
	// Matches "SNR : 1.866069dB" and "Peak SNR: 2.876166dB" (with or without space before dB)
	peakSNRPattern = regexp.MustCompile(`Peak SNR\s*:\s*(-?\d+\.?\d*)dB`)
	snrPattern     = regexp.MustCompile(`(?:^|,\s*)SNR\s*:\s*(-?\d+\.?\d*)\s*dB`)
	// Matches "Deframer : SYNCED" or frame/CADU counters
	deframerSyncPattern = regexp.MustCompile(`Deframer\s*:\s*SYNCED`)
	framePattern        = regexp.MustCompile(`(?:CADU|Frame)s?\s*:\s*(\d+)`)
)

// SatDumpDecoder implements scheduler.Decoder by managing a SatDump process.
// It uses rtl_tcp as an intermediary to work around an RTL-SDR Blog V4 bug
// where SatDump's direct RTL-SDR source fails to apply gain settings.
type SatDumpDecoder struct {
	noradID    int
	satName    string
	pipeline   *PipelineConfig
	satdumpBin string
	outputBase string // e.g. /tmp/skytracker-sat
	outputDir  string // per-pass directory

	mu          sync.Mutex
	cmd         *exec.Cmd
	tcpCmd      *exec.Cmd // rtl_tcp process
	running     bool
	done        chan struct{} // closed when satdump process exits
	cancel      context.CancelFunc
	peakSNR     float64
	totalFrames int
}

// NewSatDumpDecoder creates a SatDumpDecoder for the given satellite.
func NewSatDumpDecoder(noradID int, satName string, satdumpBin string, outputBase string) *SatDumpDecoder {
	return &SatDumpDecoder{
		noradID:    noradID,
		satName:    satName,
		pipeline:   GetPipeline(noradID),
		satdumpBin: satdumpBin,
		outputBase: outputBase,
	}
}

func (d *SatDumpDecoder) Name() string {
	return d.satName
}

func (d *SatDumpDecoder) Start(ctx context.Context, handle sdr.SDRHandle, freqHz int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.pipeline == nil {
		return fmt.Errorf("no pipeline config for NORAD %d", d.noradID)
	}

	// Create per-pass output directory.
	d.outputDir = filepath.Join(d.outputBase, fmt.Sprintf("%d_%d", d.noradID, time.Now().Unix()))
	if err := os.MkdirAll(d.outputDir, 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	childCtx, cancel := context.WithCancel(ctx)
	d.cancel = cancel

	// Use rtl_tcp as an intermediary to work around RTL-SDR Blog V4 bug
	// where SatDump's direct RTL-SDR source fails to apply gain/bias settings.
	// rtl_tcp handles hardware init correctly; SatDump connects as a TCP client.
	tcpPort := "7654"
	serial := handle.SerialNumber()
	if serial == "" {
		serial = "0"
	}
	tcpArgs := []string{
		"-d", serial,
		"-f", strconv.FormatInt(freqHz, 10),
		"-g", "20",
		"-s", strconv.Itoa(d.pipeline.SampleRate),
		"-p", tcpPort,
	}
	// Kill any stale rtl_tcp from a previous crash/restart before starting.
	exec.Command("pkill", "-f", "rtl_tcp.*-p "+tcpPort).Run()

	d.tcpCmd = exec.CommandContext(childCtx, "rtl_tcp", tcpArgs...)
	d.tcpCmd.Stdout = nil
	d.tcpCmd.Stderr = nil

	log.Printf("[satdump] starting rtl_tcp: %v", tcpArgs)
	if err := d.tcpCmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start rtl_tcp: %w", err)
	}

	// Give rtl_tcp time to initialize the hardware and bind the port.
	time.Sleep(2 * time.Second)

	// Build SatDump command connecting via rtl_tcp.
	args := []string{
		"live",
		d.pipeline.PipelineID,
		d.outputDir,
		"--source", "rtltcp",
		"--ip_address", "127.0.0.1",
		"--port", tcpPort,
		"--samplerate", strconv.Itoa(d.pipeline.SampleRate),
		"--frequency", strconv.FormatInt(freqHz, 10),
		"--timeout", "1200",
	}

	d.cmd = exec.CommandContext(childCtx, d.satdumpBin, args...)
	d.cmd.Stdout = nil // Discard stdout; SatDump writes to files.

	// Capture stderr for SNR and frame count parsing.
	stderr, err := d.cmd.StderrPipe()
	if err != nil {
		cancel()
		d.tcpCmd.Process.Kill()
		return fmt.Errorf("stderr pipe: %w", err)
	}

	log.Printf("[satdump] starting: %s %v", d.satdumpBin, args)

	if err := d.cmd.Start(); err != nil {
		cancel()
		d.tcpCmd.Process.Kill()
		return fmt.Errorf("start satdump: %w", err)
	}

	d.running = true
	d.peakSNR = 0
	d.totalFrames = 0
	d.done = make(chan struct{})

	// Parse stderr for signal metrics.
	go d.parseStderr(stderr)

	// Watchdog: sole owner of cmd.Wait(). Signals completion via d.done.
	go func() {
		err := d.cmd.Wait()
		d.mu.Lock()
		d.running = false
		d.mu.Unlock()
		if err != nil {
			log.Printf("[satdump] process exited: %v", err)
		} else {
			log.Printf("[satdump] process exited cleanly")
		}
		close(d.done)
	}()

	return nil
}

// parseStderr reads SatDump's stderr and extracts SNR and frame counts.
// SatDump output format (with ANSI color codes stripped by scanner):
//   Progress nan%, SNR : 1.866069dB, Peak SNR: 2.876166dB
//   Progress inf%, Viterbi : NOSYNC BER : 0.371094, Deframer : NOSYNC
func (d *SatDumpDecoder) parseStderr(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := stripANSI(scanner.Text())
		if line == "" {
			continue
		}
		log.Printf("[satdump:%s] %s", d.satName, line)

		// Prefer "Peak SNR" if present, otherwise use regular "SNR".
		if m := peakSNRPattern.FindStringSubmatch(line); len(m) > 1 {
			if snr, err := strconv.ParseFloat(m[1], 64); err == nil {
				d.mu.Lock()
				if snr > d.peakSNR {
					d.peakSNR = math.Round(snr*10) / 10
				}
				d.mu.Unlock()
			}
		} else if m := snrPattern.FindStringSubmatch(line); len(m) > 1 {
			if snr, err := strconv.ParseFloat(m[1], 64); err == nil {
				d.mu.Lock()
				if snr > d.peakSNR {
					d.peakSNR = math.Round(snr*10) / 10
				}
				d.mu.Unlock()
			}
		}

		// Count explicit frame/CADU numbers if reported.
		if m := framePattern.FindStringSubmatch(line); len(m) > 1 {
			if frames, err := strconv.Atoi(m[1]); err == nil {
				d.mu.Lock()
				if frames > d.totalFrames {
					d.totalFrames = frames
				}
				d.mu.Unlock()
			}
		}

		// Count deframer sync events as frames (SatDump reports SYNCED status periodically).
		if deframerSyncPattern.MatchString(line) {
			d.mu.Lock()
			d.totalFrames++
			d.mu.Unlock()
		}
	}
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func (d *SatDumpDecoder) Stop() error {
	d.mu.Lock()
	if !d.running || d.cmd == nil || d.cmd.Process == nil {
		d.mu.Unlock()
		return nil
	}

	log.Printf("[satdump] stopping %s (PID %d)", d.satName, d.cmd.Process.Pid)

	// Send SIGTERM for graceful exit.
	d.cmd.Process.Signal(syscall.SIGTERM)
	done := d.done
	d.mu.Unlock()

	// Wait for the watchdog goroutine to reap the process.
	// SatDump needs time to post-process and write images after SIGTERM (APT sync + PNG).
	select {
	case <-done:
		// Exited gracefully.
	case <-time.After(30 * time.Second):
		// Force kill, then wait for watchdog to finish.
		d.mu.Lock()
		if d.cmd != nil && d.cmd.Process != nil {
			log.Printf("[satdump] SIGKILL %s (PID %d)", d.satName, d.cmd.Process.Pid)
			d.cmd.Process.Kill()
		}
		d.mu.Unlock()
		<-done
	}

	d.mu.Lock()
	d.running = false
	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
	}
	snr := d.peakSNR
	frames := d.totalFrames
	d.mu.Unlock()

	// Kill rtl_tcp after satdump has exited.
	if d.tcpCmd != nil && d.tcpCmd.Process != nil {
		d.tcpCmd.Process.Kill()
		d.tcpCmd.Wait()
		d.tcpCmd = nil
	}

	log.Printf("[satdump] %s stats: peakSNR=%.1f dB, frames=%d", d.satName, snr, frames)
	return nil
}

func (d *SatDumpDecoder) IsRunning() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.running
}

func (d *SatDumpDecoder) OutputDir() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.outputDir
}

func (d *SatDumpDecoder) SignalStrength() float64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.peakSNR
}

func (d *SatDumpDecoder) FramesDecoded() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.totalFrames
}
