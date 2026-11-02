package core

import (
	"math"
	"sync"
	"time"
)

// A track from successive location fixes: the platform hands over latitude and
// longitude, the core turns them into what a person asks — how far have I
// moved, in which direction, how fast.
//
// The maths is plain Go, so it's tested against known distances instead of a
// phone in a pocket.

const earthRadius = 6371008.8 // metres, mean radius (IUGG)

// GeoFix is one reading: where, and how sure the device is (metres).
type GeoFix struct {
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	Accuracy float64 `json:"accuracy"`
}

// GeoPoint is the track after a fix: where you are now, and what the track
// has done since it started.
type GeoPoint struct {
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	Accuracy float64 `json:"accuracy"`
	Fixes    int     `json:"fixes"`
	Moved    float64 `json:"moved"`   // metres walked since the first fix
	Bearing  float64 `json:"bearing"` // degrees from the first fix, 0 = north
	Speed    float64 `json:"speed"`   // metres/second, smoothed
	Seconds  float64 `json:"seconds"` // since the first fix
}

// Distance is the great-circle distance between two points, in metres
// (haversine: fine at these scales, and no convergence traps near the poles).
func Distance(a, b GeoFix) float64 {
	lat1 := a.Lat * math.Pi / 180
	lat2 := b.Lat * math.Pi / 180
	dLat := lat2 - lat1
	dLon := (b.Lon - a.Lon) * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadius * math.Asin(math.Sqrt(math.Min(1, h)))
}

// Bearing is the initial compass bearing from a to b, in degrees (0 = north,
// 90 = east).
func Bearing(a, b GeoFix) float64 {
	lat1 := a.Lat * math.Pi / 180
	lat2 := b.Lat * math.Pi / 180
	dLon := (b.Lon - a.Lon) * math.Pi / 180
	y := math.Sin(dLon) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(dLon)
	return math.Mod(math.Atan2(y, x)*180/math.Pi+360, 360)
}

// Track builds a GeoPoint out of the fixes a platform callback delivers. Safe
// for concurrent use: callbacks have a way of arriving on another thread.
type Track struct {
	mu    sync.Mutex
	first GeoFix
	last  GeoFix
	lastT time.Time
	start time.Time
	moved float64
	speed float64
	fixes int
}

// Fix adds a reading and reports the track so far. now is the moment the
// reading was taken (time.Now in the app).
func (t *Track) Fix(f GeoFix, now time.Time) GeoPoint {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.fixes == 0 {
		t.first, t.last, t.start, t.lastT = f, f, now, now
		t.fixes = 1
		return t.point(f)
	}

	step := Distance(t.last, f)
	dt := now.Sub(t.lastT).Seconds()

	// A step smaller than the fix's own accuracy is noise, not walking: standing
	// still with ±22 m of GPS jitter otherwise "walks" hundreds of metres in a
	// minute. The fix still counts, and the last position still moves with it —
	// otherwise one bad fix would poison the next step.
	walks := step > max(t.last.Accuracy, 2)

	if walks || dt > 1 {
		if walks {
			t.moved += step
		}
		if dt > 0.05 && walks {
			// Smoothed, so one noisy pair doesn't swing the number the UI shows —
			// but the first measurement is the first measurement: damping it
			// against a made-up zero would understate a real walk.
			instant := step / dt
			if t.speed == 0 {
				t.speed = instant
			} else {
				t.speed = 0.6*t.speed + 0.4*instant
			}
		}
		t.last, t.lastT = f, now
	}
	t.fixes++
	return t.point(f)
}

func (t *Track) point(f GeoFix) GeoPoint {
	p := GeoPoint{
		Lat:      f.Lat,
		Lon:      f.Lon,
		Accuracy: f.Accuracy,
		Fixes:    t.fixes,
		Moved:    t.moved,
		Speed:    t.speed,
		Seconds:  t.lastT.Sub(t.start).Seconds(),
	}
	if t.fixes > 1 {
		p.Bearing = Bearing(t.first, f)
	}
	return p
}

// Reset throws the track away (the UI stopped listening, or started again
// somewhere else). Fields are cleared one by one on purpose: `*t = Track{}`
// would reset the mutex too, and the deferred Unlock would then unlock an
// unlocked one — a panic, not a fresh track.
func (t *Track) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.first, t.last = GeoFix{}, GeoFix{}
	t.start, t.lastT = time.Time{}, time.Time{}
	t.moved, t.speed, t.fixes = 0, 0, 0
}
