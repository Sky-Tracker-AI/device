package satellite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skytracker/skytracker-device/internal/sdr"
)

func TestSatDumpDecoderName(t *testing.T) {
	d := NewSatDumpDecoder(57166, "METEOR-M N2-3", "satdump", t.TempDir())
	if d.Name() != "METEOR-M N2-3" {
		t.Errorf("Name() = %q, want METEOR-M N2-3", d.Name())
	}
}

func TestSatDumpDecoderNoPipeline(t *testing.T) {
	d := NewSatDumpDecoder(99999, "UNKNOWN", "satdump", t.TempDir())
	handle := &sdr.MockSDRHandle{MockID: "sdr-0", MockSerial: "SKT-0", MockTuner: "R820T"}
	err := d.Start(context.Background(), handle, 137100000)
	if err == nil {
		t.Fatal("expected error for unknown pipeline, got nil")
	}
}

func TestSatDumpDecoderOutputDir(t *testing.T) {
	d := NewSatDumpDecoder(57166, "METEOR-M N2-3", "satdump", t.TempDir())
	if d.OutputDir() != "" {
		t.Errorf("OutputDir() before Start = %q, want empty", d.OutputDir())
	}
}

func TestSatDumpDecoderWithMockBinary(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mock "rtl_tcp" that listens on the requested port and waits.
	mockTCP := filepath.Join(tmpDir, "rtl_tcp")
	tcpScript := `#!/bin/sh
# Mock rtl_tcp: parse -p flag for port, listen with nc, then wait.
PORT=7654
while [ $# -gt 0 ]; do
  case "$1" in -p) PORT="$2"; shift;; esac
  shift
done
# Use a simple sleep — nc may not be available in all test environments.
sleep 60
`
	if err := os.WriteFile(mockTCP, []byte(tcpScript), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a mock "satdump" script that creates an output file and exits.
	mockBin := filepath.Join(tmpDir, "satdump")
	script := `#!/bin/sh
# Mock SatDump: create output directory structure and wait.
mkdir -p "$3/IMAGES"
echo "mock image" > "$3/IMAGES/test.png"
sleep 30
`
	if err := os.WriteFile(mockBin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	// Put mock binaries on PATH so the decoder finds rtl_tcp.
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	outputBase := filepath.Join(tmpDir, "output")
	d := NewSatDumpDecoder(57166, "METEOR-M N2-3", mockBin, outputBase)
	handle := &sdr.MockSDRHandle{MockID: "sdr-0", MockSerial: "SKT-0", MockTuner: "R820T"}

	err := d.Start(context.Background(), handle, 137900000)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if !d.IsRunning() {
		t.Error("expected running after Start")
	}

	outDir := d.OutputDir()
	if outDir == "" {
		t.Fatal("OutputDir() is empty after Start")
	}

	// Verify output directory was created.
	if _, err := os.Stat(outDir); os.IsNotExist(err) {
		t.Errorf("output directory %s does not exist", outDir)
	}

	// Stop the decoder.
	if err := d.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if d.IsRunning() {
		t.Error("expected not running after Stop")
	}
}

func TestParseStderrMeteorLRPT(t *testing.T) {
	d := NewSatDumpDecoder(59051, "METEOR-M N2-4", "satdump", t.TempDir())

	// Simulate real SatDump LRPT stderr output (with ANSI codes).
	input := strings.Join([]string{
		"\x1b[32m(I) Start processing...\x1b[m",
		"\x1b[32m(I) Progress nan%, SNR : 1.866069dB, Peak SNR: 2.876166dB\x1b[m",
		"\x1b[32m(I) Progress inf%, Viterbi : NOSYNC BER : 0.371094, Deframer : NOSYNC\x1b[m",
		"\x1b[32m(I) Progress nan%, SNR : 5.234000dB, Peak SNR: 8.500000dB\x1b[m",
		"\x1b[32m(I) Progress inf%, Viterbi : SYNCED BER : 0.001000, Deframer : SYNCED\x1b[m",
		"\x1b[32m(I) Progress nan%, SNR : 4.100000dB, Peak SNR: 12.300000dB\x1b[m",
		"\x1b[32m(I) Progress inf%, Viterbi : SYNCED BER : 0.000500, Deframer : SYNCED\x1b[m",
		"\x1b[32m(I) Done! Goodbye\x1b[m",
	}, "\n")

	d.parseStderr(strings.NewReader(input))

	if d.peakSNR != 12.3 {
		t.Errorf("peakSNR = %.1f, want 12.3", d.peakSNR)
	}
	if d.totalFrames != 2 {
		t.Errorf("totalFrames = %d, want 2 (two SYNCED events)", d.totalFrames)
	}
}

func TestParseStderrWithExplicitFrameCount(t *testing.T) {
	d := NewSatDumpDecoder(57166, "METEOR-M N2-3", "satdump", t.TempDir())

	input := strings.Join([]string{
		"(I) Progress 50%, SNR : 10.5dB, Peak SNR: 15.2dB",
		"(I) CADUs : 1234",
		"(I) Frames : 5678",
	}, "\n")

	d.parseStderr(strings.NewReader(input))

	if d.peakSNR != 15.2 {
		t.Errorf("peakSNR = %.1f, want 15.2", d.peakSNR)
	}
	if d.totalFrames != 5678 {
		t.Errorf("totalFrames = %d, want 5678", d.totalFrames)
	}
}

func TestStripANSI(t *testing.T) {
	input := "\x1b[32m(I) SNR : 5.0dB\x1b[m"
	got := stripANSI(input)
	want := "(I) SNR : 5.0dB"
	if got != want {
		t.Errorf("stripANSI = %q, want %q", got, want)
	}
}

func TestSatDumpDecoderStopWhenNotRunning(t *testing.T) {
	d := NewSatDumpDecoder(57166, "METEOR-M N2-3", "satdump", t.TempDir())
	// Stop should be safe when not running.
	if err := d.Stop(); err != nil {
		t.Fatalf("Stop on non-running decoder: %v", err)
	}
}
