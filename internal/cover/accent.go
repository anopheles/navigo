package cover

import (
	"image/color"
	"math"

	"github.com/lucasb-eyer/go-colorful"
)

// Accent is the cover's dominant vivid colour, evened out to a mid-tone that
// reads on light and dark terminals. It reports false for a cover with no
// colour to speak of, such as a grey or a mostly transparent one.
//
// Pixels vote for their hue (twelve bins of 30°) with their vividness:
// saturation squared, discounted towards black and white, so a small bright
// logo beats a large dull background.
func (a *Art) Accent() (color.Color, bool) {
	const bins = 12
	type bin struct{ w, ws, r, g, b float64 }
	var bs [bins]bin
	for _, p := range resample(a.img, 64, 64) {
		if p.a < 0.5 {
			continue
		}
		h, s, l := colorful.Color{R: p.r, G: p.g, B: p.b}.Hsl()
		w := s * s * (1 - math.Abs(2*l-1))
		i := int(h/30) % bins
		bs[i].w += w
		bs[i].ws += s * w
		bs[i].r += p.r * w
		bs[i].g += p.g * w
		bs[i].b += p.b * w
	}
	best := 0
	for i := range bs {
		if bs[i].w > bs[best].w {
			best = i
		}
	}
	b := bs[best]
	if b.w < 2 || b.ws/b.w < 0.25 { // grey, or too little colour to tell
		return nil, false
	}
	h, s, l := colorful.Color{R: b.r / b.w, G: b.g / b.w, B: b.b / b.w}.Hsl()
	return colorful.Hsl(h, math.Max(0.5, math.Min(0.9, s)), math.Max(0.52, math.Min(0.66, l))), true
}
