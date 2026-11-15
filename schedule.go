package main

import (
	"fmt"
	"time"
)

const day = 24 * time.Hour

// BrightnessSchedule dims the matrices at night. From Dawn the brightness
// ramps up from Night to Day over Transition; from Dusk it ramps back down.
type BrightnessSchedule struct {
	Day, Night byte
	Dawn, Dusk time.Duration // time of day, since midnight
	Transition time.Duration
}

// ParseTimeOfDay parses "HH:MM" into a duration since midnight.
func ParseTimeOfDay(s string) (time.Duration, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, fmt.Errorf("invalid time of day %q (want HH:MM)", s)
	}
	return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute, nil
}

// Validate checks that both ramps fit between dawn and dusk.
func (s *BrightnessSchedule) Validate() error {
	dayLen := mod(s.Dusk-s.Dawn, day)
	if dayLen == 0 {
		return fmt.Errorf("dawn and dusk must differ")
	}
	if s.Transition < 0 || s.Transition > dayLen || s.Transition > day-dayLen {
		return fmt.Errorf("transition %s must fit within both day (%s) and night (%s)", s.Transition, dayLen, day-dayLen)
	}
	return nil
}

// At returns the brightness for the local time of day of t.
func (s *BrightnessSchedule) At(t time.Time) byte {
	tod := time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute + time.Duration(t.Second())*time.Second
	sinceDawn := mod(tod-s.Dawn, day)
	dayLen := mod(s.Dusk-s.Dawn, day)

	var level float64 // 0 = night, 1 = day
	if sinceDawn < dayLen {
		level = ramp(sinceDawn, s.Transition)
	} else {
		level = 1 - ramp(sinceDawn-dayLen, s.Transition)
	}
	return byte(float64(s.Night) + level*(float64(s.Day)-float64(s.Night)) + 0.5)
}

// ramp goes linearly from 0 to 1 as elapsed goes from 0 to length.
func ramp(elapsed, length time.Duration) float64 {
	if elapsed >= length {
		return 1
	}
	return float64(elapsed) / float64(length)
}

func mod(d, m time.Duration) time.Duration {
	return ((d % m) + m) % m
}
