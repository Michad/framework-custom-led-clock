package main

import "testing"

func stageCol(x byte, col [Height]byte) []byte {
	return append([]byte{0x32, 0xAC, cmdStageCol, x}, col[:]...)
}

func TestPanelDecodes(t *testing.T) {
	p := NewPanel("test", "", 255, NewHub())
	var col [Height]byte
	for y := range col {
		col[y] = byte(y * 7)
	}
	var stream []byte
	stream = append(stream, 0x00, 0x32, 0x32) // junk, and a false start
	stream = append(stream, 0x32, 0xAC, cmdBrightness, 40)
	stream = append(stream, 0x32, 0xAC, 0x7F) // unsupported, skipped
	stream = append(stream, stageCol(3, col)...)
	stream = append(stream, stageCol(Width, col)...) // no such column

	// Fed one byte at a time, as commands can be split across reads.
	for _, b := range stream {
		p.Write([]byte{b})
	}
	if s := p.State(); s.Brightness != 40 || s.Frames != 0 || s.Pixels[3*Height+33] != 0 {
		t.Fatalf("before flush: brightness %d, frames %d, pixel %d; want 40, 0, 0", s.Brightness, s.Frames, s.Pixels[3*Height+33])
	}

	p.Write([]byte{0x32, 0xAC, cmdFlushCols})
	s := p.State()
	if s.Frames != 1 {
		t.Fatalf("frames = %d, want 1", s.Frames)
	}
	for y := range Height {
		if got := s.Pixels[3*Height+y]; got != col[y] {
			t.Errorf("pixel (3,%d) = %d, want %d", y, got, col[y])
		}
		if got := s.Pixels[2*Height+y]; got != 0 {
			t.Errorf("pixel (2,%d) = %d, want 0", y, got)
		}
	}
}

func TestPanelDrawBW(t *testing.T) {
	p := NewPanel("test", "", 255, NewHub())
	args := make([]byte, paramLen[cmdDrawBW])
	args[0] = 0b10            // LED 1: (1,0)
	args[1] = 0b1             // LED 8: (8,0)
	args[38] = 1 << (305 % 8) // LED 305: (8,33)
	p.Write(append([]byte{0x32, 0xAC, cmdDrawBW}, args...))
	s := p.State()
	lit := 0
	for _, v := range s.Pixels {
		if v != 0 {
			lit++
		}
	}
	at := func(x, y int) byte { return s.Pixels[x*Height+y] }
	if lit != 3 || at(1, 0) != 0xFF || at(8, 0) != 0xFF || at(8, 33) != 0xFF {
		t.Errorf("DrawBW lit %d LEDs, (1,0)=%d (8,0)=%d (8,33)=%d; want 3 at full", lit, at(1, 0), at(8, 0), at(8, 33))
	}
}
