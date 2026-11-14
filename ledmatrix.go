package main

import (
	"fmt"
	"sort"
	"strings"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

// Protocol details from inputmodule-rs (commands.md / inputmodule-control).
const (
	Width  = 9
	Height = 34

	frameworkVID = "32AC"
	ledMatrixPID = "0020"

	cmdBrightness = 0x00
	cmdStageCol   = 0x07
	cmdFlushCols  = 0x08
)

var magic = []byte{0x32, 0xAC}

// Frame is a greyscale image (0 = off, 255 = full), indexed [x][y].
type Frame [Width][Height]byte

// Matrix is an open connection to one LED matrix module.
type Matrix struct {
	path string
	port serial.Port
	last *Frame
}

func OpenMatrix(path string) (*Matrix, error) {
	m := &Matrix{path: path}
	if err := m.open(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Matrix) open() error {
	p, err := serial.Open(m.path, &serial.Mode{BaudRate: 115200})
	if err != nil {
		return fmt.Errorf("open %s: %w", m.path, err)
	}
	m.port = p
	m.last = nil
	return nil
}

func (m *Matrix) Close() error {
	if m.port == nil {
		return nil
	}
	err := m.port.Close()
	m.port = nil
	return err
}

func (m *Matrix) command(id byte, args ...byte) error {
	if m.port == nil {
		if err := m.open(); err != nil {
			return err
		}
	}
	buf := append(append(append([]byte{}, magic...), id), args...)
	if _, err := m.port.Write(buf); err != nil {
		// Drop the port so the next call reopens it (e.g. after USB reset).
		m.Close()
		return fmt.Errorf("write %s: %w", m.path, err)
	}
	return nil
}

// SetBrightness sets the global LED brightness (0-255).
func (m *Matrix) SetBrightness(b byte) error {
	return m.command(cmdBrightness, b)
}

// Draw stages each column with StageCol, then shows them all at once with
// FlushCols so the matrix is never partially updated. Unchanged frames are
// skipped.
func (m *Matrix) Draw(f *Frame) error {
	if m.last != nil && *m.last == *f {
		return nil
	}
	for x := 0; x < Width; x++ {
		args := append([]byte{byte(x)}, f[x][:]...)
		if err := m.command(cmdStageCol, args...); err != nil {
			return err
		}
	}
	if err := m.command(cmdFlushCols); err != nil {
		return err
	}
	cp := *f
	m.last = &cp
	return nil
}

// FindMatrices returns the serial device paths of all attached LED matrix
// modules, sorted by path.
func FindMatrices() ([]string, error) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range ports {
		if p.IsUSB && strings.EqualFold(p.VID, frameworkVID) && strings.EqualFold(p.PID, ledMatrixPID) {
			paths = append(paths, p.Name)
		}
	}
	sort.Strings(paths)
	return paths, nil
}
