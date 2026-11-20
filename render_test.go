package main

import (
	"fmt"
	"math"
	"testing"
	"time"
)

func TestCompactFrameLayout(t *testing.T) {
	const fillBy = time.Minute - 50*time.Millisecond
	// End of the minute: seconds columns fully lit, no animation running.
	now := time.Date(2026, 1, 1, 18, 8, 59, 990_000_000, time.Local)
	f := CompactFrame(now, Fill{Mode: FillSlideUp}, time.Second, time.Second, fillBy)
	clock := ClockFrame(now, time.Second)

	for y := 0; y < Height; y++ {
		// Digits are the full-width layout shifted flush left.
		for x := 0; x < 5; x++ {
			if f[x][y] != clock[x+clockX][y] {
				t.Errorf("digit pixel (%d,%d) = %d, want %d", x, y, f[x][y], clock[x+clockX][y])
			}
		}
		// Gap between the digits and the seconds.
		for x := 5; x < Width-compactSecondsCols; x++ {
			if f[x][y] != 0 {
				t.Errorf("gap pixel (%d,%d) = %d, want 0", x, y, f[x][y])
			}
		}
		for x := Width - compactSecondsCols; x < Width; x++ {
			if f[x][y] != 255 {
				t.Errorf("seconds pixel (%d,%d) = %d, want 255", x, y, f[x][y])
			}
		}
	}
}

func TestCompactSecondsFillFromBottom(t *testing.T) {
	const fillBy = time.Minute - 50*time.Millisecond
	now := time.Date(2026, 1, 1, 18, 8, 30, 0, time.Local)
	f := CompactFrame(now, Fill{Mode: FillSlideUp}, 0, 0, fillBy)
	lit := 0
	for x := Width - compactSecondsCols; x < Width; x++ {
		for y := 0; y < Height; y++ {
			if f[x][y] > 0 {
				lit++
			}
		}
	}
	// Half way through the minute, about half of the 102 LEDs are lit.
	if lit < 49 || lit > 53 {
		t.Errorf("%d seconds LEDs lit at :30, want about 51", lit)
	}
	if f[Width-1][Height-1] == 0 || f[Width-1][0] != 0 {
		t.Error("seconds should fill from the bottom")
	}
}

func litSeconds(f *Frame) int {
	n := 0
	for x := 0; x < Width; x++ {
		for y := 0; y < Height; y++ {
			if f[x][y] > 0 {
				n++
			}
		}
	}
	return n
}

func TestRandomFill(t *testing.T) {
	const fillBy = time.Minute - 50*time.Millisecond
	at := func(m, s, ns int) *Frame {
		return SecondsFrame(time.Date(2026, 1, 1, 18, m, s, ns, time.Local), Fill{Mode: FillRandom}, time.Second, 0, fillBy)
	}

	// Same number of LEDs lit as slideup, but not just the bottom rows.
	half := at(8, 30, 0)
	slide := SecondsFrame(time.Date(2026, 1, 1, 18, 8, 30, 0, time.Local), Fill{Mode: FillSlideUp}, time.Second, 0, fillBy)
	if got, want := litSeconds(half), litSeconds(slide); got != want {
		t.Errorf("random lit %d LEDs at :30, slideup %d", got, want)
	}
	topHalf := 0
	for x := 0; x < Width; x++ {
		for y := 0; y < Height/2; y++ {
			if half[x][y] > 0 {
				topHalf++
			}
		}
	}
	if topHalf == 0 {
		t.Error("random fill at :30 lit nothing in the top half")
	}

	// Stable within a minute, different between minutes.
	if *at(8, 30, 0) != *half {
		t.Error("random fill differs between refreshes at the same time")
	}
	if *at(9, 30, 0) == *half {
		t.Error("random fill has the same order in consecutive minutes")
	}

	// An LED stays lit once on.
	later := at(8, 45, 0)
	for x := 0; x < Width; x++ {
		for y := 0; y < Height; y++ {
			if half[x][y] > 0 && later[x][y] < half[x][y] {
				t.Fatalf("LED (%d,%d) dimmed from %d to %d", x, y, half[x][y], later[x][y])
			}
		}
	}

	// Everything lit by the end of the minute.
	if got := litSeconds(at(8, 59, 960_000_000)); got != Width*Height {
		t.Errorf("%d LEDs lit at the end of the minute, want %d", got, Width*Height)
	}
	end := at(8, 59, 960_000_000)
	for x := 0; x < Width; x++ {
		for y := 0; y < Height; y++ {
			if end[x][y] != 255 {
				t.Fatalf("LED (%d,%d) = %d at the end of the minute, want 255", x, y, end[x][y])
			}
		}
	}
}

