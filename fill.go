package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// FillMode is the order in which the seconds LEDs light up over a minute.
type FillMode int

const (
	// FillSlideUp lights LEDs from the bottom, row by row.
	FillSlideUp FillMode = iota
	// FillRandom lights LEDs in a random order, different each minute.
	FillRandom
	// FillCreepUp grows each column from the bottom on its own, its speed
	// swinging between fast and slow spells, steered so every column
	// reaches the top at the end of the minute.
	FillCreepUp
	// FillDisco flashes random LEDs on and off, the share of them on at any
	// moment matching how far through the minute it is.
	FillDisco
	// FillTopo fades random pixels in a few at a time, each one pulling its
	// neighbours part of the way up with it, so the light spreads in
	// smooth, growing patches.
	FillTopo
	// FillInfect lights a few random seeds at the start of the minute, and
	// the light spreads from them, each LED drawing in light in proportion
	// to how bright its neighbours are.
	FillInfect
	// FillSlideDown lights LEDs from the top, row by row.
	FillSlideDown
	// FillCreepDown is creepup upside down: each column grows from the top.
	FillCreepDown
	// FillSpiral lights LEDs clockwise round the edge from the top left,
	// winding inwards to the middle.
	FillSpiral
	// FillBinary splits the display into boxes that each about double the
	// area before them, growing from the top left, and counts up in binary
	// over the minute, box i lit while bit i of the
	// count is set.
	FillBinary
	// FillShuffle isn't a fill itself: it picks one of the others at random
	// each hour. Use Fill.At to get the mode in force for a minute.
	FillShuffle
)

var fillModeNames = map[FillMode]string{
	FillSlideUp:   "slideup",
	FillRandom:    "random",
	FillCreepUp:   "creepup",
	FillDisco:     "disco",
	FillTopo:      "topo",
	FillInfect:    "infect",
	FillSlideDown: "slidedown",
	FillCreepDown: "creepdown",
	FillSpiral:    "spiral",
	FillBinary:    "binary",
	FillShuffle:   "shuffle",
}

func (m FillMode) String() string { return fillModeNames[m] }

// Fill is a seconds fill mode with its parameters, as given to -fill:
// the mode's name, then any parameters separated by commas, e.g. "topo,2".
type Fill struct {
	Mode FillMode
	// N is the count parameter of the modes that take one: how many pixels
	// topo fades in at a time, or how many seeds infect starts from.
	N int
}

// fillCounts describes the modes that take a count parameter N: its
// default and what it counts, for error messages.
var fillCounts = map[FillMode]struct {
	def  int
	what string
}{
	FillTopo:   {5, "pixels fading in at a time"},
	FillInfect: {3, "seeds"},
	// shuffle's count is how many minutes it keeps each pick for.
	FillShuffle: {60, "minutes between changes"},
}

// ParseFill parses a -fill flag value.
func ParseFill(s string) (Fill, error) {
	name, rest, hasParams := strings.Cut(s, ",")
	var params []string
	if hasParams {
		params = strings.Split(rest, ",")
	}
	f := Fill{Mode: -1}
	for m, n := range fillModeNames {
		if n == name {
			f.Mode = m
		}
	}
	if f.Mode == -1 {
		return Fill{}, fmt.Errorf("unknown fill mode %q (want slideup, slidedown, random, creepup, creepdown, spiral, disco, topo, infect, binary or shuffle)", name)
	}
	count, takesCount := fillCounts[f.Mode]
	if !takesCount {
		if hasParams {
			return Fill{}, fmt.Errorf("%s takes no parameters", name)
		}
		return f, nil
	}
	f.N = count.def
	if len(params) > 1 {
		return Fill{}, fmt.Errorf("%s takes one parameter, the number of %s (e.g. %s,%d)", name, count.what, name, count.def)
	}
	if len(params) == 1 {
		n, err := strconv.Atoi(params[0])
		if err != nil || n < 1 {
			return Fill{}, fmt.Errorf("%s: %q isn't a number of %s of at least 1", name, params[0], count.what)
		}
		f.N = n
	}
	return f, nil
}

