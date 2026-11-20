package main

import (
	"fmt"
	"time"
)

// 5x7 digits. '#' is a lit pixel, '.' is off. Corners are rounded by leaving
// the corner pixel off rather than dimming it, which looks blocky on the panel.
var digits = map[rune][7]string{
	'0': {
		".###.",
		"#...#",
		"#...#",
		"#...#",
		"#...#",
		"#...#",
		".###.",
	},
	'1': {
		"..#..",
		".##..",
		"..#..",
		"..#..",
		"..#..",
		"..#..",
		".###.",
	},
	'2': {
		".###.",
		"#...#",
		"....#",
		"...#.",
		"..#..",
		".#...",
		"#####",
	},
	'3': {
		".###.",
		"#...#",
		"....#",
		"..##.",
		"....#",
		"#...#",
		".###.",
	},
	'4': {
		"...#.",
		"..##.",
		".#.#.",
		"#..#.",
		"#####",
		"...#.",
		"...#.",
	},
	'5': {
		"#####",
		"#....",
		"####.",
		"....#",
		"....#",
		"#...#",
		".###.",
	},
	'6': {
		"..##.",
		".#...",
		"#....",
		"####.",
		"#...#",
		"#...#",
		".###.",
	},
	'7': {
		"#####",
		"....#",
		"...#.",
		"..#..",
		"..#..",
		"..#..",
		"..#..",
	},
	'8': {
		".###.",
		"#...#",
		"#...#",
		".###.",
		"#...#",
		"#...#",
		".###.",
	},
	'9': {
		".###.",
		"#...#",
		"#...#",
		".####",
		"....#",
		"...#.",
		".##..",
	},
}

const digitLevel = 255

// Top row of each digit of HH:MM (7 rows each, 1 blank row between digits
// of a pair, 2 blank rows between the hours and the minutes).
var digitRows = [4]int{1, 9, 18, 26}

// ClockFrame renders HH:MM top to bottom, the hours above the minutes, with
// a wider gap between them in place of a colon.
//
// For the first scramble duration of each minute, every digit that changed
// is scrambled: its cell starts as random noise, which thins out while the
// remaining pixels flicker between the old and new digit, settling on the
// new one.
func ClockFrame(t time.Time, scramble time.Duration) *Frame {
	var f Frame
	drawClock(&f, t, scramble, clockX)
	return &f
}

// Left column of the digits: centred on a full matrix, or flush left in
// compact mode to leave room for the seconds.
const (
	clockX        = 2
	compactClockX = 0
)

// compactSecondsCols is how many columns on the right the seconds use in
// compact mode.
const compactSecondsCols = 3

// CompactFrame renders the clock and the seconds on a single matrix: the
// digits flush left and the seconds filling the rightmost columns.
func CompactFrame(t time.Time, fill Fill, fade, roll, fillBy time.Duration) *Frame {
	var f Frame
	drawClock(&f, t, roll, compactClockX)
	drawSeconds(&f, t, fill, fade, roll, fillBy, Width-compactSecondsCols, compactSecondsCols)
	return &f
}

// drawClock draws HH:MM into f with the digits' left edge at column x0.
func drawClock(f *Frame, t time.Time, scramble time.Duration, x0 int) {
	elapsed := time.Duration(t.Second())*time.Second + time.Duration(t.Nanosecond())
	cur := t.Format("1504")
	prev := cur
	if elapsed < scramble {
		prev = t.Add(-elapsed - 1).Format("1504")
	}
	for i := range cur {
		if prev[i] == cur[i] {
			drawDigit(f, i, rune(cur[i]), x0)
			continue
		}
		p := float64(elapsed) / float64(scramble)
		// Re-randomise every scrambleFlicker, varying from minute to minute.
		seed := uint64(t.Unix()/60)<<32 | uint64(elapsed/scrambleFlicker)<<8 | uint64(i)
		for gy := 0; gy < 7; gy++ {
			for gx := 0; gx < 5; gx++ {
				px := seed<<16 | uint64(gy*5+gx)<<4
				var on bool
				if random(px|1) < 1-p {
					on = random(px|2) < 0.5
				} else if random(px|3) < p {
					on = digits[rune(cur[i])][gy][gx] == '#'
				} else {
					on = digits[rune(prev[i])][gy][gx] == '#'
				}
				if on {
					f[x0+gx][digitRows[i]+gy] = digitLevel
				}
			}
		}
	}
}

