package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const emulatorName = "led-emulator"

// emulatorTimeout is how long to wait for the emulator to create its ports,
// and then for it to exit when stopped.
const emulatorTimeout = 5 * time.Second

// Emulator is a led-emulator process started by -emulator, standing in for
// the matrices.
type Emulator struct {
	cmd  *exec.Cmd
	dir  string
	done chan struct{} // closed when the process has exited
	err  error         // its exit status, once done is closed
}

// FindEmulator returns the path of the led-emulator binary: the one next to
// this program if there is one, else the one on $PATH.
func FindEmulator() (string, error) {
	if self, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(self), emulatorName)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	p, err := exec.LookPath(emulatorName)
	if err != nil {
		return "", fmt.Errorf("%s not found next to this program or on $PATH; build it with `make emulator`", emulatorName)
	}
	return p, nil
}

// StartEmulator runs the emulator at bin, emulating a matrix for each of
// names, and waits for its ports to appear. It returns each matrix's port
// path by name. extra is passed on to the emulator, after the arguments
// set here. The emulator's output goes to this program's.
func StartEmulator(bin string, names []string, extra ...string) (*Emulator, map[string]string, error) {
	dir, err := os.MkdirTemp("", "led-emulator-")
	if err != nil {
		return nil, nil, err
	}
	args := append([]string{"-panels", strings.Join(names, ","), "-dir", dir, "-hint=false"}, extra...)
	cmd := exec.Command(bin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// Its own process group, so a Ctrl-C in the terminal reaches only
		// this program, which then blanks the displays before stopping
		// the emulator.
		Setpgid: true,
		// Stop it if this program dies without stopping it (log.Fatal
		// skips deferred calls).
		Pdeathsig: syscall.SIGTERM,
	}
	if err := cmd.Start(); err != nil {
		os.Remove(dir)
		return nil, nil, fmt.Errorf("starting %s: %w", bin, err)
	}
	e := &Emulator{cmd: cmd, dir: dir, done: make(chan struct{})}
	go func() {
		e.err = cmd.Wait()
		close(e.done)
	}()

	// It creates a symlink named after each matrix once its port is ready.
	ports := map[string]string{}
	deadline := time.After(emulatorTimeout)
	for len(ports) < len(names) {
		for _, n := range names {
			link := filepath.Join(dir, n)
			if _, err := os.Readlink(link); err == nil {
				ports[n] = link
			}
		}
		if len(ports) == len(names) {
			break
		}
		select {
		case <-e.done:
			os.RemoveAll(dir)
			return nil, nil, fmt.Errorf("%s exited before its ports were ready: %v", bin, e.err)
		case <-deadline:
			e.Stop()
			return nil, nil, fmt.Errorf("%s's ports weren't ready after %s", bin, emulatorTimeout)
		case <-time.After(20 * time.Millisecond):
		}
	}
	return e, ports, nil
}

// Stop shuts the emulator down, killing it if it doesn't exit in time, and
// removes its ports' directory.
func (e *Emulator) Stop() error {
	defer os.RemoveAll(e.dir)
	select {
	case <-e.done:
		return e.err
	default:
	}
	if err := e.cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	select {
	case <-e.done:
		return nil
	case <-time.After(emulatorTimeout):
		e.cmd.Process.Kill()
		<-e.done
		return fmt.Errorf("%s didn't exit within %s; killed it", emulatorName, emulatorTimeout)
	}
}
