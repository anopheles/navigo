// Package cover turns the image a server sends for an album into what the
// terminal shows: decoded, squared and scaled, then drawn as block
// characters that keep its transparency, or as kitty graphics. It also
// keeps the images on disk. It knows nothing of Subsonic or the screen.
package cover

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strings"

	_ "image/jpeg" // registers the decoder
	_ "image/png"  // registers the decoder

	_ "golang.org/x/image/webp" // registers the decoder
)

// Side is the width and height in pixels an Art is scaled down to: enough
// for the largest cover navigo draws.
const Side = 384

// Art is a cover scaled to fit a Side square, with its transparency.
type Art struct {
	img *image.NRGBA
}

// Decode reads a WebP, PNG or JPEG and prepares it. A cover that isn't
// square is centred on a transparent square, so it keeps its shape.
func Decode(b []byte) (*Art, error) {
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("cover: %w", err)
	}
	return &Art{shrink(img, Side)}, nil
}

// Size is the width and height of the Art in pixels.
func (a *Art) Size() int { return a.img.Bounds().Dx() }

// shrink scales img to fit side×side, never up, and centres it on a
// transparent square. It averages in premultiplied colour, so the hidden
// colour of transparent pixels doesn't bleed into the edges of the drawing.
func shrink(img image.Image, side int) *image.NRGBA {
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	long := max(sw, sh)
	n := min(side, long)
	out := image.NewNRGBA(image.Rect(0, 0, n, n))
	dw, dh := max(sw*n/long, 1), max(sh*n/long, 1)
	ox, oy := (n-dw)/2, (n-dh)/2
	src := toNRGBA(img)
	for y := range dh {
		y0, y1 := y*sh/dh, max((y+1)*sh/dh, y*sh/dh+1)
		for x := range dw {
			x0, x1 := x*sw/dw, max((x+1)*sw/dw, x*sw/dw+1)
			var r, g, bl, a, cnt float64
			// Sampling every step-th pixel keeps a 3000 px cover quick.
			sx, sy := max((x1-x0)/4, 1), max((y1-y0)/4, 1)
			for yy := y0; yy < y1; yy += sy {
				for xx := x0; xx < x1; xx += sx {
					c := src.NRGBAAt(xx, yy)
					al := float64(c.A) / 255
					r += float64(c.R) * al
					g += float64(c.G) * al
					bl += float64(c.B) * al
					a += al
					cnt++
				}
			}
			if a > 0 {
				out.SetNRGBA(ox+x, oy+y, color.NRGBA{
					R: uint8(math.Round(r / a)), G: uint8(math.Round(g / a)),
					B: uint8(math.Round(bl / a)), A: uint8(math.Round(a / cnt * 255)),
				})
			}
		}
	}
	return out
}

// toNRGBA is img as straight-alpha pixels at origin 0,0.
func toNRGBA(img image.Image) *image.NRGBA {
	b := img.Bounds()
	if n, ok := img.(*image.NRGBA); ok && b.Min == (image.Point{}) {
		return n
	}
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := range b.Dy() {
		for x := range b.Dx() {
			out.SetNRGBA(x, y, color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA))
		}
	}
	return out
}

// PNG is the Art as a PNG, scaled to side pixels if that is smaller.
func (a *Art) PNG(side int) []byte {
	img := a.img
	if side < a.Size() {
		img = shrink(img, side)
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// quadrants[mask] is the glyph with the top-left (1), top-right (2),
// bottom-left (4) and bottom-right (8) quarters lit.
var quadrants = [16]string{
	" ", "▘", "▝", "▀", "▖", "▌", "▞", "▛", "▗", "▚", "▐", "▜", "▄", "▙", "▟", "█",
}

// Blocks draws the Art as rows lines of cols cells, two by two pixels a
// cell, with the transparency kept: a clear pixel is the terminal's own
// background, so the cover works on a light, translucent or pictured
// terminal. A pixel that is partly clear is blended against bg, the
// terminal's background, if it is known, and counts as drawn from half
// opaque if not. A cell is exactly cols wide.
func (a *Art) Blocks(cols, rows int, bg color.Color) []string {
	small := resample(a.img, cols*2, rows*2)
	out := make([]string, rows)
	for y := range rows {
		var sb strings.Builder
		for x := range cols {
			var c [4][3]uint8
			drawn := 0
			for i := range 4 {
				p := small[(2*y+i/2)*cols*2+2*x+i%2]
				if rgb, ok := over(p, bg); ok {
					c[i], drawn = rgb, drawn|1<<i
				}
			}
			switch drawn {
			case 0:
				sb.WriteByte(' ')
			case 15:
				mask, fg, back := bestSplit(c)
				if mask == 0 {
					fmt.Fprintf(&sb, "\x1b[48;2;%d;%d;%dm \x1b[m", fg[0], fg[1], fg[2])
				} else {
					fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm%s\x1b[m",
						fg[0], fg[1], fg[2], back[0], back[1], back[2], quadrants[mask])
				}
			default:
				fg := average(c, drawn)
				fmt.Fprintf(
					&sb,
					"\x1b[38;2;%d;%d;%dm%s\x1b[m",
					fg[0],
					fg[1],
					fg[2],
					quadrants[drawn],
				)
			}
		}
		out[y] = sb.String()
	}
	return out
}

// pixel is a straight colour with alpha, 0 to 1.
type pixel struct{ r, g, b, a float64 }

// resample area-averages img to w×h in premultiplied colour.
func resample(img *image.NRGBA, w, h int) []pixel {
	sw, sh := img.Bounds().Dx(), img.Bounds().Dy()
	out := make([]pixel, w*h)
	for y := range h {
		y0, y1 := y*sh/h, max((y+1)*sh/h, y*sh/h+1)
		for x := range w {
			x0, x1 := x*sw/w, max((x+1)*sw/w, x*sw/w+1)
			var r, g, b, a, n float64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					c := img.NRGBAAt(xx, yy)
					al := float64(c.A) / 255
					r += float64(c.R) / 255 * al
					g += float64(c.G) / 255 * al
					b += float64(c.B) / 255 * al
					a += al
					n++
				}
			}
			if a > 0 {
				out[y*w+x] = pixel{r / a, g / a, b / a, a / n}
			}
		}
	}
	return out
}

// over is p on bg as a drawn colour, and whether p is drawn at all. With a
// known bg anything but the faintest pixel is blended; without one, a pixel
// is drawn from half opaque, in its own colour.
func over(p pixel, bg color.Color) (rgb [3]uint8, drawn bool) {
	if bg == nil {
		if p.a < 0.5 {
			return rgb, false
		}
		return [3]uint8{u8(p.r), u8(p.g), u8(p.b)}, true
	}
	if p.a < 0.04 {
		return rgb, false
	}
	br, bgg, bb, _ := bg.RGBA()
	mix := func(f float64, back uint32) uint8 {
		return u8(f*p.a + float64(back>>8)/255*(1-p.a))
	}
	return [3]uint8{mix(p.r, br), mix(p.g, bgg), mix(p.b, bb)}, true
}

func u8(f float64) uint8 { return uint8(math.Round(math.Max(0, math.Min(1, f)) * 255)) }

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

// bestSplit chooses which quarters get the foreground colour so that both
// groups are as uniform as can be; mask 0 means one colour fits all four.
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
	for m := 1; m < 8; m++ { // 8 to 14 are the same splits with the groups swapped
		if cost(m) < bestCost-1 {
			best, bestCost = m, cost(m)
		}
	}
	if best == 0 {
		return 0, average(c, 15), [3]uint8{}
	}
	return best, average(c, best), average(c, 15&^best)
}