// At returns the concrete fill in force during the given minute (Unix
// time / 60). Anything but shuffle is returned as is; shuffle picks a mode
// at random every f.N minutes (an hour if unset), as a pure function of
// which run of f.N minutes since the Unix epoch the minute falls in, with
// the picked mode's default parameters.
func (f Fill) At(minute int64) Fill {
	if f.Mode != FillShuffle {
		return f
	}
	period := int64(f.N)
	if period < 1 {
		period = int64(fillCounts[FillShuffle].def)
	}
	slot := minute / period
	if minute < 0 && minute%period != 0 {
		slot--
	}
	picks := make([]FillMode, 0, len(fillModeNames))
	for m := FillMode(0); m < FillShuffle; m++ {
		picks = append(picks, m)
	}
	m := picks[int(random(uint64(slot)^0x5eed)*float64(len(picks)))]
	pick := Fill{Mode: m}
	if c, ok := fillCounts[m]; ok {
		pick.N = c.def
	}
	return pick
}

func (f Fill) String() string {
	if _, ok := fillCounts[f.Mode]; ok {
		return fmt.Sprintf("%s,%d", f.Mode, f.N)
	}
	return f.Mode.String()
}

// fillStarts returns, for each of the cols*Height seconds LEDs (counted row
// by row from the bottom left), when in the given minute (Unix time / 60) it
// starts to fade in. The first starts at 0 and the last at fillBy.
//
// The result is a pure function of its arguments, so frames stay identical
// between refreshes, and is cached since creepup and creepdown take some
// computing.
func fillStarts(mode FillMode, cols int, minute int64, fillBy time.Duration) []time.Duration {
	key := fillKey{mode, cols, minute, fillBy}
	if s, ok := fillCache[key]; ok {
		return s
	}
	n := cols * Height
	starts := make([]time.Duration, n)
	switch mode {
	case FillCreepUp, FillCreepDown:
		for c := 0; c < cols; c++ {
			for row, start := range creepRowStarts(uint64(minute)<<8|uint64(c), fillBy) {
				if mode == FillCreepDown {
					row = Height - 1 - row
				}
				starts[row*cols+c] = start
			}
		}
	default:
		order := make([]int, 0, n)
		switch mode {
		case FillSlideDown:
			for row := Height - 1; row >= 0; row-- {
				for c := 0; c < cols; c++ {
					order = append(order, row*cols+c)
				}
			}
		case FillSpiral:
			order = spiralOrder(cols)
		default:
			for i := 0; i < n; i++ {
				order = append(order, i)
			}
			if mode == FillRandom {
				shuffle(order, uint64(minute))
			}
		}
		step := fillBy / time.Duration(n-1)
		for i, led := range order {
			starts[led] = time.Duration(i) * step
		}
	}
	// Only the current and previous minute are ever needed.
	if len(fillCache) >= 8 {
		clear(fillCache)
	}
	fillCache[key] = starts
	return starts
}

// spiralOrder returns the cols*Height LEDs (counted row by row from the
// bottom left) in spiral order: clockwise round the edge from the top left,
// then round each ring inside it in turn.
func spiralOrder(cols int) []int {
	order := make([]int, 0, cols*Height)
	// The ring's bounds, rows counted from the bottom.
	left, right, bottom, top := 0, cols-1, 0, Height-1
	for left <= right && bottom <= top {
		for c := left; c <= right; c++ {
			order = append(order, top*cols+c)
		}
		for r := top - 1; r >= bottom; r-- {
			order = append(order, r*cols+right)
		}
		if bottom < top {
			for c := right - 1; c >= left; c-- {
				order = append(order, bottom*cols+c)
			}
		}
		if left < right {
			for r := bottom + 1; r < top; r++ {
				order = append(order, r*cols+left)
			}
		}
		left, right, bottom, top = left+1, right-1, bottom+1, top-1
	}
	return order
}

type fillKey struct {
	mode   FillMode
	cols   int
	minute int64
	fillBy time.Duration
}

var fillCache = map[fillKey][]time.Duration{}

// shuffle permutes s (Fisher-Yates) as a pure function of seed, so the
// order stays the same for every refresh within a minute.
func shuffle(s []int, seed uint64) {
	for i := len(s) - 1; i > 0; i-- {
		j := int(random(seed<<16|uint64(i)) * float64(i+1))
		s[i], s[j] = s[j], s[i]
	}
}

