package core

import (
	"math"
	"testing"
	"time"
)

var (
	paris  = GeoFix{Lat: 48.8566, Lon: 2.3522, Accuracy: 10}
	london = GeoFix{Lat: 51.5074, Lon: -0.1278, Accuracy: 10}
)

func TestDistanceBetweenCapitals(t *testing.T) {
	// Paris → London is 343.5 km, a number you can check on any map.
	got := Distance(paris, london)
	if math.Abs(got-343556) > 343556*0.005 {
		t.Errorf("Distance = %.0f m, want ~343556 m", got)
	}
}

func TestBearingFromParisToLondon(t *testing.T) {
	// North-north-west, 330° — the *initial* bearing. (The great circle bends:
	// you arrive in London on a 336° heading, which is the number that looks
	// familiar on a globe.)
	if got := Bearing(paris, london); math.Abs(got-330.2) > 0.5 {
		t.Errorf("Bearing = %.2f°, want ~330.2°", got)
	}
}

func TestDistanceAndBearingOnTheEquator(t *testing.T) {
	origin := GeoFix{Lat: 0, Lon: 0}
	east := GeoFix{Lat: 0, Lon: 1}
	north := GeoFix{Lat: 1, Lon: 0}

	// One degree of a great circle: 2πR/360.
	if got := Distance(origin, east); math.Abs(got-111195) > 200 {
		t.Errorf("Distance east = %.0f m, want ~111195 m", got)
	}
	if got := Bearing(origin, east); math.Abs(got-90) > 0.01 {
		t.Errorf("Bearing east = %.2f°, want 90°", got)
	}
	if got := Bearing(origin, north); math.Abs(got) > 0.01 {
		t.Errorf("Bearing north = %.2f°, want 0°", got)
	}
}

func TestTrackAddsUpTheWalk(t *testing.T) {
	var tr Track
	t0 := time.Unix(1700000000, 0)
	start := GeoFix{Lat: 0, Lon: 0, Accuracy: 5}
	one := GeoFix{Lat: 0, Lon: 0.001, Accuracy: 5} // ~111 m east
	two := GeoFix{Lat: 0, Lon: 0.002, Accuracy: 5} // another ~111 m east

	p := tr.Fix(start, t0)
	if p.Fixes != 1 || p.Moved != 0 || p.Bearing != 0 || p.Speed != 0 || p.Seconds != 0 {
		t.Errorf("first fix = %+v, want a track with nothing behind it", p)
	}

	p = tr.Fix(one, t0.Add(10*time.Second))
	if math.Abs(p.Moved-111.2) > 1 {
		t.Errorf("Moved = %.1f m after ~111 m, want ~111 m", p.Moved)
	}
	if math.Abs(p.Bearing-90) > 0.5 {
		t.Errorf("Bearing = %.1f°, want ~90° (east)", p.Bearing)
	}
	// 111 m in 10 s: the first measurement isn't damped towards zero.
	if p.Speed < 10.5 || p.Speed > 11.7 {
		t.Errorf("Speed = %.1f m/s, want ~11.1 m/s", p.Speed)
	}
	if math.Abs(p.Seconds-10) > 0.01 {
		t.Errorf("Seconds = %.1f, want 10", p.Seconds)
	}

	p = tr.Fix(two, t0.Add(20*time.Second))
	if math.Abs(p.Moved-222.4) > 2 {
		t.Errorf("Moved = %.1f m after two ~111 m legs, want ~222 m", p.Moved)
	}
	if p.Fixes != 3 {
		t.Errorf("Fixes = %d, want 3", p.Fixes)
	}
	if p.Speed < 10.5 || p.Speed > 11.7 {
		t.Errorf("Speed = %.1f m/s, want it held at ~11.1 m/s", p.Speed)
	}
}

func TestTrackIgnoresJitterWhenStandingStill(t *testing.T) {
	var tr Track
	t0 := time.Unix(1700000000, 0)
	// Standing still with a ±20 m fix, wandering 8 m each way: the numbers say
	// "no idea", so the track must not claim 500 m walked.
	here := GeoFix{Lat: 39.9879, Lon: 116.4719, Accuracy: 20}
	tr.Fix(here, t0)
	for i := 1; i <= 30; i++ {
		off := GeoFix{Lat: here.Lat + 0.00007, Lon: here.Lon, Accuracy: 20} // ~8 m
		tr.Fix(off, t0.Add(time.Duration(i)*time.Second))
		tr.Fix(here, t0.Add(time.Duration(i)*time.Second+500*time.Millisecond))
	}
	p := tr.Fix(here, t0.Add(40*time.Second))
	if p.Moved != 0 {
		t.Errorf("Moved = %.1f m while standing still in ±20 m noise, want 0", p.Moved)
	}
	if p.Speed != 0 {
		t.Errorf("Speed = %.2f m/s while standing still, want 0", p.Speed)
	}
	if p.Fixes != 62 {
		t.Errorf("Fixes = %d, want every reading counted (62)", p.Fixes)
	}
}

func TestTrackIgnoresARepeatedFix(t *testing.T) {
	var tr Track
	t0 := time.Unix(1700000000, 0)
	at := GeoFix{Lat: 31.2304, Lon: 121.4737, Accuracy: 8}

	tr.Fix(at, t0)
	// Same spot 200 ms later: standing still, and GPS says so twice.
	p := tr.Fix(at, t0.Add(200*time.Millisecond))
	if p.Moved != 0 || p.Speed != 0 {
		t.Errorf("Moved = %.3f, Speed = %.3f after a repeated fix, want 0", p.Moved, p.Speed)
	}
	if p.Fixes != 2 {
		t.Errorf("Fixes = %d, want 2: the reading is still counted", p.Fixes)
	}
}

func TestTrackResetStartsOver(t *testing.T) {
	var tr Track
	t0 := time.Unix(1700000000, 0)
	tr.Fix(GeoFix{Lat: 0, Lon: 0}, t0)
	tr.Fix(GeoFix{Lat: 0, Lon: 0.01}, t0.Add(time.Second))
	tr.Reset()

	p := tr.Fix(GeoFix{Lat: 10, Lon: 10}, t0.Add(2*time.Second))
	if p.Fixes != 1 || p.Moved != 0 || p.Bearing != 0 {
		t.Errorf("after Reset: %+v, want a fresh track", p)
	}
}
