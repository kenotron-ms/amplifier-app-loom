//go:build cgo

package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// makeIcon generates a 22×22 monochrome PNG suitable for a macOS template image.
//
// It draws a loom plain-weave mark: 3 vertical warp threads crossed by 2
// horizontal weft threads. The over-under alternation is indicated by 1px
// notch cuts in the weft band at each "weft under warp" crossing — the same
// technique used in vector woven-ribbon SVG icons.
//
// Layout (width): margin(2) | warp(4) | gap(3) | warp(4) | gap(3) | warp(4) | margin(2) = 22px
// Layout (height): margin(4) | weft(4) | gap(6) | weft(4) | margin(4) = 22px
//
// Plain weave over-under pattern:
//
//	weft1: OVER warp1 · UNDER warp2 · OVER warp3
//	weft2: UNDER warp1 · OVER warp2 · UNDER warp3
func makeIcon() []byte {
	const size = 22
	img := image.NewNRGBA(image.Rect(0, 0, size, size))

	black := color.NRGBA{0, 0, 0, 255}
	clear := color.NRGBA{}

	fill := func(x0, y0, x1, y1 int, c color.NRGBA) {
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				img.Set(x, y, c)
			}
		}
	}

	// ── Warp threads (vertical, full height) ─────────────────────────────────
	const (
		w1L, w1R = 2, 5   // warp 1: x=2..5
		w2L, w2R = 9, 12  // warp 2: x=9..12
		w3L, w3R = 16, 19 // warp 3: x=16..19
	)
	fill(w1L, 0, w1R, size-1, black)
	fill(w2L, 0, w2R, size-1, black)
	fill(w3L, 0, w3R, size-1, black)

	// ── Weft threads (horizontal, full width) ─────────────────────────────────
	const (
		wf1T, wf1B = 4, 7   // weft 1: y=4..7
		wf2T, wf2B = 14, 17 // weft 2: y=14..17
	)
	fill(0, wf1T, size-1, wf1B, black)
	fill(0, wf2T, size-1, wf2B, black)

	// ── Over-under notches ────────────────────────────────────────────────────
	// At each "weft UNDER warp" crossing, cut a 1px transparent column on each
	// lateral edge of the weft band — just outside the warp column. This
	// brackets the crossing and signals the weft is passing behind the warp.
	//
	// weft1 UNDER warp2  →  notch at x=8 and x=13, y=wf1T..wf1B
	fill(w2L-1, wf1T, w2L-1, wf1B, clear)
	fill(w2R+1, wf1T, w2R+1, wf1B, clear)

	// weft2 UNDER warp1  →  notch at x=1 and x=6, y=wf2T..wf2B
	fill(w1L-1, wf2T, w1L-1, wf2B, clear)
	fill(w1R+1, wf2T, w1R+1, wf2B, clear)

	// weft2 UNDER warp3  →  notch at x=15 and x=20, y=wf2T..wf2B
	fill(w3L-1, wf2T, w3L-1, wf2B, clear)
	fill(w3R+1, wf2T, w3R+1, wf2B, clear)

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
