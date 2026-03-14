//go:build cgo

package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// makeIcon generates a 22×22 monochrome PNG suitable for a macOS template image.
// It draws a small "daemon" icon: an outer ring with two inner dots (like an agent/face).
func makeIcon() []byte {
	const size = 22
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	black := color.NRGBA{0, 0, 0, 255}

	cx, cy := float64(size-1)/2, float64(size-1)/2

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := float64(x), float64(y)
			dist := math.Sqrt((fx-cx)*(fx-cx) + (fy-cy)*(fy-cy))

			// Outer ring
			if dist >= 8.5 && dist <= 10.5 {
				img.Set(x, y, black)
				continue
			}
			// Two inner dots (like eyes / indicators)
			for _, dot := range []struct{ dx, dy float64 }{{-3, -1}, {3, -1}} {
				if math.Sqrt((fx-(cx+dot.dx))*(fx-(cx+dot.dx))+(fy-(cy+dot.dy))*(fy-(cy+dot.dy))) <= 1.5 {
					img.Set(x, y, black)
				}
			}
			// Bottom arc (smile / sweep indicator)
			if dist >= 4 && dist <= 6 && fy > cy+0.5 {
				img.Set(x, y, black)
			}
		}
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
