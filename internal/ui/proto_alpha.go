// PROTOTYPE: throwaway (anopheles/navigo#60). Never merge into main.

package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
)

// px is a premultiplied-free pixel: straight colour plus alpha, all 0..1.
type px struct{ r, g, b, a float64 }

// shrink area-averages img to w×h in premultiplied space, so edges between
// transparent and opaque pixels don't pick up the hidden colour of the
// transparent ones.
func shrink(img image.Image, w, h int) []px {
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	out := make([]px, w*h)
	for y := range h {
		y0, y1 := y*sh/h, max((y+1)*sh/h, y*sh/h+1)
		for x := range w {
			x0, x1 := x*sw/w, max((x+1)*sw/w, x*sw/w+1)
			sx, sy := max((x1-x0)/6, 1), max((y1-y0)/6, 1)
			var r, g, bl, a, n float64
			for yy := y0; yy < y1; yy += sy {
				for xx := x0; xx < x1; xx += sx {
					c := color.NRGBAModel.Convert(img.At(b.Min.X+xx, b.Min.Y+yy)).(color.NRGBA)
					al := float64(c.A) / 255
					r += float64(c.R) / 255 * al
					g += float64(c.G) / 255 * al
					bl += float64(c.B) / 255 * al
					a += al
					n++
				}
			}
			if a > 0 {
				out[y*w+x] = px{r / a, g / a, bl / a, a / n}
			}
		}
	}
	return out
}

// over puts p on the background colour bg (if known) and reports whether
// the result is a drawn pixel at all. Without a known background a pixel
// is drawn when it is at least half opaque; with one, anything visible is
// blended, so soft edges look right on that background. A pixel that is
// almost fully transparent is never drawn, so the terminal's own background
// (and its transparency or image) shows through.
func over(p px, bg color.Color) (c [3]uint8, drawn bool) {
	if bg == nil {
		if p.a < 0.5 {
			return c, false
		}
		return [3]uint8{u8(p.r), u8(p.g), u8(p.b)}, true
	}
	if p.a < 0.04 {
		return c, false
	}
	br, bgg, bb, _ := bg.RGBA()
	mix := func(f float64, back uint32) uint8 {
		return u8(f*p.a + float64(back>>8)/255*(1-p.a))
	}
	return [3]uint8{mix(p.r, br), mix(p.g, bgg), mix(p.b, bb)}, true
}

func u8(f float64) uint8 { return uint8(math.Round(math.Max(0, math.Min(1, f)) * 255)) }

// quadrants[mask] is the glyph with the TL(1) TR(2) BL(4) BR(8) quarters lit.
var quadrants = []string{" ", "▘", "▝", "▀", "▖", "▌", "▞", "▛", "▗", "▚", "▐", "▜", "▄", "▙", "▟", "█"}

// alphaHalf draws img as w×h cells of quadrant blocks (2×2 pixels a cell)
// with transparency kept. Pixels that are clear stay the terminal's own
// background: a cell with some clear pixels draws only the others, in their
// average colour. A cell with none is split into the two colours that fit
// its four pixels best.
func alphaHalf(img image.Image, w, h int, bg color.Color) string {
	pix := shrink(img, w*2, h*2)
	var sb strings.Builder
	for y := range h {
		for x := range w {
			var c [4][3]uint8
			var ok [4]bool
			drawn := 0
			for i := range 4 {
				c[i], ok[i] = over(pix[(2*y+i/2)*w*2+2*x+i%2], bg)
				if ok[i] {
					drawn |= 1 << i
				}
			}
			switch drawn {
			case 0:
				sb.WriteByte(' ')
			case 15:
				mask, fg, bgc := bestSplit(c)
				if mask == 0 {
					fmt.Fprintf(&sb, "\x1b[48;2;%d;%d;%dm \x1b[m", fg[0], fg[1], fg[2])
				} else {
					fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm%s\x1b[m",
						fg[0], fg[1], fg[2], bgc[0], bgc[1], bgc[2], quadrants[mask])
				}
			default:
				fg := average(c, drawn)
				fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm%s\x1b[m", fg[0], fg[1], fg[2], quadrants[drawn])
			}
		}
		if y < h-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func average(c [4][3]uint8, mask int) [3]uint8 {
	var sum [3]int
	n := 0
	for i := range 4 {
		if mask&(1<<i) != 0 {
			for k := range 3 {
				sum[k] += int(c[i][k])
			}
			n++
		}
	}
	if n == 0 {
		return [3]uint8{}
	}
	return [3]uint8{uint8(sum[0] / n), uint8(sum[1] / n), uint8(sum[2] / n)}
}

// bestSplit picks which quarters get the foreground colour so that both
// groups are as uniform as possible; mask 0 means one colour fits all.
func bestSplit(c [4][3]uint8) (mask int, fg, bg [3]uint8) {
	cost := func(m int) float64 {
		var t float64
		for _, g := range []int{m, 15 &^ m} {
			a := average(c, g)
			for i := range 4 {
				if g&(1<<i) != 0 {
					for k := range 3 {
						d := float64(c[i][k]) - float64(a[k])
						t += d * d
					}
				}
			}
		}
		return t
	}
	best, bestCost := 0, cost(0)
	for m := 1; m < 8; m++ { // 8..15 are the same splits swapped
		if cost(m) < bestCost-1 {
			best, bestCost = m, cost(m)
		}
	}
	if best == 0 {
		return 0, average(c, 15), [3]uint8{}
	}
	return best, average(c, best), average(c, 15&^best)
}

// roundCorners returns img with rounded corners of radius r (a fraction of
// the side), anti-aliased through alpha: a stand-in for a cover with real
// transparency.
func roundCorners(img image.Image, r float64) image.Image {
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	rad := r * float64(min(b.Dx(), b.Dy()))
	for y := range b.Dy() {
		for x := range b.Dx() {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			dx := math.Max(rad-float64(x)-0.5, float64(x)+0.5-(float64(b.Dx())-rad))
			dy := math.Max(rad-float64(y)-0.5, float64(y)+0.5-(float64(b.Dy())-rad))
			if dx > 0 && dy > 0 {
				d := math.Hypot(dx, dy) - rad
				c.A = uint8(float64(c.A) * math.Max(0, math.Min(1, 0.5-d)))
			}
			out.SetNRGBA(x, y, c)
		}
	}
	return out
}