// scrambleFlicker is how long each random pattern of a scramble is shown.
const scrambleFlicker = 50 * time.Millisecond

func drawDigit(f *Frame, i int, c rune, x0 int) {
	for gy, row := range digits[c] {
		for gx, px := range row {
			if px == '#' {
				f[x0+gx][digitRows[i]+gy] = digitLevel
			}
		}
	}
}

// random hashes key (splitmix64) to a number in [0, 1). Being a pure
// function of its key keeps a frame identical across refreshes within one
// flicker period, so unchanged frames aren't resent.
func random(key uint64) float64 {
	z := key + 0x9e3779b97f4a7c15
	z = (z ^ z>>30) * 0xbf58476d1ce4e5b9
	z = (z ^ z>>27) * 0x94d049bb133111eb
	z ^= z >> 31
	return float64(z>>11) / (1 << 53)
}

// SecondsFrame lights the LEDs as the current minute progresses, in the
// pattern set by fill, the last one coming on at fillBy. Each LED fades in
// from off to full brightness over the fade duration after its turn comes
// up; near the end the fades are shortened so they all finish by fillBy,
// leaving the whole display lit at the end of the minute.
//
// For the first roll duration of each minute, the previous minute's full
// display empties instead of switching off at once: its fill plays in
// reverse, sped up to fit the roll duration, the last LED to have come on
// going off first. A soft edge several rows' worth of LEDs wide fades them
// out, so slideup rolls down from the top (slidedown up from the bottom),
// random winks out at random, creepup's (and creepdown's) columns shrink at
// their own wobbling speeds and spiral unwinds from the middle. Disco ignores fade,
// switching LEDs fully on and off, and empties by rewinding its share lit
// back to none at its rewind speed. Topo also ignores fade (its picks fade
// in fill.N at a time) and empties by rewinding, its patches shrinking away.
// Infect likewise ignores fade and rewinds, the light drawing back into its
// seeds. Binary ignores fade too, its boxes switching fully on and off,
// and empties by counting back down to zero.
func SecondsFrame(t time.Time, fill Fill, fade, roll, fillBy time.Duration) *Frame {
	var f Frame
	drawSeconds(&f, t, fill, fade, roll, fillBy, 0, Width)
	return &f
}

// drawSeconds draws the seconds fill into the cols columns of f starting at
// column x0. A shuffle fill resolves to a concrete mode per minute, so at
// the hour change the old mode's fill is rewound as the new one starts.
func drawSeconds(f *Frame, t time.Time, fill Fill, fade, roll, fillBy time.Duration, x0, cols int) {
	elapsed := time.Duration(t.Second())*time.Second + time.Duration(t.Nanosecond())
	minute := t.Unix() / 60
	pixel := func(led int) *byte { return &f[x0+led%cols][Height-1-led/cols] }

	cur := fill.At(minute)
	drawFill(pixel, cur, cols, minute, elapsed, fade, fillBy)
	if elapsed < roll {
		p := float64(elapsed) / float64(roll)
		// Ease in and out so the roll starts and lands gently.
		p = p * p * (3 - 2*p)
		drawRewind(pixel, fill.At(minute-1), cols, minute-1, p, fade, fillBy)
	}
}

// drawFill draws the given minute's fill, elapsed into it. fill must be a
// concrete mode, not shuffle.
func drawFill(pixel func(int) *byte, fill Fill, cols int, minute int64, elapsed, fade, fillBy time.Duration) {
	switch fill.Mode {
	case FillInfect:
		drawInfect(pixel, cols, minute, elapsed, fillBy, fill.N)
		return
	case FillTopo:
		drawTopo(pixel, cols, minute, elapsed, fillBy, fill.N)
		return
	case FillBinary:
		drawBinary(pixel, cols, elapsed, fillBy)
		return
	case FillDisco:
		drawDisco(pixel, cols, minute, elapsed, fillBy)
		return
	}
	for led, start := range fillStarts(fill.Mode, cols, minute, fillBy) {
		since := elapsed - start
		if since < 0 {
			continue
		}
		d := min(fade, fillBy-start)
		level := 1.0
		if since < d {
			level = float64(since) / float64(d)
		}
		*pixel(led) = brightness(level)
	}
}

