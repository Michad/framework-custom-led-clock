package main

import "time"

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
// column x0.
func drawSeconds(f *Frame, t time.Time, fill Fill, fade, roll, fillBy time.Duration, x0, cols int) {
	mode := fill.Mode
	elapsed := time.Duration(t.Second())*time.Second + time.Duration(t.Nanosecond())
	minute := t.Unix() / 60
	pixel := func(led int) *byte { return &f[x0+led%cols][Height-1-led/cols] }

	if mode == FillInfect {
		drawInfect(pixel, cols, minute, elapsed, fillBy, fill.N)
		if elapsed < roll {
			// Rewind the previous minute's infection, back to its seeds.
			p := float64(elapsed) / float64(roll)
			p = p * p * (3 - 2*p)
			drawInfect(pixel, cols, minute-1, time.Duration((1-p)*float64(fillBy)), fillBy, fill.N)
		}
		return
	}

	if mode == FillTopo {
		drawTopo(pixel, cols, minute, elapsed, fillBy, fill.N)
		if elapsed < roll {
			// Rewind the previous minute's topo, patches shrinking away.
			p := float64(elapsed) / float64(roll)
			p = p * p * (3 - 2*p)
			drawTopo(pixel, cols, minute-1, time.Duration((1-p)*float64(fillBy)), fillBy, fill.N)
		}
		return
	}

	if mode == FillBinary {
		drawBinary(pixel, cols, elapsed, fillBy)
		if elapsed < roll {
			// Count the previous minute back down to zero.
			p := float64(elapsed) / float64(roll)
			p = p * p * (3 - 2*p)
			drawBinary(pixel, cols, time.Duration((1-p)*float64(fillBy)), fillBy)
		}
		return
	}

	if mode == FillDisco {
		drawDisco(pixel, cols, minute, elapsed, fillBy)
		if elapsed < roll {
			// Rewind the previous minute's disco: the share lit falls from
			// all to none, the beats racing by as fast as the rewind.
			p := float64(elapsed) / float64(roll)
			p = p * p * (3 - 2*p)
			drawDisco(pixel, cols, minute-1, time.Duration((1-p)*float64(fillBy)), fillBy)
		}
		return
	}

	for led, start := range fillStarts(mode, cols, minute, fillBy) {
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

	if elapsed < roll {
		p := float64(elapsed) / float64(roll)
		// Ease in and out so the roll starts and lands gently.
		p = p * p * (3 - 2*p)
		// The previous minute's fill, rewound: rewind runs from past fillBy
		// back to 0, and each LED fades from full to off over edge as
		// rewind passes back through its start time. edge is how long
		// rollEdgeRows rows take to fill on average.
		edge := float64(fillBy) * float64(rollEdgeRows*cols) / float64(cols*Height-1)
		rewind := (1 - p) * (float64(fillBy) + edge)
		for led, start := range fillStarts(mode, cols, minute-1, fillBy) {
			level := min(max((rewind-float64(start))/edge, 0), 1)
			*pixel(led) = max(*pixel(led), brightness(level))
		}
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
