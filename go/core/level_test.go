package core

import (
	"math"
	"testing"
	"time"
)

func TestLevelFlatIsZero(t *testing.T) {
	var got LevelReading
	l := NewLevel(func(r LevelReading) { got = r })
	// Face up on a table: gravity down the z axis.
	for i := 0; i < 50; i++ {
		l.Add(0, 0, 9.8)
	}
	if math.Abs(got.Pitch) > 1 || math.Abs(got.Roll) > 1 {
		t.Fatalf("flat: pitch %.1f roll %.1f", got.Pitch, got.Roll)
	}
}

func TestLevelTipped(t *testing.T) {
	var got LevelReading
	l := NewLevel(func(r LevelReading) { got = r })
	// Tipped 30° to the right: gravity splits between x and z.
	x := 9.8 * math.Sin(30*math.Pi/180)
	z := 9.8 * math.Cos(30*math.Pi/180)
	for i := 0; i < 200; i++ {
		l.Add(x, 0, z)
	}
	if math.Abs(got.Roll-30) > 1 {
		t.Fatalf("roll: %.1f, want ~30", got.Roll)
	}
	if math.Abs(got.Pitch) > 1 {
		t.Fatalf("pitch: %.1f, want ~0", got.Pitch)
	}
}

func TestLevelSmoothsASpike(t *testing.T) {
	var got LevelReading
	l := NewLevel(func(r LevelReading) { got = r })
	for i := 0; i < 50; i++ {
		l.Add(0, 0, 9.8)
	}
	l.Add(9.8, 0, 0) // a bump: the raw value would jump to 90°
	if got.Roll > 30 {
		t.Fatalf("one sample moved the bubble to %.1f°", got.Roll)
	}
}

func TestLevelRate(t *testing.T) {
	var got LevelReading
	l := NewLevel(func(r LevelReading) { got = r })
	// Two seconds of wall time and 100 samples in total: the rate the UI shows
	// is samples over elapsed, reported once a second.
	l.since = l.since.Add(-2 * time.Second)
	l.samples = 99
	l.Add(0, 0, 9.8)
	if got.Rate < 40 || got.Rate > 60 {
		t.Fatalf("rate: %.0f samples/s, want ~50", got.Rate)
	}
}