// Creepup tuning. Each column's speed swings around its average between
// (1-creepSwing) and (1+creepSwing) times, with a random period in
// [creepMinPeriod, creepMaxPeriod]. creepFlatness squares the sine off, so
// it lingers at fast and slow and moves smoothly between them.
const (
	creepSwing     = 0.8
	creepMinPeriod = 12 * time.Second
	creepMaxPeriod = 30 * time.Second
	creepFlatness  = 2.5
	creepSamples   = 600
)

// creepRowStarts returns when each row of one creepup column starts to fade
// in, the column's height growing at a wobbling speed w(t).
//
// The column is steered by a guidance law: at every instant its speed is
// w(t) scaled by (distance left to climb) / (wobble left before fillBy).
// That keeps the wobble's shape while correcting for wherever the column
// has got to, so it always reaches the top exactly at fillBy, however its
// fast and slow spells fall. The law's exact solution is height =
// Height * W(t)/W(fillBy), with W the integral of w, which is what's computed
// here rather than stepping the controller frame by frame.
func creepRowStarts(seed uint64, fillBy time.Duration) []time.Duration {
	seed = seed<<4 | 0xc // keep clear of the other modes' random keys
	period := creepMinPeriod + time.Duration(random(seed|1)*float64(creepMaxPeriod-creepMinPeriod))
	phase := 2 * math.Pi * random(seed|2)
	norm := math.Tanh(creepFlatness)
	wobble := func(t time.Duration) float64 {
		s := math.Sin(2*math.Pi*float64(t)/float64(period) + phase)
		return 1 + creepSwing*math.Tanh(creepFlatness*s)/norm
	}

	// W[j] is the wobble integral up to sample j (trapezoid rule).
	dt := fillBy / creepSamples
	var W [creepSamples + 1]float64
	prev := wobble(0)
	for j := 1; j <= creepSamples; j++ {
		w := wobble(time.Duration(j) * dt)
		W[j] = W[j-1] + (prev+w)/2
		prev = w
	}

	// Row r starts when the column is r/(Height-1) of the way up, so the
	// bottom row starts at 0 and the top one at fillBy.
	starts := make([]time.Duration, Height)
	j := 0
	for r := 1; r < Height; r++ {
		target := W[creepSamples] * float64(r) / (Height - 1)
		for j < creepSamples-1 && W[j+1] < target {
			j++
		}
		frac := (target - W[j]) / (W[j+1] - W[j])
		starts[r] = time.Duration((float64(j) + min(max(frac, 0), 1)) * float64(dt))
	}
	starts[Height-1] = fillBy
	return starts
}

// discoBeat is roughly how long each of disco's random patterns is shown.
const discoBeat = 200 * time.Millisecond

// drawDisco lights a random fraction of the cols*Height LEDs (counted row by
// row from the bottom left) through pixel, at time at into the given minute.
//
// The minute up to fillBy is split into equal beats of about discoBeat, each
// showing a new random set of LEDs. Beat k of B lights k/(B-1) of them, so
// the first beat is dark, the last fully lit, and the share lit stays
// within a beat's worth of the share of the minute gone: about one LED on a
// full matrix. The count only
// changes on a beat: changing it mid-beat would add a lone LED to a pattern
// that's otherwise standing still, which looks like a stall.
func drawDisco(pixel func(led int) *byte, cols int, minute int64, at, fillBy time.Duration) {
	n := cols * Height
	beats := max(int((fillBy+discoBeat/2)/discoBeat), 2)
	beat := min(int(int64(at)*int64(beats)/int64(fillBy)), beats-1)
	if at < 0 {
		beat = 0
	}
	on := (n*beat + (beats-1)/2) / (beats - 1)
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	shuffle(order, (uint64(minute)<<12|uint64(beat))<<4|0xd)
	for _, led := range order[:on] {
		*pixel(led) = 255
	}
}

// Topo tuning. A pick lights its own pixel fully and pulls each pixel near
// it up to topoPull * exp(-d²/2σ²) of the way, d being the distance in
// pixels and σ topoSpread. topoPaceSamples is how finely the pacing (see
// topoField) is worked out.
const (
	topoPull        = 0.5
	topoSpread      = 1.0
	topoPaceSamples = 200
)

// topoReach is how far a pick's pull extends, in pixels: 3σ, past which
// the pull is negligible.
var topoReach = int(math.Ceil(3 * topoSpread))