// The clearing at the start of a minute runs the previous minute's fill
// backwards: LEDs that came on last go off first.
func TestClearReversesFill(t *testing.T) {
	const fillBy = time.Minute - 50*time.Millisecond
	const roll = time.Second
	for _, mode := range []FillMode{FillSlideUp, FillSlideDown, FillRandom, FillCreepUp, FillCreepDown, FillSpiral} {
		prev := time.Date(2026, 1, 1, 18, 8, 0, 0, time.Local)
		starts := fillStarts(mode, Width, prev.Unix()/60, fillBy)
		first, last := 0, 0
		for led, s := range starts {
			if s < starts[first] {
				first = led
			}
			if s > starts[last] {
				last = led
			}
		}
		px := func(f *Frame, led int) byte { return f[led%Width][Height-1-led/Width] }

		// Start of the next minute: still fully lit.
		start := SecondsFrame(prev.Add(time.Minute), Fill{Mode: mode}, time.Second, roll, fillBy)
		if litSeconds(start) != Width*Height || px(start, first) != 255 || px(start, last) != 255 {
			t.Errorf("%s: display not full at the start of the clearing", mode)
		}
		// Half way: the last LED to come on is off, the first still on.
		mid := SecondsFrame(prev.Add(time.Minute+roll/2), Fill{Mode: mode}, time.Second, roll, fillBy)
		if px(mid, last) != 0 || px(mid, first) != 255 {
			t.Errorf("%s: half way through the clearing, last=%d first=%d; want 0, 255", mode, px(mid, last), px(mid, first))
		}
		// After the clearing only the new minute's fill is showing.
		after := prev.Add(time.Minute + roll)
		if *SecondsFrame(after, Fill{Mode: mode}, time.Second, roll, fillBy) != *SecondsFrame(after, Fill{Mode: mode}, time.Second, 0, fillBy) {
			t.Errorf("%s: clearing still showing after roll", mode)
		}
	}
}

func TestFillStartsRange(t *testing.T) {
	const fillBy = time.Minute - 50*time.Millisecond
	for _, mode := range []FillMode{FillSlideUp, FillSlideDown, FillRandom, FillCreepUp, FillCreepDown, FillSpiral} {
		for _, cols := range []int{Width, compactSecondsCols} {
			for minute := int64(29_000_000); minute < 29_000_050; minute++ {
				starts := fillStarts(mode, cols, minute, fillBy)
				lo, hi := starts[0], starts[0]
				for _, s := range starts {
					lo, hi = min(lo, s), max(hi, s)
				}
				// Slideup and random step in whole nanoseconds, so may end
				// a few short of fillBy.
				if lo != 0 || hi > fillBy || hi < fillBy-time.Microsecond {
					t.Fatalf("%s cols=%d minute=%d: starts span %s-%s, want 0-%s", mode, cols, minute, lo, hi, fillBy)
				}
			}
		}
	}
}

// creepHeights returns how many LEDs are lit in each column of f.
func creepHeights(f *Frame) [Width]int {
	var h [Width]int
	for x := range h {
		for y := 0; y < Height; y++ {
			if f[x][y] > 0 {
				h[x]++
			}
		}
	}
	return h
}

