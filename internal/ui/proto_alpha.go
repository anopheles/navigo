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

// alphaHalf draws img as w×h cells of ▀ ▄ with transparency kept: a cell
// whose top half is clear and bottom half drawn is ▄ in the bottom colour on
// the terminal's own background, and so on.
func alphaHalf(img image.Image, w, h int, bg color.Color) string {
	pix := shrink(img, w, h*2)
	var sb strings.Builder
	for y := range h {
		for x := range w {
			t, tok := over(pix[(2*y)*w+x], bg)
			b, bok := over(pix[(2*y+1)*w+x], bg)
			switch {
			case tok && bok:
				fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm▀\x1b[m", t[0], t[1], t[2], b[0], b[1], b[2])
			case tok:
				fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm▀\x1b[m", t[0], t[1], t[2])
			case bok:
				fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm▄\x1b[m", b[0], b[1], b[2])
			default:
				sb.WriteByte(' ')
			}
		}
		if y < h-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
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
