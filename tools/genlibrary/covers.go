package main

import (
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"

	"github.com/lucasb-eyer/go-colorful"

	"github.com/anopheles/navigo/tools/internal/fixturelib"
)

// coverSide is the width and height of the synthetic covers.
const coverSide = 600

// writeCover paints the album's cover to path: a sun over banded water in
// colours taken from a hash of the album, with rounded corners cut out
// through alpha. It is made up, like everything in the fixture library, and
// is deterministic.
func writeCover(path string, a fixturelib.Album) error {
	h := fnv.New32a()
	_, _ = h.Write([]byte(a.AlbumArtist + "\x00" + a.Title))
	hue := float64(h.Sum32() % 360)
	sky0 := colorful.Hsl(hue, 0.7, 0.18)
	sky1 := colorful.Hsl(math.Mod(hue+40, 360), 0.8, 0.55)
	sun := colorful.Hsl(math.Mod(hue+70, 360), 0.9, 0.7)
	sea := colorful.Hsl(math.Mod(hue+200, 360), 0.6, 0.25)

	img := image.NewNRGBA(image.Rect(0, 0, coverSide, coverSide))
	horizon := float64(coverSide) * 0.62
	cx, cy, r := float64(coverSide)*0.5, horizon-float64(coverSide)*0.06, float64(coverSide)*0.2
	const corner = 0.12 * coverSide
	for y := range coverSide {
		for x := range coverSide {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			var c colorful.Color
			if fy < horizon {
				c = sky0.BlendHcl(sky1, fy/horizon)
				if d := math.Hypot(fx-cx, fy-cy); d < r {
					c = sky1.BlendHcl(sun, 0.4+0.6*(1-d/r))
				}
			} else {
				depth := (fy - horizon) / (coverSide - horizon)
				c = sea.BlendHcl(sky0, depth)
				// bands of light under the sun
				if math.Abs(fx-cx) < r*(1-depth)*1.2 &&
					math.Mod(fy-horizon, 14+depth*20) < 4+depth*6 {
					c = c.BlendHcl(sun, 0.7-0.5*depth)
				}
			}
			dx := math.Max(corner-fx, fx-(coverSide-corner))
			dy := math.Max(corner-fy, fy-(coverSide-corner))
			alpha := 1.0
			if dx > 0 && dy > 0 {
				alpha = math.Max(0, math.Min(1, 0.5-(math.Hypot(dx, dy)-corner)))
			}
			rr, gg, bb := c.Clamped().RGB255()
			img.SetNRGBA(x, y, color.NRGBA{rr, gg, bb, uint8(alpha * 255)})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