func TestCreepUp(t *testing.T) {
	const fillBy = time.Minute - 50*time.Millisecond
	base := time.Date(2026, 1, 1, 18, 8, 0, 0, time.Local)
	at := func(d time.Duration) *Frame { return SecondsFrame(base.Add(d), Fill{Mode: FillCreepUp}, 0, 0, fillBy) }

	var prev [Width]int
	spread := 0
	for d := time.Duration(0); d < time.Minute; d += 250 * time.Millisecond {
		f := at(d)
		h := creepHeights(f)
		lo, hi := Height, 0
		for x := 0; x < Width; x++ {
			// Each column is solid from the bottom up.
			for y := Height - h[x]; y < Height; y++ {
				if f[x][y] == 0 {
					t.Fatalf("at %s column %d has a gap at row %d", d, x, y)
				}
			}
			if h[x] < prev[x] {
				t.Fatalf("at %s column %d shrank from %d to %d", d, x, prev[x], h[x])
			}
			lo, hi = min(lo, h[x]), max(hi, h[x])
		}
		spread = max(spread, hi-lo)
		prev = h
	}
	// The columns wander apart rather than moving in step...
	if spread < 4 {
		t.Errorf("columns were never more than %d LEDs apart, want them to wander", spread)
	}
	// ...and all reach the top by the end of the minute.
	for x, h := range creepHeights(at(fillBy)) {
		if h != Height {
			t.Errorf("column %d at %d LEDs at fillBy, want %d", x, h, Height)
		}
	}

	// Each column has fast and slow spells: its quickest rows rise several
	// times faster than its slowest.
	starts := fillStarts(FillCreepUp, Width, base.Unix()/60, fillBy)
	for c := 0; c < Width; c++ {
		fast, slow := time.Hour, time.Duration(0)
		for r := 1; r < Height; r++ {
			d := starts[r*Width+c] - starts[(r-1)*Width+c]
			fast, slow = min(fast, d), max(slow, d)
		}
		if slow < 3*fast {
			t.Errorf("column %d rows take %s-%s, want a wider spread of speeds", c, fast, slow)
		}
	}
}

// Slidedown and creepdown are slideup and creepup upside down.
func TestDownModesMirrorUp(t *testing.T) {
	const fillBy = time.Minute - 50*time.Millisecond
	base := time.Date(2026, 1, 1, 18, 8, 0, 0, time.Local)
	for _, pair := range [][2]FillMode{{FillSlideUp, FillSlideDown}, {FillCreepUp, FillCreepDown}} {
		for _, cols := range []int{Width, compactSecondsCols} {
			up := fillStarts(pair[0], cols, base.Unix()/60, fillBy)
			down := fillStarts(pair[1], cols, base.Unix()/60, fillBy)
			for led := range up {
				row, c := led/cols, led%cols
				flipped := (Height-1-row)*cols + c
				// Slide modes go left to right on every row either way.
				if pair[0] == FillSlideUp {
					if got, want := down[flipped]-down[(Height-1-row)*cols], up[led]-up[row*cols]; got != want {
						t.Fatalf("%s cols=%d: LED %d starts %s into its row, want %s", pair[1], cols, flipped, got, want)
					}
					continue
				}
				if down[flipped] != up[led] {
					t.Fatalf("%s cols=%d: LED %d starts at %s, want %s", pair[1], cols, flipped, down[flipped], up[led])
				}
			}
		}
	}
	// Slidedown starts in the top left of the frame and ends bottom right.
	f := SecondsFrame(base.Add(time.Second), Fill{Mode: FillSlideDown}, 0, 0, fillBy)
	if f[0][0] == 0 || f[Width-1][Height-1] != 0 {
		t.Error("slidedown should fill from the top")
	}
}

func TestSpiralOrder(t *testing.T) {
	for _, cols := range []int{Width, compactSecondsCols, 1, 2} {
		order := spiralOrder(cols)
		if len(order) != cols*Height {
			t.Fatalf("cols=%d: %d LEDs in spiral, want %d", cols, len(order), cols*Height)
		}
		seen := make([]bool, cols*Height)
		for i, led := range order {
			if seen[led] {
				t.Fatalf("cols=%d: LED %d appears twice", cols, led)
			}
			seen[led] = true
			// Each step moves to a neighbouring LED, except when moving in
			// to the next ring, which is diagonal.
			if i > 0 {
				dx, dy := led%cols-order[i-1]%cols, led/cols-order[i-1]/cols
				if dx*dx+dy*dy > 2 {
					t.Fatalf("cols=%d: spiral jumps from LED %d to %d", cols, order[i-1], led)
				}
			}
		}
		// Starts at the top left, heading right.
		if order[0] != (Height-1)*cols {
			t.Errorf("cols=%d: spiral starts at LED %d, want the top left", cols, order[0])
		}
	}
}

