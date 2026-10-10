package qrcode

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
)

func TestSVG(t *testing.T) {
	b, err := SVG("http://192.168.1.20:8080/m/abcdefghijklmnopqrstuvwxyz")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.HasPrefix(s, `<svg xmlns="http://www.w3.org/2000/svg"`) || !strings.HasSuffix(s, "</svg>") || strings.Count(s, "M") < 50 {
		t.Fatalf("svg: %.200s", s)
	}
}

func TestPNG(t *testing.T) {
	b, err := PNG("https://example.com/m/x", 8)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if w := img.Bounds().Dx(); w < 21*8 {
		t.Fatalf("png width %d", w)
	}
}
