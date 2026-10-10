// Package qrcode renders QR codes as SVG and PNG, offline.
package qrcode

import (
	"fmt"
	"strings"

	"rsc.io/qr"
)

// quiet is the blank margin the QR specification requires, in modules.
const quiet = 4

// SVG renders text as a scalable QR code. Medium error correction survives a
// scuffed table sticker.
func SVG(text string) ([]byte, error) {
	c, err := qr.Encode(text, qr.M)
	if err != nil {
		return nil, err
	}
	n := c.Size + 2*quiet
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges">`, n, n)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#fff"/><path fill="#000" d="`, n, n)
	for y := 0; y < c.Size; y++ {
		for x := 0; x < c.Size; x++ {
			if c.Black(x, y) {
				// Merge runs of dark modules in a row into one rectangle.
				run := 1
				for x+run < c.Size && c.Black(x+run, y) {
					run++
				}
				fmt.Fprintf(&b, "M%d %dh%dv1h-%dz", x+quiet, y+quiet, run, run)
				x += run - 1
			}
		}
	}
	b.WriteString(`"/></svg>`)
	return []byte(b.String()), nil
}

// PNG renders text as a PNG with scale pixels per module.
func PNG(text string, scale int) ([]byte, error) {
	c, err := qr.Encode(text, qr.M)
	if err != nil {
		return nil, err
	}
	c.Scale = scale
	return c.PNG(), nil
}
