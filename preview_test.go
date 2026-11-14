package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"
)

func TestPreview(t *testing.T) {
	out := os.Getenv("PREVIEW_PNG")
	if out == "" {
		t.Skip()
	}
	times := [][2]int{{1, 23}, {4, 56}, {17, 8}, {20, 39}}
	const cell = 16
	img := image.NewRGBA(image.Rect(0, 0, len(times)*(Width+2)*cell, Height*cell))
	for i := range img.Pix {
		img.Pix[i] = 0
		if i%4 == 3 {
			img.Pix[i] = 255
		}
	}
	for n, ts := range times {
		f := ClockFrame(time.Date(2026, 1, 1, ts[0], ts[1], 0, 0, time.Local), 0)
		for x := 0; x < Width; x++ {
			for y := 0; y < Height; y++ {
				v := f[x][y]
				for dx := 2; dx < cell-2; dx++ {
					for dy := 2; dy < cell-2; dy++ {
						img.Set((n*(Width+2)+x)*cell+dx, y*cell+dy, color.RGBA{max(v, 25), max(v, 25), max(v/2, 25), 255})
					}
				}
			}
		}
	}
	fh, _ := os.Create(out)
	defer fh.Close()
	png.Encode(fh, img)
}
