package main

import (
	"testing"
	"time"
)

func TestFormatSeconds(t *testing.T) {
	cases := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"whole seconds", 30 * time.Second, "30s"},
		{"zero", 0, "0s"},
		{"half second", 500 * time.Millisecond, "0.5s"},
		{"one and a half seconds", 1500 * time.Millisecond, "1.5s"},
		{"tenth of a second", 100 * time.Millisecond, "0.1s"},
		{"no trailing zeros to trim", 33 * time.Millisecond, "0.033s"},
		{"quarter second", 1250 * time.Millisecond, "1.25s"},
		{"large duration stays in seconds, not minutes", 3661 * time.Second, "3661s"},
		{"sub-millisecond rounds away to zero", time.Nanosecond, "0s"},
		{"negative duration", -1500 * time.Millisecond, "-1.5s"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatSeconds(tc.d)
			if got != tc.want {
				t.Errorf("formatSeconds(%v) = %q, want %q", tc.d, got, tc.want)
			}
		})
	}
}

// formatSeconds is only fed durations parsed by time.ParseDuration on the
// way in, so round-tripping through it should reproduce the same duration
// for every value that survives the truncation to milliseconds.
func TestFormatSecondsRoundTrip(t *testing.T) {
	durations := []time.Duration{
		1 * time.Second,
		250 * time.Millisecond,
		2500 * time.Millisecond,
		60 * time.Second,
	}

	for _, d := range durations {
		s := formatSeconds(d)
		got, err := time.ParseDuration(s)
		if err != nil {
			t.Fatalf("formatSeconds(%v) produced %q, which failed to parse: %v", d, s, err)
		}
		if got != d {
			t.Errorf("round trip of %v through %q gave %v", d, s, got)
		}
	}
}