func TestDisco(t *testing.T) {
	const fillBy = time.Minute - 50*time.Millisecond
	const roll = time.Second
	const n = Width * Height
	base := time.Date(2026, 1, 1, 18, 8, 0, 0, time.Local)
	at := func(d time.Duration) *Frame {
		return SecondsFrame(base.Add(d), Fill{Mode: FillDisco}, time.Second, roll, fillBy)
	}

	// The share lit tracks the share of the minute gone, within a beat's
	// worth of LEDs (about one).
	for d := roll; d < fillBy; d += 37 * time.Millisecond {
		share := float64(n) * float64(d) / float64(fillBy)
		beatLEDs := float64(n) * float64(discoBeat) / float64(fillBy)
		if got := float64(litSeconds(at(d))); got < share-beatLEDs-0.5 || got > share+beatLEDs+0.5 {
			t.Fatalf("at %s %v LEDs lit, want about %.1f", d, got, share)
		}
	}
	if got := litSeconds(at(fillBy)); got != n {
		t.Errorf("%d LEDs lit at fillBy, want all %d", got, n)
	}

	// The pattern only changes on a beat, so a lone LED never turns on
	// in a standing pattern (which looks like a stall), in either layout.
	beats := int64((fillBy + discoBeat/2) / discoBeat)
	beatOf := func(d time.Duration) int64 { return int64(d) * beats / int64(fillBy) }
	const tick = 10 * time.Millisecond
	for _, compact := range []bool{false, true} {
		frame := func(d time.Duration) *Frame {
			if compact {
				return CompactFrame(base.Add(d), Fill{Mode: FillDisco}, time.Second, roll, fillBy)
			}
			return at(d)
		}
		for d := roll + tick; d < fillBy; d += tick {
			if *frame(d) != *frame(d - tick) && beatOf(d) == beatOf(d-tick) {
				t.Fatalf("compact=%v: frame changed mid-beat at %s", compact, d)
			}
		}
	}

	// LEDs are fully on or off.
	f := at(20 * time.Second)
	for x := 0; x < Width; x++ {
		for y := 0; y < Height; y++ {
			if v := f[x][y]; v != 0 && v != 255 {
				t.Fatalf("LED (%d,%d) = %d, want 0 or 255", x, y, v)
			}
		}
	}

	// Stable within a beat, a new pattern each beat: across a beat boundary
	// many LEDs that were on go off.
	a, b := at(30*time.Second+50*time.Millisecond), at(30*time.Second+250*time.Millisecond)
	if *a != *at(30*time.Second + 50*time.Millisecond) {
		t.Error("disco frame differs between refreshes at the same time")
	}
	wentOff := 0
	for x := 0; x < Width; x++ {
		for y := 0; y < Height; y++ {
			if a[x][y] > 0 && b[x][y] == 0 {
				wentOff++
			}
		}
	}
	if wentOff < n/8 {
		t.Errorf("only %d LEDs went off across a beat, want a fresh random pattern", wentOff)
	}

	// The clearing goes from all lit to just the new minute's few.
	prev := n + 1
	for d := time.Duration(0); d < roll; d += 50 * time.Millisecond {
		got := litSeconds(at(d))
		if d == 0 && got != n {
			t.Errorf("%d LEDs lit at the start of the clearing, want %d", got, n)
		}
		if got > prev+1 {
			t.Errorf("clearing went up from %d to %d LEDs at %s", prev, got, d)
		}
		prev = got
	}
	if *at(roll) != *SecondsFrame(base.Add(roll), Fill{Mode: FillDisco}, time.Second, 0, fillBy) {
		t.Error("clearing still showing after roll")
	}
}

// level undoes brightness, giving an LED's 0-1 level.
func level(b byte) float64 { return math.Sqrt(float64(b) / 255) }

