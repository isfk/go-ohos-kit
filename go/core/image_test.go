package core

import "testing"

// rgba builds width×height pixels, row stride bytes long, with the padding
// filled by junk — which is what a platform buffer looks like.
func rgba(width, height, stride int, at func(x, y int) [3]uint8) []byte {
	if stride <= 0 {
		stride = width * 4
	}
	buf := make([]byte, stride*height)
	for i := range buf {
		buf[i] = 0xA5 // junk: a padded row must not read as a pixel
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := at(x, y)
			p := buf[y*stride+x*4:]
			p[0], p[1], p[2], p[3] = c[0], c[1], c[2], 0xFF
		}
	}
	return buf
}

func halves(width, height, stride int, left, right [3]uint8) []byte {
	return rgba(width, height, stride, func(x, _ int) [3]uint8 {
		if x < width/2 {
			return left
		}
		return right
	})
}

func TestAnalyzeBlackAndWhiteHalves(t *testing.T) {
	black := [3]uint8{0, 0, 0}
	white := [3]uint8{255, 255, 255}
	// 16 wide, so every one of the 8 grid columns gets samples: a picture
	// smaller than the grid would leave cells with nothing to average.
	a, err := AnalyzePixels(halves(16, 16, 0, black, white), 16, 16, 0)
	if err != nil {
		t.Fatalf("AnalyzePixels: %v", err)
	}
	if a.MeanLuma != 50 {
		t.Errorf("MeanLuma = %d, want 50", a.MeanLuma)
	}
	if a.Histo[0] != 50 || a.Histo[7] != 50 {
		t.Errorf("Histo = %v, want 50%% in the darkest and the brightest bucket", a.Histo)
	}
	if a.MeanRGB != [3]int{127, 127, 127} {
		t.Errorf("MeanRGB = %v, want ~127 each", a.MeanRGB)
	}
	// Left cells dark, right cells bright: the grid is a picture, not an average.
	if a.Grid[0] != 0 || a.Grid[7] != 100 {
		t.Errorf("Grid = %v, want 0 on the left and 100 on the right", a.Grid)
	}
	for i, v := range a.Grid {
		if i%8 < 4 && v != 0 || i%8 >= 4 && v != 100 {
			t.Errorf("Grid[%d] = %d, want the half it sits on", i, v)
		}
	}
}

func TestAnalyzeNamesTheColors(t *testing.T) {
	red := [3]uint8{255, 0, 0}
	blue := [3]uint8{0, 0, 255}
	a, err := AnalyzePixels(halves(8, 4, 0, red, blue), 8, 4, 0)
	if err != nil {
		t.Fatalf("AnalyzePixels: %v", err)
	}
	if len(a.Colors) != 2 {
		t.Fatalf("Colors = %v, want exactly the two the picture has", a.Colors)
	}
	got := map[string]float64{}
	for _, c := range a.Colors {
		got[c.Hex] = c.Share
	}
	for _, want := range []string{"#ff0000", "#0000ff"} {
		if share, ok := got[want]; !ok || share != 0.5 {
			t.Errorf("Colors = %v, want %s at 50%%", a.Colors, want)
		}
	}
}

func TestAnalyzeMergesColoursInTheSameBucket(t *testing.T) {
	// The bucket key keeps the top 4 bits of each channel, so two reds that
	// differ only below that are the same colour to this analysis — and the
	// average the UI shows has to be the average of both, not one of them.
	a, err := AnalyzePixels(halves(8, 4, 0, [3]uint8{255, 0, 0}, [3]uint8{240, 0, 0}), 8, 4, 0)
	if err != nil {
		t.Fatalf("AnalyzePixels: %v", err)
	}
	if len(a.Colors) != 1 {
		t.Fatalf("Colors = %v, want the two reds merged into one bucket", a.Colors)
	}
	if a.Colors[0].Hex != "#f70000" {
		t.Errorf("Hex = %s, want #f70000 (the average of 255 and 240)", a.Colors[0].Hex)
	}
	if a.Colors[0].Share != 1 {
		t.Errorf("Share = %v, want the whole picture", a.Colors[0].Share)
	}
}

