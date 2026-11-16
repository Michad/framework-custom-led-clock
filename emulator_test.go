package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestEmulator starts the real emulator, draws a frame on each of its
// ports and stops it.
func TestEmulator(t *testing.T) {
	if testing.Short() {
		t.Skip("builds led-emulator")
	}
	bin := filepath.Join(t.TempDir(), emulatorName)
	if out, err := exec.Command("go", "build", "-o", bin, "./cmd/led-emulator").CombinedOutput(); err != nil {
		t.Fatalf("building emulator: %v\n%s", err, out)
	}

	emu, ports, err := StartEmulator(bin, []string{"left", "right"}, "-listen", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stopped := false
	defer func() {
		if !stopped {
			emu.Stop()
		}
	}()
	if len(ports) != 2 || ports["left"] == "" || ports["right"] == "" {
		t.Fatalf("ports = %v, want left and right", ports)
	}
	var f Frame
	f[4][17] = 255
	for name, path := range ports {
		m, err := OpenMatrix(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := m.Draw(&f); err != nil {
			t.Errorf("%s: draw: %v", name, err)
		}
		m.Close()
	}

	stopped = true
	if err := emu.Stop(); err != nil {
		t.Errorf("stop: %v", err)
	}
	if emu.cmd.ProcessState == nil {
		t.Error("emulator still running after Stop")
	}
	if _, err := os.Stat(emu.dir); !os.IsNotExist(err) {
		t.Errorf("ports directory %s left behind (stat: %v)", emu.dir, err)
	}
}

func TestEmulatorMissing(t *testing.T) {
	_, _, err := StartEmulator(filepath.Join(t.TempDir(), "nonexistent"), []string{"left"})
	if err == nil {
		t.Fatal("StartEmulator with a missing binary succeeded")
	}
}
