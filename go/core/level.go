package core

import (
	"math"
	"sync"
	"time"
)

// Level is what the core makes of the accelerometer: how far the phone is
// tipped, smoothed so the number doesn't shake, and how fast samples arrive.
//
// The sensor calls this 50 times a second from the UI thread; it does the
// arithmetic in Go because that's the app's business, not the UI's.
type Level struct {
	mu       sync.Mutex
	x, y, z  float64 // smoothed gravity, m/s²
	primed   bool
	alpha    float64 // smoothing: how much of a new sample to believe
	samples  int
	since    time.Time
	perSec   float64
	onChange func(LevelReading)
}

// LevelReading is what the UI gets: degrees, and the sample rate.
type LevelReading struct {
	Pitch float64 `json:"pitch"` // tipped up or down, degrees
	Roll  float64 `json:"roll"`  // tipped left or right, degrees
	Rate  float64 `json:"rate"`  // samples a second the sensor is giving us
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Z     float64 `json:"z"`
}

// NewLevel returns a level with a 1/5 weight on each new sample: fast enough to
// feel live, slow enough that the bubble doesn't jitter.
func NewLevel(onChange func(LevelReading)) *Level {
	return &Level{alpha: 0.2, onChange: onChange, since: time.Now()}
}

// Add takes one accelerometer sample (m/s², gravity included) and reports the
// level, at most once a second for the rate.
func (l *Level) Add(x, y, z float64) {
	l.mu.Lock()
	if !l.primed {
		l.x, l.y, l.z = x, y, z
		l.primed = true
	} else {
		l.x += l.alpha * (x - l.x)
		l.y += l.alpha * (y - l.y)
		l.z += l.alpha * (z - l.z)
	}
	l.samples++
	elapsed := time.Since(l.since).Seconds()
	if elapsed >= 1 {
		l.perSec = float64(l.samples) / elapsed
		l.samples = 0
		l.since = time.Now()
	}
	reading := LevelReading{
		Pitch: l.pitch(),
		Roll:  l.roll(),
		Rate:  l.perSec,
		X:     l.x, Y: l.y, Z: l.z,
	}
	onChange := l.onChange
	l.mu.Unlock()
	if onChange != nil {
		onChange(reading)
	}
}

// pitch is how far the top of the phone is tipped toward or away from you.
func (l *Level) pitch() float64 {
	return degrees(math.Atan2(-l.y, math.Hypot(l.x, l.z)))
}

// roll is how far it's tipped left or right.
func (l *Level) roll() float64 {
	return degrees(math.Atan2(l.x, math.Hypot(l.y, l.z)))
}

func degrees(rad float64) float64 { return rad * 180 / math.Pi }