// drawRewind draws the previous minute's full fill emptying, p (0 to 1,
// eased) of the way through the roll.
func drawRewind(pixel func(int) *byte, fill Fill, cols int, minute int64, p float64, fade, fillBy time.Duration) {
	back := time.Duration((1 - p) * float64(fillBy))
	switch fill.Mode {
	case FillInfect:
		// Back to its seeds.
		drawInfect(pixel, cols, minute, back, fillBy, fill.N)
		return
	case FillTopo:
		// Patches shrinking away.
		drawTopo(pixel, cols, minute, back, fillBy, fill.N)
		return
	case FillBinary:
		// Count back down to zero.
		drawBinary(pixel, cols, back, fillBy)
		return
	case FillDisco:
		// The share lit falls from all to none, the beats racing by as
		// fast as the rewind.
		drawDisco(pixel, cols, minute, back, fillBy)
		return
	}
	// Rewind runs from past fillBy back to 0, and each LED fades from full
	// to off over edge as rewind passes back through its start time. edge
	// is how long rollEdgeRows rows take to fill on average.
	edge := float64(fillBy) * float64(rollEdgeRows*cols) / float64(cols*Height-1)
	rewind := (1 - p) * (float64(fillBy) + edge)
	for led, start := range fillStarts(fill.Mode, cols, minute, fillBy) {
		level := min(max((rewind-float64(start))/edge, 0), 1)
		*pixel(led) = max(*pixel(led), brightness(level))
	}
}

// rollEdgeRows is how many rows' worth of LEDs the end-of-minute clearing's
// soft edge spans.
const rollEdgeRows = 4

// brightness converts a 0-1 level to an LED value, squared so fades look
// even to the eye.
func brightness(level float64) byte {
	return byte(level*level*255 + 0.5)
}

// 3x5 digits for the centiseconds in compact mode, where the seconds have
// only three columns.
var smallDigits = map[rune][5]string{
	'0': {"###", "#.#", "#.#", "#.#", "###"},
	'1': {".#.", "##.", ".#.", ".#.", "###"},
	'2': {"###", "..#", "###", "#..", "###"},
	'3': {"###", "..#", ".##", "..#", "###"},
	'4': {"#.#", "#.#", "###", "..#", "..#"},
	'5': {"###", "#..", "###", "..#", "###"},
	'6': {"###", "#..", "###", "#.#", "###"},
	'7': {"###", "..#", ".#.", ".#.", ".#."},
	'8': {"###", "#.#", "###", "#.#", "###"},
	'9': {"###", "#.#", "###", "..#", "###"},
}

// centis formats the seconds and centiseconds of t as "SScc".
func centis(t time.Time) string {
	return t.Format("05") + fmt.Sprintf("%02d", t.Nanosecond()/1e7)
}

// CentisFrame shows the seconds above the centiseconds (SS over cc), in
// the same font and layout as ClockFrame's HH over MM.
func CentisFrame(t time.Time) *Frame {
	var f Frame
	for i, c := range centis(t) {
		drawDigit(&f, i, c, clockX)
	}
	return &f
}

// CompactCentisFrame renders the clock and, in place of the seconds fill,
// the seconds above the centiseconds in 3x5 digits, each centred in the
// rows of the clock digit beside it.
func CompactCentisFrame(t time.Time, roll time.Duration) *Frame {
	var f Frame
	drawClock(&f, t, roll, compactClockX)
	x0 := Width - compactSecondsCols
	for i, c := range centis(t) {
		for gy, row := range smallDigits[c] {
			for gx, px := range row {
				if px == '#' {
					f[x0+gx][digitRows[i]+1+gy] = digitLevel
				}
			}
		}
	}
	return &f
}