// topoWeights[dy+topoReach][dx+topoReach] is how far a pick pulls the pixel
// (dx, dy) away from it.
var topoWeights = func() [][]float64 {
	w := make([][]float64, 2*topoReach+1)
	for dy := range w {
		w[dy] = make([]float64, 2*topoReach+1)
		for dx := range w[dy] {
			x, y := float64(dx-topoReach), float64(dy-topoReach)
			w[dy][dx] = topoPull * math.Exp(-(x*x+y*y)/(2*topoSpread*topoSpread))
		}
	}
	w[topoReach][topoReach] = 1
	return w
}()

// topoField is one minute of topo: every pixel is picked once, in a random
// order, and picks fade in topoN (Fill.N) at a time.
//
// Picks are spaced evenly along a "pick clock" running from 0 to fillBy.
// Left at that, the display would run well ahead of the minute, since early
// picks pull in lots of dark neighbours while late ones only top up nearly
// lit pixels. So the pick clock is paced against the real one: pace[k] is
// the pick time at which the average level reaches k/topoPaceSamples, and
// real time t maps to the pick time where the average is t/fillBy.
type topoField struct {
	cols  int
	order []int
	step  time.Duration
	fade  time.Duration
	pace  [topoPaceSamples + 1]time.Duration
}

// levels returns each LED's level (0-1) at pick time tau. Each pick covers
// the share of a pixel's remaining darkness given by its pull times how far
// it has faded in; that compounds, so a pixel left at 20% by one pick and
// pulled 25% by another is at 40%, and when it's picked itself it rises from
// there to 100%.
func (tf *topoField) levels(tau time.Duration) []float64 {
	dark := make([]float64, len(tf.order))
	for i := range dark {
		dark[i] = 1
	}
	for i, led := range tf.order {
		start := time.Duration(i) * tf.step
		if tau <= start {
			break
		}
		q := min(float64(tau-start)/float64(tf.fade), 1)
		q = q * q * (3 - 2*q)
		px, py := led%tf.cols, led/tf.cols
		for dy := -topoReach; dy <= topoReach; dy++ {
			y := py + dy
			if y < 0 || y >= Height {
				continue
			}
			for dx := -topoReach; dx <= topoReach; dx++ {
				x := px + dx
				if x < 0 || x >= tf.cols {
					continue
				}
				dark[y*tf.cols+x] *= 1 - q*topoWeights[dy+topoReach][dx+topoReach]
			}
		}
	}
	for i := range dark {
		dark[i] = 1 - dark[i]
	}
	return dark
}

// pickTime maps real time at (0 to fillBy) to pick time.
func (tf *topoField) pickTime(at, fillBy time.Duration) time.Duration {
	u := min(max(float64(at)/float64(fillBy), 0), 1) * topoPaceSamples
	k := min(int(u), topoPaceSamples-1)
	return tf.pace[k] + time.Duration((u-float64(k))*float64(tf.pace[k+1]-tf.pace[k]))
}

func newTopoField(cols int, minute int64, fillBy time.Duration, topoN int) *topoField {
	n := cols * Height
	topoN = max(topoN, 1)
	tf := &topoField{cols: cols, order: make([]int, n)}
	for i := range tf.order {
		tf.order[i] = i
	}
	shuffle(tf.order, uint64(minute)<<4|0xe)
	// The last pick finishes fading in exactly at fillBy.
	tf.step = fillBy / time.Duration(n-1+topoN)
	tf.fade = time.Duration(topoN) * tf.step

	// Sample the average level along the pick clock, then invert it.
	var avg [topoPaceSamples + 1]float64
	for j := 1; j <= topoPaceSamples; j++ {
		sum := 0.0
		for _, l := range tf.levels(fillBy * time.Duration(j) / topoPaceSamples) {
			sum += l
		}
		avg[j] = sum / float64(n)
	}
	avg[topoPaceSamples] = 1
	j := 0
	for k := 1; k < topoPaceSamples; k++ {
		target := float64(k) / topoPaceSamples
		for j < topoPaceSamples-1 && avg[j+1] < target {
			j++
		}
		frac := min(max((target-avg[j])/(avg[j+1]-avg[j]), 0), 1)
		tf.pace[k] = time.Duration((float64(j) + frac) * float64(fillBy) / topoPaceSamples)
	}
	tf.pace[topoPaceSamples] = fillBy
	return tf
}

