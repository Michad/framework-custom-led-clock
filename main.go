// Command led-clock drives two Framework 16 LED matrix modules: the left one
// shows the time (24h HH:MM), the right one fills up over each minute.
package main

import (
	"flag"
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
	roll := flag.Duration("roll", time.Second, "how long the minute-change animations last: the seconds roll-down and the clock digit scramble (0 disables them)")
	rollInterval := flag.Duration("roll-interval", 16*time.Millisecond, "time to sleep between display refreshes during the roll-down")
	brightness := flag.Uint("brightness", 0, "LED brightness 1-255 (0 leaves it unchanged)")
	flag.Parse()

	if *leftPath == "" || *rightPath == "" {
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
	log.Printf("left=%s right=%s interval=%s fade=%s roll=%s", *leftPath, *rightPath, *interval, *fade, *roll)

	left, err := OpenMatrix(*leftPath)
	if err != nil {
		log.Fatal(err)
	}
	defer left.Close()
	right, err := OpenMatrix(*rightPath)
	if err != nil {
		log.Fatal(err)
	}
	defer right.Close()

	if *brightness > 0 {
		for _, m := range []*Matrix{left, right} {
			if err := m.SetBrightness(byte(min(*brightness, 255))); err != nil {
				log.Print(err)
			}
		}
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Finish the fill one refresh early so the last frame of each minute
	// is guaranteed to show every LED fully lit.
	fillBy := time.Minute - *interval

	refresh := func() {
		now := time.Now()
		if err := left.Draw(ClockFrame(now, *roll)); err != nil {
			log.Print(err)
		}
		if err := right.Draw(SecondsFrame(now, *fade, *roll, fillBy)); err != nil {
			log.Print(err)
		}
	}

	// nextDelay is how long to sleep before the next refresh: short during
	// the roll-down so it animates smoothly, and never past the start of the
	// next minute so the roll-down begins on time.
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
			left.Draw(&blank)
			right.Draw(&blank)
			return
		case <-timer.C:
			refresh()
			timer.Reset(nextDelay())
		}
	}
}