func TestTopo(t *testing.T) {
	const fillBy = time.Minute - 50*time.Millisecond
	const roll = time.Second
	const topoN = 5
	base := time.Date(2026, 1, 1, 18, 8, 0, 0, time.Local)
	at := func(d time.Duration) *Frame {
		return SecondsFrame(base.Add(d), Fill{Mode: FillTopo, N: topoN}, time.Second, roll, fillBy)
	}
	mean := func(f *Frame) float64 {
		sum := 0.0
		for x := 0; x < Width; x++ {
			for y := 0; y < Height; y++ {
				sum += level(f[x][y])
			}
		}
		return sum / (Width * Height)
	}

	// Average brightness tracks the minute; levels only rise, and smoothly.
	var last *Frame
	for d := roll; d < fillBy; d += 50 * time.Millisecond {
		f := at(d)
		if m, want := mean(f), float64(d)/float64(fillBy); math.Abs(m-want) > 0.02 {
			t.Fatalf("at %s mean level %.3f, want %.3f", d, m, want)
		}
		if last != nil {
			for x := 0; x < Width; x++ {
				for y := 0; y < Height; y++ {
					if f[x][y] < last[x][y] {
						t.Fatalf("at %s LED (%d,%d) dimmed from %d to %d", d, x, y, last[x][y], f[x][y])
					}
					if jump := level(f[x][y]) - level(last[x][y]); jump > 0.25 {
						t.Fatalf("at %s LED (%d,%d) jumped %.2f in one refresh", d, x, y, jump)
					}
				}
			}
		}
		last = f
	}
	end := at(fillBy)
	for x := 0; x < Width; x++ {
		for y := 0; y < Height; y++ {
			if end[x][y] != 255 {
				t.Fatalf("LED (%d,%d) = %d at fillBy, want 255", x, y, end[x][y])
			}
		}
	}

	// Picks pull their neighbours part way: mid-minute many LEDs are
	// partly lit, and fully lit LEDs have lit neighbours.
	mid := at(30 * time.Second)
	partial := 0
	for x := 0; x < Width; x++ {
		for y := 0; y < Height; y++ {
			if v := mid[x][y]; v > 0 && v < 250 {
				partial++
			}
			if mid[x][y] < 255 {
				continue
			}
			for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				nx, ny := x+d[0], y+d[1]
				if nx >= 0 && nx < Width && ny >= 0 && ny < Height && mid[nx][ny] == 0 {
					t.Errorf("LED (%d,%d) is full but its neighbour (%d,%d) is dark", x, y, nx, ny)
				}
			}
		}
	}
	if partial < Width*Height/3 {
		t.Errorf("only %d LEDs partly lit at :30, want the pull to leave many in between", partial)
	}

	// topoN picks are fading in at once.
	for _, n := range []int{1, 5, 12} {
		tf := newTopoField(Width, base.Unix()/60, fillBy, n)
		for tau := fillBy / 10; tau < fillBy*9/10; tau += fillBy / 37 {
			fading := 0
			for i := range tf.order {
				start := time.Duration(i) * tf.step
				if start < tau && tau < start+tf.fade {
					fading++
				}
			}
			if fading < n-1 || fading > n {
				t.Errorf("topoN=%d: %d picks fading at pick time %s", n, fading, tau)
			}
		}
	}

	// The clearing rewinds from full to just the new minute's start.
	prevMean := 2.0
	for d := time.Duration(0); d < roll; d += 50 * time.Millisecond {
		m := mean(at(d))
		if d == 0 && m != 1 {
			t.Errorf("mean level %.3f at the start of the clearing, want 1", m)
		}
		if m > prevMean {
			t.Errorf("clearing brightened from %.3f to %.3f at %s", prevMean, m, d)
		}
		prevMean = m
	}
	if *at(roll) != *SecondsFrame(base.Add(roll), Fill{Mode: FillTopo, N: topoN}, time.Second, 0, fillBy) {
		t.Error("clearing still showing after roll")
	}
}

