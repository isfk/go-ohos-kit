// Benchmarks for the one place in this project where Go does real arithmetic:
// describing a photo's pixels. Everything else is a file read, a JSON marshal
// or a bridge call.
//
//	go test ./core/ -bench Analyze -benchmem
//
// The colours are the expensive part, so the synthetic picture below is a
// gradient with thousands of distinct colours. A flat one would fill a single
// bucket and make the bucketing look free — which is how a per-pixel map
// stayed in this function for a while.
package core

import "testing"

// photo is width×height RGBA with enough distinct colours to be a picture
// rather than a wall.
func photo(width, height int) []byte {
	return rgba(width, height, 0, func(x, y int) [3]uint8 {
		return [3]uint8{
			uint8((x * 7) % 256),
			uint8((y * 11) % 256),
			uint8((x*3 + y*5) % 256),
		}
	})
}

// BenchmarkAnalyzePixels240x320 is the app's workload: the platform decodes
// and downscales a photo, and the core gets 300 KB of RGBA.
func BenchmarkAnalyzePixels240x320(b *testing.B) {
	const w, h = 240, 320
	px := photo(w, h)
	b.SetBytes(int64(len(px)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := AnalyzePixels(px, w, h, 0); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAnalyzePixels10MP is the same maths on a full-resolution photo. It
// should cost roughly what the 1 MP case costs: maxSamples caps the work, so
// "12 MP takes 300 ms" is a property of the old code, not of the algorithm.
func BenchmarkAnalyzePixels10MP(b *testing.B) {
	const w, h = 4000, 2500
	px := photo(w, h)
	b.SetBytes(int64(len(px)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := AnalyzePixels(px, w, h, 0); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAnalyzePixelsPaddedStride keeps the padding honest at speed: the
// junk between rows must never be read as a pixel, and that check has to stay
// free on the hot path.
func BenchmarkAnalyzePixelsPaddedStride(b *testing.B) {
	const w, h = 240, 320
	const stride = w*4 + 32 // what a platform buffer does
	px := rgba(w, h, stride, func(x, y int) [3]uint8 {
		return [3]uint8{uint8((x * 7) % 256), uint8((y * 11) % 256), 128}
	})
	b.SetBytes(int64(len(px)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := AnalyzePixels(px, w, h, stride); err != nil {
			b.Fatal(err)
		}
	}
}
