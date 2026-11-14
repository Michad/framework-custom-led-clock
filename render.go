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
	elapsed := time.Duration(t.Second())*time.Second + time.Duration(t.Nanosecond())
	cur := t.Format("1504")
	prev := cur
	if elapsed < scramble {
		prev = t.Add(-elapsed - 1).Format("1504")
	}
	for i := range cur {
		if prev[i] == cur[i] {
			drawDigit(&f, i, rune(cur[i]))
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
					f[2+gx][digitRows[i]+gy] = digitLevel
				}
			}
		}
	}
	return &f
}

// scrambleFlicker is how long each random pattern of a scramble is shown.
const scrambleFlicker = 50 * time.Millisecond

func drawDigit(f *Frame, i int, c rune) {
	for gy, row := range digits[c] {
		for gx, px := range row {
			if px == '#' {
				f[2+gx][digitRows[i]+gy] = digitLevel
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

// SecondsFrame lights LEDs from the bottom, row by row, as the current
// minute progresses, the last one coming on at fillBy. Each LED fades in from
// off to full brightness over the fade duration after its turn comes up; near
// the top the fades are shortened so they all finish by fillBy, leaving the
// whole display lit at the end of the minute.
//
// For the first roll duration of each minute, the previous minute's full
// display rolls down to empty, a soft edge sweeping whole rows out from the
// top, instead of switching off at once.
func SecondsFrame(t time.Time, fade, roll, fillBy time.Duration) *Frame {
	var f Frame
	elapsed := time.Duration(t.Second())*time.Second + time.Duration(t.Nanosecond())
	// Time between successive LEDs starting to fade in.
	step := fillBy / (Width*Height - 1)
	for i := 0; i < Width*Height; i++ {
		start := time.Duration(i) * step
		since := elapsed - start
		if since <= 0 {
			break
		}
		d := min(fade, fillBy-start)
		level := 1.0
		if since < d {
			level = float64(since) / float64(d)
		}
		f[i%Width][Height-1-i/Width] = brightness(level)
	}

	if elapsed < roll {
		p := float64(elapsed) / float64(roll)
		// Ease in and out so the roll starts and lands gently.
		p = p * p * (3 - 2*p)
		// pos is the height (in rows from the bottom) of the soft edge's top.
		pos := (1 - p) * (Height + rollEdgeRows)
		for r := 0; r < Height; r++ {
			level := min(max((pos-float64(r))/rollEdgeRows, 0), 1)
			y := Height - 1 - r
			for x := 0; x < Width; x++ {
				f[x][y] = max(f[x][y], brightness(level))
			}
		}
	}
	return &f
}

// rollEdgeRows is how many rows the roll-down's soft edge spans.
const rollEdgeRows = 4

// brightness converts a 0-1 level to an LED value, squared so fades look
// even to the eye.
func brightness(level float64) byte {
	return byte(level*level*255 + 0.5)
}