func TestParseFill(t *testing.T) {
	for _, c := range []struct {
		in   string
		want Fill
	}{
		{"slideup", Fill{Mode: FillSlideUp}},
		{"random", Fill{Mode: FillRandom}},
		{"creepup", Fill{Mode: FillCreepUp}},
		{"slidedown", Fill{Mode: FillSlideDown}},
		{"creepdown", Fill{Mode: FillCreepDown}},
		{"spiral", Fill{Mode: FillSpiral}},
		{"disco", Fill{Mode: FillDisco}},
		{"topo", Fill{Mode: FillTopo, N: 5}},
		{"topo,2", Fill{Mode: FillTopo, N: 2}},
		{"infect", Fill{Mode: FillInfect, N: 3}},
		{"infect,8", Fill{Mode: FillInfect, N: 8}},
		{"binary", Fill{Mode: FillBinary}},
		{"shuffle", Fill{Mode: FillShuffle, N: 60}},
		{"shuffle,5", Fill{Mode: FillShuffle, N: 5}},
	} {
		got, err := ParseFill(c.in)
		if err != nil || got != c.want {
			t.Errorf("ParseFill(%q) = %+v, %v; want %+v", c.in, got, err, c.want)
		}
		if again, _ := ParseFill(got.String()); again != got {
			t.Errorf("ParseFill(%q).String() = %q doesn't parse back", c.in, got.String())
		}
	}
	for _, in := range []string{"", "nope", "slideup,2", "disco,", "topo,", "topo,0", "topo,-1", "topo,x", "topo,2,3", "Topo", "infect,0", "infect,x", "binary,2", "shuffle,0", "shuffle,x"} {
		if f, err := ParseFill(in); err == nil {
			t.Errorf("ParseFill(%q) = %+v, want an error", in, f)
		}
	}
}

func TestInfect(t *testing.T) {
	const fillBy = time.Minute - 50*time.Millisecond
	const roll = time.Second
	base := time.Date(2026, 1, 1, 18, 8, 0, 0, time.Local)
	for _, cols := range []int{Width, compactSecondsCols} {
		for _, seeds := range []int{1, 3, 8} {
			fill := Fill{Mode: FillInfect, N: seeds}
			at := func(d time.Duration) *Frame {
				var f Frame
				drawSeconds(&f, base.Add(d), fill, time.Second, roll, fillBy, 0, cols)
				return &f
			}
			mean := func(f *Frame) float64 {
				sum := 0.0
				for x := 0; x < cols; x++ {
					for y := 0; y < Height; y++ {
						sum += level(f[x][y])
					}
				}
				return sum / float64(cols*Height)
			}
			name := fmt.Sprintf("cols=%d seeds=%d", cols, seeds)

			// It starts from the seeds: light spreads one LED a step, so
			// after two steps only they and their neighbours are lit.
			tf := newInfectField(cols, base.Unix()/60, seeds)
			if len(tf.seeds) != seeds {
				t.Fatalf("%s: %d seeds", name, len(tf.seeds))
			}
			early := tf.levels[2]
			for led, l := range early {
				if l == 0 {
					continue
				}
				near := false
				for _, s := range tf.seeds {
					dx, dy := led%cols-s%cols, led/cols-s/cols
					near = near || (dx*dx <= 1 && dy*dy <= 1)
				}
				if !near {
					t.Errorf("%s: LED %d lit at step 2, not next to a seed", name, led)
				}
			}

			// The light needed is doled out so the average tracks the
			// minute. (Checked on the simulation's levels: the display
			// rounds the faintest to off.)
			for k, levels := range tf.levels {
				sum := 0.0
				for _, l := range levels {
					sum += float64(l)
				}
				if m, want := sum/float64(len(levels)), float64(k)/infectSteps; math.Abs(m-want) > 0.005 {
					t.Fatalf("%s: after step %d mean level %.3f, want %.3f", name, k, m, want)
				}
			}

			// LEDs only brighten, and gradually.
			var last *Frame
			for d := roll; d < fillBy; d += 50 * time.Millisecond {
				f := at(d)
				if last != nil {
					for x := 0; x < cols; x++ {
						for y := 0; y < Height; y++ {
							if f[x][y] < last[x][y] {
								t.Fatalf("%s: at %s LED (%d,%d) dimmed", name, d, x, y)
							}
							if jump := level(f[x][y]) - level(last[x][y]); jump > 0.15 {
								t.Fatalf("%s: at %s LED (%d,%d) jumped %.2f in a refresh", name, d, x, y, jump)
							}
						}
					}
				}
				last = f
			}
			// Everything lit by the end, the last LEDs already well on
			// their way rather than popping on.
			end := at(fillBy)
			for x := 0; x < cols; x++ {
				for y := 0; y < Height; y++ {
					if end[x][y] != 255 {
						t.Fatalf("%s: LED (%d,%d) = %d at fillBy, want 255", name, x, y, end[x][y])
					}
				}
			}
			for led, l := range tf.levels[infectSteps-20] {
				if l < 0.3 {
					t.Errorf("%s: LED %d only at %.2f a second before the end", name, led, l)
				}
			}

			// The clearing rewinds from full back to the new minute's
			// seeds.
			prev := 2.0
			for d := time.Duration(0); d < roll; d += 50 * time.Millisecond {
				m := mean(at(d))
				if d == 0 && m != 1 {
					t.Errorf("%s: mean %.3f at the start of the clearing, want 1", name, m)
				}
				if m > prev {
					t.Errorf("%s: clearing brightened at %s", name, d)
				}
				prev = m
			}
			var after Frame
			drawSeconds(&after, base.Add(roll), fill, time.Second, 0, fillBy, 0, cols)
			if *at(roll) != after {
				t.Errorf("%s: clearing still showing after roll", name)
			}
		}
	}
}

