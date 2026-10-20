// PROTOTYPE: throwaway (accent from the cover). Never merge into main.

package cover

import (
	"image/color"
	"math"
	"sort"

	"github.com/lucasb-eyer/go-colorful"
)

// Palette is the cover's one or two dominant vivid colours, evened out to
// mid-tones that read on light and dark terminals; nil for a grey cover.
func (a *Art) Palette() []color.Color {
	const bins = 12
	type bin struct {
		w, r, g, b float64
		hue        float64
	}
	var bs [bins]bin
	for _, p := range resample(a.img, 64, 64) {
		if p.a < 0.5 {
			continue
		}
		c := colorful.Color{R: p.r, G: p.g, B: p.b}
		h, s, l := c.Hsl()
		w := s * (1 - math.Abs(2*l-1)) // vivid and neither black nor white
		i := int(h/30) % bins
		bs[i].w += w
		bs[i].r += p.r * w
		bs[i].g += p.g * w
		bs[i].b += p.b * w
		bs[i].hue += h * w
	}
	idx := make([]int, bins)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(x, y int) bool { return bs[idx[x]].w > bs[idx[y]].w })
	first := bs[idx[0]]
	if first.w < 4 { // a grey cover
		return nil
	}
	tone := func(b bin) color.Color {
		c := colorful.Color{R: b.r / b.w, G: b.g / b.w, B: b.b / b.w}
		h, s, l := c.Hsl()
		return colorful.Hsl(h, math.Max(0.5, math.Min(0.9, s)), math.Max(0.52, math.Min(0.66, l)))
	}
	out := []color.Color{tone(first)}
	h0 := first.hue / first.w
	for _, i := range idx[1:] {
		b := bs[i]
		if b.w < first.w*0.25 {
			break
		}
		d := math.Abs(b.hue/b.w - h0)
		if math.Min(d, 360-d) >= 40 {
			return append(out, tone(b))
		}
	}
	return out
}
