package main

import (
	"log"
	"sync"
	"time"
)

// Same geometry as the real module (and led-clock's ledmatrix.go).
const (
	Width  = 9
	Height = 34
)

// Commands understood, from inputmodule-rs commands.md.
const (
	cmdBrightness = 0x00
	cmdDrawBW     = 0x06
	cmdStageCol   = 0x07
	cmdFlushCols  = 0x08
)

// paramLen is the number of parameter bytes each known command takes.
var paramLen = map[byte]int{
	cmdBrightness: 1,
	cmdDrawBW:     (Width*Height + 7) / 8,
	cmdStageCol:   1 + Height,
	cmdFlushCols:  0,
}

// Frame is a greyscale image (0 = off, 255 = full), indexed [x][y].
type Frame [Width][Height]byte

// Panel is the state of one emulated matrix. Bytes written to its serial
// port are fed to Write, which decodes them as the module's firmware would.
type Panel struct {
	Name string
	Path string // the pty led-clock should open

	hub *Hub

	mu         sync.Mutex
	shown      Frame
	staged     Frame
	brightness byte
	lastCmd    time.Time // when bytes last arrived
	frames     uint64    // frames shown so far

	// Decoder state: the bytes of the command being read so far, magic
	// included.
	buf []byte
	// unknown is the set of unknown command IDs already logged.
	unknown map[byte]bool
}

func NewPanel(name, path string, brightness byte, hub *Hub) *Panel {
	return &Panel{Name: name, Path: path, brightness: brightness, hub: hub, unknown: map[byte]bool{}}
}

// Write decodes data written to the panel's serial port. Commands may be
// split across writes. Bytes that aren't part of a command are skipped
// until the next magic number.
func (p *Panel) Write(data []byte) {
	p.mu.Lock()
	p.lastCmd = time.Now()
	changed := false
	for _, b := range data {
		switch len(p.buf) {
		case 0:
			if b == 0x32 {
				p.buf = append(p.buf, b)
			}
			continue
		case 1:
			switch b {
			case 0xAC:
				p.buf = append(p.buf, b)
			case 0x32:
				// Still possibly the start of the magic.
			default:
				p.buf = p.buf[:0]
			}
			continue
		case 2:
			if _, ok := paramLen[b]; !ok {
				if !p.unknown[b] {
					p.unknown[b] = true
					log.Printf("%s: ignoring unsupported command 0x%02x", p.Name, b)
				}
				p.buf = p.buf[:0]
				continue
			}
		}
		p.buf = append(p.buf, b)
		if len(p.buf) == 3+paramLen[p.buf[2]] {
			if p.exec(p.buf[2], p.buf[3:]) {
				changed = true
			}
			p.buf = p.buf[:0]
		}
	}
	p.mu.Unlock()
	if changed {
		p.hub.Notify()
	}
}

// exec runs one complete command, reporting whether what's shown changed.
func (p *Panel) exec(cmd byte, args []byte) bool {
	switch cmd {
	case cmdBrightness:
		if p.brightness == args[0] {
			return false
		}
		p.brightness = args[0]
		return true
	case cmdStageCol:
		x := int(args[0])
		if x >= Width {
			log.Printf("%s: StageCol for column %d, which doesn't exist", p.Name, x)
			return false
		}
		copy(p.staged[x][:], args[1:])
		return false
	case cmdFlushCols:
		p.shown = p.staged
		p.frames++
		return true
	case cmdDrawBW:
		// One bit per LED, row by row from the top left, LSB first.
		for i := range Width * Height {
			var v byte
			if args[i/8]&(1<<(i%8)) != 0 {
				v = 0xFF
			}
			p.shown[i%Width][i/Width] = v
		}
		p.frames++
		return true
	}
	return false
}

// Reset drops any partly received command, as after the sender has gone.
func (p *Panel) Reset() {
	p.mu.Lock()
	p.buf = p.buf[:0]
	p.mu.Unlock()
}

// PanelState is a snapshot of a panel, as sent to the browser.
type PanelState struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Brightness byte   `json:"brightness"`
	// SinceCommand is the milliseconds since bytes last arrived, or -1 if
	// none have.
	SinceCommand int64  `json:"sinceCommand"`
	Frames       uint64 `json:"frames"`
	// Pixels is column by column, each top to bottom, as the protocol
	// sends them (base64 in JSON).
	Pixels []byte `json:"pixels"`
}

func (p *Panel) State() PanelState {
	p.mu.Lock()
	defer p.mu.Unlock()
	px := make([]byte, 0, Width*Height)
	for x := range Width {
		px = append(px, p.shown[x][:]...)
	}
	since := int64(-1)
	if !p.lastCmd.IsZero() {
		since = time.Since(p.lastCmd).Milliseconds()
	}
	return PanelState{
		Name:         p.Name,
		Path:         p.Path,
		Brightness:   p.brightness,
		SinceCommand: since,
		Frames:       p.frames,
		Pixels:       px,
	}
}

// Hub tells watchers when any panel changes.
type Hub struct {
	mu      sync.Mutex
	changed chan struct{}
}

func NewHub() *Hub { return &Hub{changed: make(chan struct{})} }

// Changed returns a channel that's closed at the next change.
func (h *Hub) Changed() <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.changed
}

func (h *Hub) Notify() {
	h.mu.Lock()
	close(h.changed)
	h.changed = make(chan struct{})
	h.mu.Unlock()
}