func TestBinaryLayout(t *testing.T) {
	// The top of the full layout, boxes numbered from 0.
	top := []string{
		"013355555",
		"223355555",
		"444455555",
		"444455555",
		"666666666",
	}
	box, _ := binaryLayout(Width)
	for y, row := range top {
		for x := range row {
			if got := box[(Height-1-y)*Width+x]; got != int(row[x]-'0') {
				t.Errorf("LED (%d,%d) from the top left in box %d, want %c", x, y, got, row[x])
			}
		}
	}
	for _, c := range []struct {
		cols  int
		areas string
	}{
		{Width, "[1 1 2 4 8 20 36 72 162]"},
		{compactSecondsCols, "[1 2 3 6 12 24 54]"},
	} {
		cols, areas := c.cols, c.areas
		box, n := binaryLayout(cols)
		size := make([]int, n)
		for _, b := range box {
			if b < 0 || b >= n {
				t.Fatalf("cols=%d: LED in box %d, want 0-%d", cols, b, n-1)
			}
			size[b]++
		}
		if fmt.Sprint(size) != areas {
			t.Errorf("cols=%d: box sizes %v, want %v", cols, size, areas)
		}
		// Every box is in one piece.
		for b := 0; b < n; b++ {
			start := -1
			for led, lb := range box {
				if lb == b {
					start = led
					break
				}
			}
			seen := map[int]bool{start: true}
			stack := []int{start}
			for len(stack) > 0 {
				led := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				x, y := led%cols, led/cols
				for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
					nx, ny := x+d[0], y+d[1]
					nb := ny*cols + nx
					if nx >= 0 && nx < cols && ny >= 0 && ny < Height && box[nb] == b && !seen[nb] {
						seen[nb] = true
						stack = append(stack, nb)
					}
				}
			}
			if len(seen) != size[b] {
				t.Errorf("cols=%d: box %d is in pieces (%d of %d LEDs connected)", cols, b, len(seen), size[b])
			}
		}
	}
}

func TestBinaryCounts(t *testing.T) {
	const fillBy = time.Minute - 50*time.Millisecond
	box, n := binaryLayout(Width)
	step := fillBy / time.Duration(1<<n-1)
	for _, count := range []int{0, 1, 2, 3, 4, 100, 1<<n - 1} {
		// Partway into the step, with the roll over.
		at := time.Duration(count)*step + step/2
		if count == 1<<n-1 {
			at = fillBy
		}
		var f Frame
		drawSeconds(&f, time.Date(2026, 1, 1, 18, 8, 0, 0, time.Local).Add(at), Fill{Mode: FillBinary}, time.Second, 0, fillBy, 0, Width)
		for led, b := range box {
			want := byte(0)
			if count>>b&1 == 1 {
				want = 255
			}
			if got := f[led%Width][Height-1-led/Width]; got != want {
				t.Fatalf("count %d: LED %d (box %d) = %d, want %d", count, led, b, got, want)
			}
		}
	}
}