type topoKey struct {
	cols   int
	minute int64
	fillBy time.Duration
	topoN  int
}

var topoCache = map[topoKey]*topoField{}

// drawTopo draws the topo fill at real time at into the given minute
// through pixel (LEDs counted row by row from the bottom left).
func drawTopo(pixel func(led int) *byte, cols int, minute int64, at, fillBy time.Duration, topoN int) {
	key := topoKey{cols, minute, fillBy, topoN}
	tf, ok := topoCache[key]
	if !ok {
		// Only the current and previous minute are ever needed.
		if len(topoCache) >= 4 {
			clear(topoCache)
		}
		tf = newTopoField(cols, minute, fillBy, topoN)
		topoCache[key] = tf
	}
	for led, l := range tf.levels(tf.pickTime(at, fillBy)) {
		*pixel(led) = max(*pixel(led), brightness(l))
	}
}

// Infect tuning. The simulation steps every fillBy/infectSteps (about 50ms,
// the default refresh). An LED's pull is the light around it, its direct
// neighbours counting 1 and its diagonals infectDiagonal, times the room it
// has left to brighten; seeds also pull infectSeedPull of their own, which
// is what lights them before anything else is lit. No LED brightens by more
// than infectMaxRise a step, so even a lone seed fades in over half a second.
const (
	infectSteps    = 1200
	infectDiagonal = 0.5
	infectSeedPull = 1.0
	infectMaxRise  = 0.1
)

// infectField is one minute of infect, simulated: levels[k][led] is each
// LED's level (0-1) after step k, LEDs counted row by row from the bottom
// left.
type infectField struct {
	levels [][]float32
	seeds  []int
}

// newInfectField simulates a minute of infect from seeds random seeds.
//
// Every step works out how much light is still needed for every LED to be
// full by the last step, and hands out that much divided by the steps left
// to go, each LED getting a share in proportion to its pull (see the infect
// tuning constants). So the light always spreads outwards from what's lit,
// at a pace that brings the whole display to full exactly at fillBy. An
// LED can't go past full; light it can't take goes back out to the rest in
// the same step, or failing that is added to later steps' budgets.
func newInfectField(cols int, minute int64, seeds int) *infectField {
	n := cols * Height
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	shuffle(order, uint64(minute)<<4|0xf)
	seedLEDs := order[:min(max(seeds, 1), n)]
	seedPull := make([]float64, n)
	for _, led := range seedLEDs {
		seedPull[led] = infectSeedPull
	}

	level := make([]float64, n)
	pull := make([]float64, n)
	rose := make([]float64, n) // how much each LED has brightened this step
	tf := &infectField{levels: make([][]float32, infectSteps+1), seeds: seedLEDs}
	tf.levels[0] = make([]float32, n)
	for k := 1; k <= infectSteps; k++ {
		// Pulls come from the previous step's levels.
		total := 0.0
		for led := range level {
			total += level[led]
			rose[led] = 0
			room := 1 - level[led]
			if room <= 0 {
				pull[led] = 0
				continue
			}
			x, y := led%cols, led/cols
			around := 0.0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := x+dx, y+dy
					if (dx == 0 && dy == 0) || nx < 0 || nx >= cols || ny < 0 || ny >= Height {
						continue
					}
					w := 1.0
					if dx != 0 && dy != 0 {
						w = infectDiagonal
					}
					around += w * level[ny*cols+nx]
				}
			}
			pull[led] = (around + seedPull[led]) * room
		}
		budget := (float64(n) - total) / float64(infectSteps-k+1)
		// Hand the budget out by pull, a few rounds so that what LEDs
		// near full, or rising as fast as they can, can't take goes to
		// the others.
		for round := 0; round < 4 && budget > 1e-12; round++ {
			sum := 0.0
			for _, p := range pull {
				sum += p
			}
			if sum == 0 {
				break
			}
			given := 0.0
			for led, p := range pull {
				if p == 0 {
					continue
				}
				add := min(budget*p/sum, 1-level[led], infectMaxRise-rose[led])
				level[led] += add
				rose[led] += add
				given += add
				if level[led] >= 1 || rose[led] >= infectMaxRise {
					pull[led] = 0
				}
			}
			budget -= given
		}
		if k == infectSteps {
			// Whatever couldn't be handed out (an LED with no lit
			// neighbours) gets topped up on the last step.
			for led := range level {
				level[led] = 1
			}
		}
		lv := make([]float32, n)
		for led, l := range level {
			lv[led] = float32(l)
		}
		tf.levels[k] = lv
	}
	return tf
}

