// Command led-clock drives two Framework 16 LED matrix modules: the left one
// shows the time (24h HH:MM), the right one fills up over each minute. With
// -compact left or -compact right it drives just that matrix, showing both.
// With -emulator it drives led-emulator's emulated matrices instead.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	leftPath := flag.String("left", "", "serial device of the left matrix (default: auto-detect)")
	rightPath := flag.String("right", "", "serial device of the right matrix (default: auto-detect)")
	interval := flag.Duration("interval", 50*time.Millisecond, "time to sleep between display refreshes")
	fade := flag.Duration("fade", time.Second, "how long each seconds LED takes to fade in")
	roll := flag.Duration("roll", time.Second, "how long the minute-change animations last: the seconds clearing (the fill in reverse) and the clock digit scramble (0 disables them)")
	rollInterval := flag.Duration("roll-interval", 16*time.Millisecond, "time to sleep between display refreshes during the minute-change animations")
	brightness := flag.Uint("brightness", 0, "LED brightness 1-255 (0 leaves it unchanged); the daytime brightness when -night-brightness is set")
	nightBrightness := flag.Uint("night-brightness", 0, "LED brightness 1-255 at night (0 disables dimming); requires -brightness")
	dawn := flag.String("dawn", "07:00", "time of day (HH:MM) to start brightening to -brightness")
	dusk := flag.String("dusk", "21:00", "time of day (HH:MM) to start dimming to -night-brightness")
	transition := flag.Duration("transition", time.Hour, "how long brightening and dimming take")
	emulate := flag.Bool("emulator", false, "drive emulated matrices instead of real ones: start led-emulator (from next to this program, or $PATH), use its ports and stop it on exit; not allowed with -left or -right")
	var compact compactFlag
	flag.Var(&compact, "compact", "drive a single matrix, left (the default) or right, e.g. -compact right: the time on its left and the seconds in its three rightmost columns")
	fillName := flag.String("fill", "slideup", "how the seconds display fills up, as mode[,parameters]: slideup (from the bottom, row by row), slidedown (from the top, row by row), random (random LEDs fade in), creepup (each column grows on its own at a wobbling speed), creepdown (likewise, from the top), spiral (clockwise round the edge from the top left, winding inwards), disco (random LEDs flash on and off, the share lit tracking the minute), topo[,N] (random pixels fade in N at a time, default 5, pulling their neighbours up with them), infect[,N] (light spreads out from N random seeds, default 3) or binary (boxes growing from the top left, each about doubling the area before it, count up in binary)")
	if err := parseArgs(flag.CommandLine, os.Args[1:], &compact); err != nil {
		fmt.Fprintln(os.Stderr, err)
		flag.Usage()
		os.Exit(2)
	}

	fill, err := ParseFill(*fillName)
	if err != nil {
		log.Fatalf("-fill: %v", err)
	}

	var schedule *BrightnessSchedule
	if *nightBrightness > 0 {
		if *brightness == 0 {
			log.Fatal("-night-brightness requires -brightness")
		}
		dawnAt, err := ParseTimeOfDay(*dawn)
		if err != nil {
			log.Fatalf("-dawn: %v", err)
		}
		duskAt, err := ParseTimeOfDay(*dusk)
		if err != nil {
			log.Fatalf("-dusk: %v", err)
		}
		schedule = &BrightnessSchedule{
			Day:        byte(min(*brightness, 255)),
			Night:      byte(min(*nightBrightness, 255)),
			Dawn:       dawnAt,
			Dusk:       duskAt,
			Transition: *transition,
		}
		if err := schedule.Validate(); err != nil {
			log.Fatal(err)
		}
		log.Printf("brightness %d by day, %d at night; dawn=%s dusk=%s transition=%s", schedule.Day, schedule.Night, *dawn, *dusk, *transition)
	}

	if *emulate {
		if *leftPath != "" || *rightPath != "" {
			log.Fatal("-emulator can't be used with -left or -right")
		}
		bin, err := FindEmulator()
		if err != nil {
			log.Fatal(err)
		}
		names := []string{"left", "right"}
		if compact.side != "" {
			names = []string{compact.side}
		}
		emu, ports, err := StartEmulator(bin, names)
		if err != nil {
			log.Fatal(err)
		}
		// Deferred first, so it runs last: after the displays are
		// blanked and the ports closed.
		defer func() {
			if err := emu.Stop(); err != nil {
				log.Print(err)
			}
		}()
		*leftPath, *rightPath = ports["left"], ports["right"]
	}

	// In compact mode, compactPath is the one matrix to drive: -left or
	// -right, whichever side was asked for.
	var compactPath string
	if compact.side != "" {
		pathFlag, otherFlag := "-left", "-right"
		path, other := leftPath, rightPath
		// Which of the found matrices is on this side, as in the
		// two-matrix layout.
		foundIdx := 1
		if compact.side == "right" {
			pathFlag, otherFlag = otherFlag, pathFlag
			path, other = other, path
			foundIdx = 0
		}
		if *other != "" {
			log.Fatalf("%s can't be used with -compact %s; use %s to pick the matrix", otherFlag, compact.side, pathFlag)
		}
		if *path == "" {
			found, err := FindMatrices()
			if err != nil {
				log.Fatalf("enumerating serial ports: %v", err)
			}
			if len(found) == 0 {
				log.Fatalf("no LED matrix found; use %s to specify", pathFlag)
			}
			// With only one attached, there's no telling which side it's
			// on, so use it.
			*path = found[min(foundIdx, len(found)-1)]
		}
		compactPath = *path
	} else if *leftPath == "" || *rightPath == "" {
		found, err := FindMatrices()
		if err != nil {
			log.Fatalf("enumerating serial ports: %v", err)
		}
		if len(found) < 2 {
			log.Fatalf("need 2 LED matrices, found %d %v; use -left/-right to specify", len(found), found)
		}
		if *leftPath == "" {
			*leftPath = found[1]
		}
		if *rightPath == "" {
			*rightPath = found[0]
		}
	}
	if compact.side != "" {
		log.Printf("compact %s matrix=%s interval=%s fill=%s fade=%s roll=%s", compact.side, compactPath, *interval, fill, *fade, *roll)
	} else {
		log.Printf("left=%s right=%s interval=%s fill=%s fade=%s roll=%s", *leftPath, *rightPath, *interval, fill, *fade, *roll)
	}

	// open opens the matrix at path, adding it to matrices.
	var matrices []*Matrix
	open := func(path string) *Matrix {
		m, err := OpenMatrix(path)
		if err != nil {
			log.Fatal(err)
		}
		matrices = append(matrices, m)
		return m
	}
	var left, right, single *Matrix
	if compact.side != "" {
		single = open(compactPath)
	} else {
		left = open(*leftPath)
		right = open(*rightPath)
	}
	defer func() {
		for _, m := range matrices {
			m.Close()
		}
	}()

	// setBrightness sends b to every matrix unless it was already sent.
	// A failed send is retried on the next refresh.
	var lastBrightness int = -1
	setBrightness := func(b byte) {
		if int(b) == lastBrightness {
			return
		}
		lastBrightness = int(b)
		for _, m := range matrices {
			if err := m.SetBrightness(b); err != nil {
				log.Print(err)
				lastBrightness = -1
			}
		}
	}
	if schedule == nil && *brightness > 0 {
		setBrightness(byte(min(*brightness, 255)))
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Finish the fill one refresh early so the last frame of each minute
	// is guaranteed to show every LED fully lit.
	fillBy := time.Minute - *interval

	refresh := func() {
		now := time.Now()
		if schedule != nil {
			setBrightness(schedule.At(now))
		}
		if single != nil {
			if err := single.Draw(CompactFrame(now, fill, *fade, *roll, fillBy)); err != nil {
				log.Print(err)
			}
			return
		}
		if err := left.Draw(ClockFrame(now, *roll)); err != nil {
			log.Print(err)
		}
		if err := right.Draw(SecondsFrame(now, fill, *fade, *roll, fillBy)); err != nil {
			log.Print(err)
		}
	}

	// nextDelay is how long to sleep before the next refresh: short during
	// the minute-change animations so they run smoothly, and never past the
	// start of the next minute so they begin on time.
	nextDelay := func() time.Duration {
		now := time.Now()
		intoMinute := time.Duration(now.Second())*time.Second + time.Duration(now.Nanosecond())
		if intoMinute < *roll {
			return *rollInterval
		}
		return min(*interval, time.Minute-intoMinute)
	}

	refresh()
	timer := time.NewTimer(nextDelay())
	defer timer.Stop()
	for {
		select {
		case sig := <-sigs:
			log.Printf("received %s, clearing displays and exiting", sig)
			var blank Frame
			for _, m := range matrices {
				m.Draw(&blank)
			}
			return
		case <-timer.C:
			refresh()
			timer.Reset(nextDelay())
		}
	}
}
