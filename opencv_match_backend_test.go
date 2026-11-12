//go:build opencv_cgo || (windows && amd64)

package main

import (
	"image"
	"image/color"
	"testing"
)

func TestOpenCVBackendFindsKnownTemplate(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			source.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 17), G: uint8(y * 17), B: uint8(x * y * 13), A: 255})
		}
	}
	template, err := encodeOpenCVTemplatePNG(cropImage(source, image.Rect(4, 5, 8, 9)))
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []struct {
		name                   string
		gray, transparent, all bool
	}{
		{name: "color"},
		{name: "gray", gray: true},
		{name: "transparent", transparent: true},
		{name: "all", all: true},
	} {
		t.Run(mode.name, func(t *testing.T) {
			result, err := runOpenCVImageMatch(source, template, openCVMatchOptions{
				X2: 15, Y2: 15, Sim: 0.9999,
				IsGray: mode.gray, IsTransparent: mode.transparent, FindAll: mode.all,
			})
			if err != nil {
				t.Fatalf("find known template: %v", err)
			}
			if len(result.Matches) != 1 || result.Matches[0].Point != image.Pt(4, 5) {
				t.Fatalf("expected a match at (4,5), got %+v", result)
			}
		})
	}
}