type infectKey struct {
	cols   int
	minute int64
	seeds  int
}

var infectCache = map[infectKey]*infectField{}

// drawInfect draws the infect fill at time at into the given minute through
// pixel (LEDs counted row by row from the bottom left), blending between the
// simulation's steps.
func drawInfect(pixel func(led int) *byte, cols int, minute int64, at, fillBy time.Duration, seeds int) {
	key := infectKey{cols, minute, seeds}
	tf, ok := infectCache[key]
	if !ok {
		// Only the current and previous minute are ever needed.
		if len(infectCache) >= 4 {
			clear(infectCache)
		}
		tf = newInfectField(cols, minute, seeds)
		infectCache[key] = tf
	}
	u := min(max(float64(at)/float64(fillBy), 0), 1) * infectSteps
	k := min(int(u), infectSteps-1)
	frac := float32(u - float64(k))
	a, b := tf.levels[k], tf.levels[k+1]
	for led := range a {
		l := a[led] + (b[led]-a[led])*frac
		*pixel(led) = max(*pixel(led), brightness(float64(l)))
	}
}

// binaryLayout returns which box each of the cols*Height LEDs (counted row
// by row from the bottom left) belongs to, and the number of boxes.
//
// The boxes grow from the top left corner, each one about doubling the
// area covered so far: 1 is the corner LED, 2 the one to its right, 3 the
// two below them, 4 the 2x2 to the right, 5 the 4x2 below, and so on, each
// box widening the covered rectangle when it's no wider than it is tall and
// lengthening it otherwise. Once a box reaches an edge, any sliver it would
// leave there that's no bigger than the box itself goes to it as well, so
// on a 9-wide display box 6 takes 5 columns rather than leaving 1, and from
// then on the boxes are full-width strips, the last taking the rows left.
//
//	1 2 4 4 6 6 6 6 6
//	3 3 4 4 6 6 6 6 6
//	5 5 5 5 6 6 6 6 6
//	5 5 5 5 6 6 6 6 6
//	7 7 7 7 7 7 7 7 7
//	...
func binaryLayout(cols int) ([]int, int) {
	if l, ok := binaryLayouts[cols]; ok {
		return l.box, l.n
	}
	box := make([]int, cols*Height)
	// set puts the LEDs in columns x0 to x1-1, rows y0 to y1-1 counted
	// from the top, in box b.
	set := func(b, x0, x1, y0, y1 int) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				box[(Height-1-y)*cols+x] = b
			}
		}
	}
	// The covered rectangle, w wide and h tall, starts as box 0.
	w, h, n := 1, 1, 1
	for w < cols || h < Height {
		if (w <= h && w < cols) || h == Height {
			add := min(w, cols-w)
			if cols-w-add <= add {
				add = cols - w
			}
			set(n, w, w+add, 0, h)
			w += add
		} else {
			add := min(h, Height-h)
			if Height-h-add <= add {
				add = Height - h
			}
			set(n, 0, w, h, h+add)
			h += add
		}
		n++
	}
	binaryLayouts[cols] = binaryLayoutEntry{box, n}
	return box, n
}

type binaryLayoutEntry struct {
	box []int
	n   int
}

var binaryLayouts = map[int]binaryLayoutEntry{}

// drawBinary draws the binary fill at time at into the minute through
// pixel (LEDs counted row by row from the bottom left). With n boxes the
// count runs from 0 to 2^n-1, stepping every fillBy/(2^n-1), so the
// display is dark at the start of the minute and every box lit by fillBy.
func drawBinary(pixel func(led int) *byte, cols int, at, fillBy time.Duration) {
	box, n := binaryLayout(cols)
	steps := int64(1)<<n - 1
	count := min(max(int64(at)*steps/int64(fillBy), 0), steps)
	for led, b := range box {
		if count>>b&1 == 1 {
			*pixel(led) = 255
		}
	}
}
