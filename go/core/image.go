package core

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// What the core makes of a photo's pixels: the numbers behind "is this shot
// bright?", "what colour is it?", "where is the light?".
//
// Plain arithmetic on RGBA bytes, no decoding: the platform decodes (its image
// decoder handles HEIF and everything else the camera writes), the core does
// the maths — and that half is testable without a phone.

const (
	lumaBuckets = 8 // the histogram the UI draws as bars
	lumaGrid    = 8 // the coarse brightness map the UI draws as a square
	colorBits   = 4 // per channel: 4096 buckets, enough to name a colour
	topColors   = 3 //
	maxSamples  = 1 << 20
)

// ColorShare is one of the colours that make up the picture, with its share of
// the pixels (0..1).
type ColorShare struct {
	Hex   string  `json:"hex"`
	Share float64 `json:"share"`
}

// Analysis is what a picture is, in numbers.
type Analysis struct {
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	MeanLuma int    `json:"meanLuma"` // 0..100
	MeanRGB  [3]int `json:"meanRgb"`  // 0..255
	Histo    []int  `json:"histo"`    // lumaBuckets percentages, summing to 100
	Grid     []int  `json:"grid"`     // lumaGrid×lumaGrid brightness 0..100; a cell
	// that no sampled pixel lands in (only possible when the picture is
	// smaller than the grid) reads 0
	Colors []ColorShare `json:"colors"` // topColors, brightest share first
	Micros int64        `json:"micros"` // how long the maths took
}

// ErrBadPixels is what a buffer that isn't RGBA of the stated size gets.
var ErrBadPixels = errors.New("pixels: buffer doesn't match the size")

// AnalyzePixels reads RGBA_8888 pixels and describes them. stride is the
// number of bytes in a row (the platform's buffers are padded); pass 0 for a
// tightly packed picture.
func AnalyzePixels(rgba []byte, width, height, stride int) (Analysis, error) {
	if width <= 0 || height <= 0 {
		return Analysis{}, ErrBadPixels
	}
	if stride <= 0 {
		stride = width * 4
	}
	if stride < width*4 || len(rgba) < stride*height {
		return Analysis{}, ErrBadPixels
	}
	start := time.Now()

	// A 12 MP photo through the bridge is fine, but there's no point doing the
	// maths on every pixel: sample about a million of them at most.
	step := 1
	for (width/step)*(height/step) > maxSamples {
		step++
	}

	var (
		pixels    int64
		sumR      int64
		sumG      int64
		sumB      int64
		sumLuma   int64
		histo     [lumaBuckets]int64
		cells     [lumaGrid * lumaGrid]int64
		cellCount [lumaGrid * lumaGrid]int64
		buckets   = make(map[uint16]*colorBucket, 1024)
	)
	for y := 0; y < height; y += step {
		row := rgba[y*stride:]
		cellY := y * lumaGrid / height
		for x := 0; x < width; x += step {
			p := row[x*4:]
			r, g, b := int64(p[0]), int64(p[1]), int64(p[2])

			// ITU-R BT.601 luma, in integers: what the eye sees, not an average.
			luma := (299*r + 587*g + 114*b) / 1000

			pixels++
			sumR, sumG, sumB = sumR+r, sumG+g, sumB+b
			sumLuma += luma
			histo[luma*lumaBuckets/256]++
			cell := cellY*lumaGrid + x*lumaGrid/width
			cells[cell] += luma
			cellCount[cell]++

			key := uint16(r>>(8-colorBits))<<(2*colorBits) | uint16(g>>(8-colorBits))<<colorBits | uint16(b>>(8-colorBits))
			bk := buckets[key]
			if bk == nil {
				bk = &colorBucket{}
				buckets[key] = bk
			}
			bk.count++
			bk.r, bk.g, bk.b = bk.r+r, bk.g+g, bk.b+b
		}
	}
	if pixels == 0 {
		return Analysis{}, ErrBadPixels
	}

	a := Analysis{
		Width:    width,
		Height:   height,
		MeanLuma: int(sumLuma * 100 / (pixels * 255)), // 0..255 → 0..100
		MeanRGB:  [3]int{int(sumR / pixels), int(sumG / pixels), int(sumB / pixels)},
		Histo:    make([]int, lumaBuckets),
		Grid:     make([]int, lumaGrid*lumaGrid),
	}
	for i, n := range histo {
		a.Histo[i] = int(n * 100 / pixels)
	}
	// Rounding leaves a few percent unaccounted for; give them to the tallest
	// bar, so the bars the UI draws add up to the whole picture.
	rest := 100
	tallest := 0
	for i, p := range a.Histo {
		rest -= p
		if p > a.Histo[tallest] {
			tallest = i
		}
	}
	a.Histo[tallest] += rest

	for i := range a.Grid {
		if cellCount[i] > 0 {
			a.Grid[i] = int(cells[i] * 100 / (cellCount[i] * 255))
		}
	}

	all := make([]*colorBucket, 0, len(buckets))
	for _, bk := range buckets {
		all = append(all, bk)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].count > all[j].count })
	for i := 0; i < len(all) && i < topColors; i++ {
		bk := all[i]
		a.Colors = append(a.Colors, ColorShare{
			Hex:   fmt.Sprintf("#%02x%02x%02x", bk.r/bk.count, bk.g/bk.count, bk.b/bk.count),
			Share: float64(bk.count) / float64(pixels),
		})
	}

	a.Micros = time.Since(start).Microseconds()
	return a, nil
}

type colorBucket struct{ count, r, g, b int64 }
