package main

import (
	"testing"
	"time"
)

func TestBrightnessSchedule(t *testing.T) {
	s := BrightnessSchedule{Day: 200, Night: 20, Dawn: 7 * time.Hour, Dusk: 21 * time.Hour, Transition: time.Hour}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		h, m int
		want byte
	}{
		{0, 0, 20},
		{6, 59, 20},
		{7, 0, 20},
		{7, 30, 110},
		{8, 0, 200},
		{12, 0, 200},
		{21, 0, 200},
		{21, 30, 110},
		{22, 0, 20},
		{23, 59, 20},
	}
	for _, c := range cases {
		got := s.At(time.Date(2026, 1, 1, c.h, c.m, 0, 0, time.Local))
		if got != c.want {
			t.Errorf("%02d:%02d: got %d, want %d", c.h, c.m, got, c.want)
		}
	}
}

func TestBrightnessScheduleWrapsMidnight(t *testing.T) {
	// Night shift: "day" from 22:00 to 06:00, dusk ramp crosses nothing,
	// dawn ramp crosses midnight.
	s := BrightnessSchedule{Day: 100, Night: 0, Dawn: 23*time.Hour + 30*time.Minute, Dusk: 6 * time.Hour, Transition: time.Hour}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := s.At(time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)); got != 50 {
		t.Errorf("00:00: got %d, want 50", got)
	}
	if got := s.At(time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)); got != 0 {
		t.Errorf("12:00: got %d, want 0", got)
	}
}

func TestBrightnessScheduleValidate(t *testing.T) {
	bad := []BrightnessSchedule{
		{Dawn: 7 * time.Hour, Dusk: 7 * time.Hour},
		{Dawn: 7 * time.Hour, Dusk: 8 * time.Hour, Transition: 2 * time.Hour},
		{Dawn: 7 * time.Hour, Dusk: 6 * time.Hour, Transition: 2 * time.Hour},
		{Dawn: 7 * time.Hour, Dusk: 21 * time.Hour, Transition: -time.Minute},
	}
	for _, s := range bad {
		if s.Validate() == nil {
			t.Errorf("%+v: expected error", s)
		}
	}
}