func TestCentisFrame(t *testing.T) {
	now := time.Date(2026, 1, 1, 18, 8, 42, 370_000_000, time.Local)
	f := CentisFrame(now)
	// Laid out as the clock's HH over MM would be.
	var want Frame
	for i, c := range "4237" {
		drawDigit(&want, i, c, clockX)
	}
	if *f != want {
		t.Error("CentisFrame at :42.37 doesn't match the clock layout of 42 over 37")
	}
	// Centiseconds truncate rather than round.
	if *CentisFrame(now.Add(9 * time.Millisecond)) != *f {
		t.Error("CentisFrame changed within a centisecond")
	}
	if *CentisFrame(now.Add(10 * time.Millisecond)) == *f {
		t.Error("CentisFrame didn't change after a centisecond")
	}
}

func TestCompactCentisFrame(t *testing.T) {
	now := time.Date(2026, 1, 1, 18, 8, 5, 90_000_000, time.Local)
	f := CompactCentisFrame(now, time.Second)
	clock := CompactFrame(now, Fill{Mode: FillSlideUp}, 0, time.Second, time.Minute)
	x0 := Width - compactSecondsCols
	for y := 0; y < Height; y++ {
		for x := 0; x < x0; x++ {
			if f[x][y] != clock[x][y] {
				t.Errorf("clock pixel (%d,%d) = %d, want %d", x, y, f[x][y], clock[x][y])
			}
		}
	}
	// 05.09, each 3x5 digit centred in its clock digit's rows.
	for i, c := range "0509" {
		top := digitRows[i] + 1
		for gy := -1; gy < 6; gy++ {
			for gx := 0; gx < 3; gx++ {
				want := byte(0)
				if gy >= 0 && gy < 5 && smallDigits[c][gy][gx] == '#' {
					want = digitLevel
				}
				if got := f[x0+gx][top+gy]; got != want {
					t.Errorf("digit %d pixel (%d,%d) = %d, want %d", i, gx, gy, got, want)
				}
			}
		}
	}
}

func TestAlignToCentisecond(t *testing.T) {
	ms := time.Millisecond
	for _, tc := range []struct {
		intoMs, d, want time.Duration
	}{
		{3 * ms, 10 * ms, 7 * ms},
		{0, 10 * ms, 10 * ms},
		{9*ms + 999_999, 10 * ms, 1},
		{3 * ms, 50 * ms, 47 * ms},
		{3 * ms, 2 * ms, 7 * ms},
	} {
		now := time.Date(2026, 1, 1, 0, 0, 0, int(tc.intoMs), time.Local)
		if got := alignToCentisecond(now, tc.d); got != tc.want {
			t.Errorf("alignToCentisecond(+%s, %s) = %s, want %s", tc.intoMs, tc.d, got, tc.want)
		}
	}
}

func TestShuffleAt(t *testing.T) {
	f := Fill{Mode: FillShuffle}
	seen := map[FillMode]bool{}
	for hour := int64(0); hour < 200; hour++ {
		first := f.At(hour * 60)
		if first.Mode == FillShuffle {
			t.Fatalf("hour %d resolved to shuffle", hour)
		}
		for _, m := range []int64{1, 30, 59} {
			if got := f.At(hour*60 + m); got != first {
				t.Fatalf("hour %d: minute %d gave %v, want %v", hour, m, got, first)
			}
		}
		seen[first.Mode] = true
	}
	if len(seen) != int(FillShuffle) {
		t.Errorf("shuffle picked %d modes in 200 hours, want all %d", len(seen), FillShuffle)
	}
	if got := (Fill{Mode: FillSpiral}).At(5); got.Mode != FillSpiral {
		t.Errorf("non-shuffle fill changed: %v", got)
	}
}

func TestShuffleEveryMinutes(t *testing.T) {
	f := Fill{Mode: FillShuffle, N: 7}
	changes := 0
	for minute := int64(0); minute < 7*100; minute++ {
		got := f.At(minute)
		if minute%7 != 0 && got != f.At(minute-1) {
			t.Fatalf("minute %d changed mid-period", minute)
		}
		if minute > 0 && got != f.At(minute-1) {
			changes++
		}
	}
	if changes < 50 {
		t.Errorf("only %d changes in 100 periods", changes)
	}
}