func TestAnalyzeKeepsColoursInDifferentBuckets(t *testing.T) {
	// One step coarser than the test above: 224>>4 is 14, not 15, so these are
	// two colours. Guards the key arithmetic against an off-by-one in the
	// shift, which a per-pixel array index would happily hide.
	a, err := AnalyzePixels(halves(8, 4, 0, [3]uint8{255, 0, 0}, [3]uint8{224, 0, 0}), 8, 4, 0)
	if err != nil {
		t.Fatalf("AnalyzePixels: %v", err)
	}
	if len(a.Colors) != 2 {
		t.Fatalf("Colors = %v, want the two buckets these reds fall in", a.Colors)
	}
}

func TestAnalyzeIgnoresRowPadding(t *testing.T) {
	packed := halves(4, 4, 0, [3]uint8{0, 0, 0}, [3]uint8{255, 255, 255})
	padded := halves(4, 4, 4*4+8, [3]uint8{0, 0, 0}, [3]uint8{255, 255, 255})

	a, err := AnalyzePixels(padded, 4, 4, 4*4+8)
	if err != nil {
		t.Fatalf("AnalyzePixels: %v", err)
	}
	b, err := AnalyzePixels(packed, 4, 4, 0)
	if err != nil {
		t.Fatalf("AnalyzePixels: %v", err)
	}
	if a.MeanLuma != b.MeanLuma || a.MeanRGB != b.MeanRGB {
		t.Errorf("padded = %d/%v, packed = %d/%v: the junk between rows leaked in",
			a.MeanLuma, a.MeanRGB, b.MeanLuma, b.MeanRGB)
	}
}

func TestAnalyzeMidGrey(t *testing.T) {
	grey := [3]uint8{128, 128, 128}
	a, err := AnalyzePixels(halves(4, 4, 0, grey, grey), 4, 4, 0)
	if err != nil {
		t.Fatalf("AnalyzePixels: %v", err)
	}
	// 128/255 is 50%: one bucket, all of it.
	if a.MeanLuma != 50 || a.Histo[4] != 100 {
		t.Errorf("MeanLuma = %d, Histo = %v, want 50 and everything in bucket 4", a.MeanLuma, a.Histo)
	}
	if len(a.Colors) != 1 || a.Colors[0].Share != 1 {
		t.Errorf("Colors = %v, want one colour covering the whole picture", a.Colors)
	}
}

func TestAnalyzeSamplingKeepsBigPicturesHonest(t *testing.T) {
	const w, h = 1200, 1000 // more pixels than maxSamples, so it samples
	grey := [3]uint8{51, 51, 51}
	a, err := AnalyzePixels(halves(w, h, 0, grey, grey), w, h, 0)
	if err != nil {
		t.Fatalf("AnalyzePixels: %v", err)
	}
	if a.MeanLuma != 20 {
		t.Errorf("MeanLuma = %d, want 20 (51/255): sampling changed the answer", a.MeanLuma)
	}
	if a.Micros <= 0 {
		t.Errorf("Micros = %d, want the cost of the maths", a.Micros)
	}
}

func TestAnalyzeRejectsBadBuffers(t *testing.T) {
	if _, err := AnalyzePixels(nil, 4, 4, 0); err != ErrBadPixels {
		t.Errorf("nil buffer: err = %v, want ErrBadPixels", err)
	}
	if _, err := AnalyzePixels(make([]byte, 4*4*4), 4, 4, 4*4*2); err != ErrBadPixels {
		t.Errorf("stride < width*4: err = %v, want ErrBadPixels", err)
	}
	if _, err := AnalyzePixels(make([]byte, 16), 0, 4, 0); err != ErrBadPixels {
		t.Errorf("zero width: err = %v, want ErrBadPixels", err)
	}
	if _, err := AnalyzePixels(make([]byte, 63), 4, 4, 0); err != ErrBadPixels {
		t.Errorf("short buffer: err = %v, want ErrBadPixels", err)
	}
}
