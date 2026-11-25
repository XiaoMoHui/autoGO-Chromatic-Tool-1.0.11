package main

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
)

func TestFontSplitPreviewKeepsOutlineAtReducedZoom(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	binary := image.NewNRGBA(image.Rect(0, 0, 5, 5))
	foreground := color.NRGBA{G: 255, A: 255}
	fillFontTestRect(binary, image.Rect(0, 0, 3, 3), foreground)
	binary.SetNRGBA(4, 4, foreground)

	dots, cells := renderDotMatrix(binary, 0, 0)
	if len(cells) != 2 {
		t.Fatalf("got %d split cells, want the glyph and the single pixel", len(cells))
	}
	viewer := newFontImageViewer()
	viewer.SetImage(dots)
	viewer.splitCells = cells
	viewer.SetZoom(fontPreviewZoomForSourceZoom(1))

	window := a.NewWindow("")
	defer window.Close()
	window.SetPadded(false)
	window.SetContent(viewer)
	window.Resize(viewer.MinSize())
	preview := window.Canvas().Capture()
	red := func(x, y int) bool {
		r, g, _, _ := preview.At(x, y).RGBA()
		return r > 0xc000 && g < 0x6000
	}
	for _, p := range []image.Point{{1, 0}, {0, 1}, {1, 3}, {3, 1}} {
		if !red(p.X, p.Y) {
			t.Errorf("missing red segment outline at %v", p)
		}
	}
	for y := 4; y < preview.Bounds().Max.Y; y++ {
		for x := 4; x < preview.Bounds().Max.X; x++ {
			if red(x, y) {
				t.Errorf("single foreground pixel has a red outline at (%d,%d)", x, y)
			}
		}
	}
}

func newSolidFontTestImage(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func fillFontTestRect(img *image.NRGBA, rect image.Rectangle, c color.NRGBA) {
	rect = rect.Intersect(img.Bounds())
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
}

func assertColorClose(t *testing.T, got, want color.NRGBA, maxDistanceSq int) {
	t.Helper()
	if fontColorDistanceSq(got, want) > maxDistanceSq {
		t.Fatalf("color mismatch: got %v, want near %v", got, want)
	}
}

func assertRectContains(t *testing.T, outer, inner image.Rectangle) {
	t.Helper()
	if !inner.Min.In(outer) || !image.Pt(inner.Max.X-1, inner.Max.Y-1).In(outer) {
		t.Fatalf("rect %v does not contain %v", outer, inner)
	}
}

func TestParseFontLibIgnoresHeadersAndReadsExportedEntries(t *testing.T) {
	content := "# AutoGo FontLib v1.0\n# Color:FFFFFF-202020\n#\n" +
		"向$c000200018e0ff1f000c000600e38f11c40862043182187f8c0046000380017c$1$0.0.74$15\n" +
		"英$00086010fcff034100800020004000fe3f0441088210042108f2ff1f70003001300638381ec003$1$0.0.91$17\n"
	chars := parseFontLib(content)
	if len(chars) != 2 {
		t.Fatalf("parsed %d font chars, want 2", len(chars))
	}
	if chars[0].Char != "向" || chars[0].Width != 15 || chars[1].Char != "英" || chars[1].Width != 17 {
		t.Fatalf("unexpected parsed font chars: %+v", chars)
	}
}

func TestFontLibFileAllowsEmptyAndSynchronizesOCRAfterSaving(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fontlib.txt")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatal(err)
	}
	chars, err := readFontLibFile(path)
	if err != nil || len(chars) != 0 {
		t.Fatalf("empty dictionary cannot be opened for editing: chars=%v err=%v", chars, err)
	}
	bitmap := [][]bool{{true, true}, {false, true}, {false, true}, {false, true}}
	hex, count := encodeBitmapHex(bitmap)
	chars = []FontChar{{Char: "英", Width: 2, Height: 4, Bitmap: bitmap, HexData: hex, WhitePixels: count}}
	var ocrChars []FontChar
	notifications := 0
	if err := saveFontLibFile(path, chars, func(updated []FontChar) {
		notifications++
		ocrChars = updated
		data, err := os.ReadFile(path)
		if err != nil || string(data) != exportFontLibEntries(updated) {
			t.Errorf("OCR notified before successful write: data=%q err=%v", data, err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	chars[0].Char = "雄"
	if notifications != 1 || len(ocrChars) != 1 || ocrChars[0].Char != "英" {
		t.Fatalf("OCR did not receive independent dictionary snapshot: %+v", ocrChars)
	}
	img := newSolidFontTestImage(2, 4, color.NRGBA{A: 255})
	for y, row := range bitmap {
		for x, dot := range row {
			if dot {
				img.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 255})
			}
		}
	}
	matches, err := recognizeFontImage(img, ocrChars, "ffffff-000000", 0, 0, 1)
	if err != nil || len(matches) != 1 || matches[0].Char != "英" {
		t.Fatalf("new dictionary cannot be used immediately by OCR: matches=%+v err=%v", matches, err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "#") || !strings.HasSuffix(string(data), "\n") {
		t.Fatalf("saved dictionary must contain only character lines: %q", data)
	}
	readBack, err := readFontLibFile(path)
	if err != nil || len(readBack) != 1 || readBack[0].Char != "英" {
		t.Fatalf("saved character could not be reloaded: %+v, %v", readBack, err)
	}
	if err := saveFontLibFile(path, nil, func(updated []FontChar) { ocrChars = updated }); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if len(ocrChars) != 0 || len(data) != 0 {
		t.Fatal("clearing dictionary did not clear both file and OCR")
	}
}

func TestFontLibEntriesWritesSeparateLinesWithoutHeaders(t *testing.T) {
	chars := []FontChar{
		{Char: "英", HexData: "ff", Width: 2, WhitePixels: 8},
		{Char: "雄", HexData: "7f", Width: 2, WhitePixels: 7},
	}
	want := "英$ff$1$0.0.8$2\n雄$7f$1$0.0.7$2\n"
	if got := exportFontLibEntries(chars); got != want {
		t.Fatalf("dictionary contents = %q, want %q", got, want)
	}
}

func TestFontLibFileDoesNotNotifyOCRIfSaveFails(t *testing.T) {
	called := false
	err := saveFontLibFile(filepath.Join(t.TempDir(), "missing", "fontlib.txt"), nil, func([]FontChar) { called = true })
	if err == nil || called {
		t.Fatalf("failed save must not notify OCR: err=%v notified=%v", err, called)
	}
}

func TestFontLibPathHistoryDeduplicatesAndRemovesPaths(t *testing.T) {
	paths := normalizeFontLibPaths([]string{`C:\fonts\a.txt`, `c:\FONTS\A.txt`, "", `D:\fonts\b.txt`})
	if len(paths) != 2 {
		t.Fatalf("normalized paths = %v, want two unique paths", paths)
	}
	paths = addFontLibPath(paths, `E:\fonts\c.txt`)
	if len(paths) != 3 || paths[0] != `E:\fonts\c.txt` {
		t.Fatalf("added path history = %v", paths)
	}
	paths = removeFontLibPath(paths, `d:\FONTS\B.txt`)
	if len(paths) != 2 {
		t.Fatalf("removed path history = %v", paths)
	}
}

func TestFontTextMatchesSortByLineThenXAndFindFirst(t *testing.T) {
	bitmapA := [][]bool{{true, false}, {true, true}}
	bitmapB := [][]bool{{false, true}, {true, false}}
	lib := []FontChar{
		{Char: "向", Bitmap: bitmapA},
		{Char: "英", Bitmap: bitmapB},
	}
	cells := []CharCell{
		{BBox: image.Rect(20, 20, 22, 22), Bitmap: bitmapA},
		{BBox: image.Rect(4, 20, 6, 22), Bitmap: bitmapA},
		{BBox: image.Rect(1, 1, 3, 3), Bitmap: bitmapB},
	}

	matches := matchFontCharCells(cells, lib, 1)
	if len(matches) != 3 {
		t.Fatalf("got %d matches, want 3", len(matches))
	}
	if matches[0].Char != "英" || matches[0].Line != 0 || matches[1].Bounds.Min.X != 4 || matches[2].Bounds.Min.X != 20 {
		t.Fatalf("unexpected reading order: %+v", matches)
	}
	first := findFontTextMatch(matches, "向")
	if len(first) != 1 || first[0].Bounds.Min.X != 4 {
		t.Fatalf("findstr returned %+v, want first 向 at x=4", first)
	}
}

func TestRecognizeFontImagePreservesImageCoordinates(t *testing.T) {
	img := image.NewNRGBA(image.Rect(10, 20, 12, 21))
	img.SetNRGBA(10, 20, color.NRGBA{255, 255, 255, 255})
	img.SetNRGBA(11, 20, color.NRGBA{255, 255, 255, 255})
	lib := []FontChar{{Char: "向", Bitmap: [][]bool{{true, true}}}}

	matches, err := recognizeFontImage(img, lib, "FFFFFF-000000", 0, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Bounds != image.Rect(10, 20, 12, 21) {
		t.Fatalf("got matches %+v, want one match at image coordinates (10,20)-(12,21)", matches)
	}
}

func TestRecognizeFontImageFindsTouchingCharacters(t *testing.T) {
	tBitmap := [][]bool{{true, true, true}, {false, true, false}, {false, true, false}, {false, true, false}}
	aBitmap := [][]bool{{false, true, false}, {true, false, true}, {true, true, true}, {true, false, true}}
	img := newSolidFontTestImage(12, 7, color.NRGBA{A: 255})
	for i, bitmap := range [][][]bool{tBitmap, aBitmap} {
		for y, row := range bitmap {
			for x, foreground := range row {
				if foreground {
					img.SetNRGBA(2+i*3+x, 2+y, color.NRGBA{255, 255, 255, 255})
				}
			}
		}
	}
	// A bridge removed during dictionary creation is still present on screen.
	img.SetNRGBA(4, 3, color.NRGBA{255, 255, 255, 255})
	matches, err := recognizeFontImage(img, []FontChar{{Char: "T", Bitmap: tBitmap}, {Char: "a", Bitmap: aBitmap}}, "ffffff-000000", 1, 1, 0.8)
	if err != nil {
		t.Fatal(err)
	}
	found := findFontTextMatch(matches, "Ta")
	if len(found) != 2 || found[0].Bounds.Min != image.Pt(2, 2) || found[1].Bounds.Min != image.Pt(5, 2) {
		t.Fatalf("touching Ta not recognized at its original coordinates: %+v", matches)
	}
}

func TestScanFontTemplatesRejectsNoiseAndEmptyImage(t *testing.T) {
	bitmap := [][]bool{{true, true, true, true, true}, {false, false, true, false, false}, {false, false, true, false, false}, {false, false, true, false, false}, {false, false, true, false, false}}
	lib := []FontChar{{Char: "T", Bitmap: bitmap}}
	img := newSolidFontTestImage(20, 12, color.NRGBA{A: 255})
	img.SetNRGBA(5, 5, color.NRGBA{G: 230, A: 255})
	if matches := scanFontTemplates(img, lib, 0.8); len(matches) != 0 {
		t.Fatalf("isolated noise matched: %+v", matches)
	}
}

func TestFontBinaryPreviewDoesNotSplitUntilRequested(t *testing.T) {
	img := newSolidFontTestImage(4, 3, color.NRGBA{A: 255})
	img.SetNRGBA(1, 1, color.NRGBA{G: 230, A: 255})
	preview, cells := renderFontBinary(img, nil)
	if len(cells) != 0 || preview.NRGBAAt(dotCellSize+2, dotCellSize+2).G <= 128 {
		t.Fatal("preview must retain green dots without generating character cells")
	}
	_, cells = renderDotMatrix(img, 0, 0)
	if len(cells) != 1 {
		t.Fatalf("explicit split produced %d cells", len(cells))
	}
}

func TestCropFontOCRRegionClampsRangeAndReturnsOrigin(t *testing.T) {
	img := newSolidFontTestImage(8, 6, color.NRGBA{255, 255, 255, 255})
	crop, origin, err := cropFontOCRRegion(img, 2, 1, 6, 5)
	if err != nil {
		t.Fatal(err)
	}
	if crop.Bounds() != image.Rect(0, 0, 4, 4) || origin != image.Pt(2, 1) {
		t.Fatalf("got crop bounds %v and origin %v", crop.Bounds(), origin)
	}
	full, fullOrigin, err := cropFontOCRRegion(img, 0, 0, 0, 0)
	if err != nil || full.Bounds().Dx() != 8 || full.Bounds().Dy() != 6 || fullOrigin != (image.Point{}) {
		t.Fatalf("zero range should use the full image: bounds=%v origin=%v err=%v", full.Bounds(), fullOrigin, err)
	}
}

func TestBestFontCharMatchRejectsSinglePixelNoiseButAcceptsT(t *testing.T) {
	tTemplate := [][]bool{
		{true, true, true, true, true},
		{false, false, true, false, false},
		{false, false, true, false, false},
		{false, false, true, false, false},
		{false, false, true, false, false},
	}
	lib := []FontChar{{Char: "T", Bitmap: tTemplate}}
	if name, _, ok := bestFontCharMatch([][]bool{{true}}, lib, 0.8); ok {
		t.Fatalf("single-pixel noise matched %q", name)
	}
	if name, score, ok := bestFontCharMatch(tTemplate, lib, 0.8); !ok || name != "T" || score < 0.99 {
		t.Fatalf("valid T did not match: name=%q score=%.3f ok=%v", name, score, ok)
	}
}

func TestBuildDotOCRCodeFormatsFindStrAndOCRCalls(t *testing.T) {
	find := buildDotOCRCode("findstr", "商店", "ffffff-101010,ff0000-202020", "0.80", `F:\字库\font"1.txt`, false)
	if !strings.Contains(find, `dotocr.FindStr(0, 0, 0, 0, "商店", "ffffff-101010,ff0000-202020", 0.80, "F:\\字库\\font\"1.txt", 0)`) {
		t.Fatalf("FindStr code is missing expected arguments: %s", find)
	}
	if !strings.Contains(find, "if x != -1 && y != -1") {
		t.Fatalf("FindStr code is missing coordinate guard: %s", find)
	}

	ocr := buildDotOCRCode("ocr", "", "ffffff-101010", "0.90", "", false)
	if !strings.Contains(ocr, `dotocr.Ocr(0, 0, 100, 50, "ffffff-101010", 0.90, false, "字库1", 0)`) {
		t.Fatalf("Ocr code has unexpected format: %s", ocr)
	}
	jsonOCR := buildDotOCRCode("ocr", "", "ffffff-101010", "0.90", "字库1", true)
	if !strings.Contains(jsonOCR, `dotocr.Ocr(0, 0, 100, 50, "ffffff-101010", 0.90, true, "字库1", 0)`) {
		t.Fatalf("JSON Ocr code has unexpected format: %s", jsonOCR)
	}
}

func TestDotOCRColorParamUsesSelectedColorsAndOffsets(t *testing.T) {
	got := dotOCRColorParam([]ColorPoint{
		{Color: "#FFFFFF", Offset: "101010", Selected: true},
		{Color: "#FF0000", Offset: "202020", Selected: false},
		{Color: "00FF00", Offset: "303030", Selected: true},
	})
	if got != "ffffff-101010,00ff00-303030" {
		t.Fatalf("got OCR color param %q", got)
	}
}

func TestFormatFontOCRResultUsesReadableSeparatorsForChineseCoordinates(t *testing.T) {
	got := formatFontOCRResult([]fontTextMatch{
		{Char: "英", Score: 0.98, Bounds: image.Rect(399, 509, 414, 526), Line: 0},
		{Char: "雄", Score: 0.98, Bounds: image.Rect(418, 509, 433, 526), Line: 0},
		{Char: "英", Score: 0.98, Bounds: image.Rect(237, 1057, 252, 1074), Line: 1},
	})
	for _, want := range []string{"识别: 英雄\n英", "英  (399,509)  0.98", "雄  (418,509)  0.98", "英  (237,1057)  0.98"} {
		if !strings.Contains(got, want) {
			t.Errorf("formatted result %q does not contain %q", got, want)
		}
	}
	if strings.Contains(got, "\t") {
		t.Fatalf("formatted result still contains tabs: %q", got)
	}
}

func TestCreateBinaryPreviewFromSelectedColorPointsSupportsMultipleRanges(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	img.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	img.SetNRGBA(1, 0, color.NRGBA{0, 255, 1, 255})
	img.SetNRGBA(2, 0, color.NRGBA{0, 0, 255, 255})
	points := []ColorPoint{
		{Color: "#FF0000", Offset: "000000", Selected: true},
		{Color: "00FF00", Offset: "010101", Selected: true},
		{Color: "0000FF", Offset: "000000", Selected: false},
	}

	got := createBinaryPreviewFromColorPoints(img, points)
	for x, wantForeground := range []bool{true, true, false} {
		foreground := got.NRGBAAt(x, 0).G > 128
		if foreground != wantForeground {
			t.Fatalf("pixel %d foreground = %v, want %v", x, foreground, wantForeground)
		}
	}
}

func TestCreateBinaryPreviewFromColorParamSupportsMultipleOffsets(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{255, 255, 255, 255})
	img.SetNRGBA(1, 0, color.NRGBA{255, 0, 0, 255})
	got := createBinaryPreviewFromColorParam(img, "ffffff-000000,ff0000-000000")
	for x := 0; x < 2; x++ {
		if got.NRGBAAt(x, 0).G <= 128 {
			t.Fatalf("pixel %d was not recognized by its color range", x)
		}
	}
}

func TestFontPreviewColorPointsMergeToolAndManualSamples(t *testing.T) {
	tool := []ColorPoint{{Color: "FF0000", Offset: "101010", Selected: true}}
	manual := []ColorPoint{
		{Color: "00FF00", Offset: "202020", Selected: true},
		{Color: "0000FF", Offset: "303030", Selected: true},
		{Color: "ff0000", Offset: "101010", Selected: true},
	}

	got := fontPreviewColorPoints(tool, manual, nil)
	if len(got) != 3 {
		t.Fatalf("got %d merged color points, want 3 unique points", len(got))
	}
	if got[0].Color != "FF0000" || got[1].Color != "00FF00" || got[2].Color != "0000FF" {
		t.Fatalf("unexpected merged color order: %+v", got)
	}
}

func TestFontPreviewColorPointsUsesAutomaticSampleAsFallback(t *testing.T) {
	automatic := &ColorPoint{Color: "AABBCC", Offset: "010203", Selected: true}
	got := fontPreviewColorPoints(nil, nil, []ColorPoint{*automatic})
	if len(got) != 1 || got[0] != *automatic {
		t.Fatalf("got automatic fallback %+v, want %+v", got, automatic)
	}
}

func TestApplyFontToleranceOverridePreservesSourceOffsetsWhenBlank(t *testing.T) {
	points := []ColorPoint{{Color: "FF0000", Offset: "102030", Selected: true}}
	got, ok := applyFontToleranceOverride(points, "  ")
	if !ok || got[0].Offset != "102030" {
		t.Fatalf("blank override changed source offset: %+v, valid=%v", got, ok)
	}
}

func TestApplyFontToleranceOverrideReplacesAllOffsets(t *testing.T) {
	points := []ColorPoint{
		{Color: "FF0000", Offset: "102030", Selected: true},
		{Color: "00FF00", Offset: "405060", Selected: true},
	}
	got, ok := applyFontToleranceOverride(points, "#0A0B0C")
	if !ok || len(got) != 2 || got[0].Offset != "0A0B0C" || got[1].Offset != "0A0B0C" {
		t.Fatalf("override did not update every color: %+v, valid=%v", got, ok)
	}
}

func TestEstimateFontForegroundColorPointsFindsMultipleColors(t *testing.T) {
	img := newSolidFontTestImage(40, 18, color.NRGBA{245, 245, 245, 255})
	red := color.NRGBA{220, 24, 40, 255}
	blue := color.NRGBA{24, 90, 220, 255}
	fillFontTestRect(img, image.Rect(2, 3, 12, 14), red)
	fillFontTestRect(img, image.Rect(25, 4, 37, 15), blue)

	_, points, ok := estimateFontForegroundColorPoints(img)
	if !ok || len(points) != 2 {
		t.Fatalf("got %d automatic colors, valid=%v; want 2", len(points), ok)
	}
	matched := map[string]bool{}
	for _, point := range points {
		base, tolerance := parseColorWithOffset(colorPointValue(point))
		if isColorMatch(red, base, tolerance) {
			matched["red"] = true
		}
		if isColorMatch(blue, base, tolerance) {
			matched["blue"] = true
		}
	}
	if !matched["red"] || !matched["blue"] {
		t.Fatalf("automatic colors do not cover both foreground colors: %+v", points)
	}
}

func TestEstimateFontColorPointsFromReferencePointsKeepsColorsSeparate(t *testing.T) {
	img := newSolidFontTestImage(36, 20, color.NRGBA{245, 245, 245, 255})
	red := color.NRGBA{220, 24, 40, 255}
	blue := color.NRGBA{24, 90, 220, 255}
	fillFontTestRect(img, image.Rect(2, 3, 13, 15), red)
	fillFontTestRect(img, image.Rect(22, 4, 34, 16), blue)

	_, points, ok := estimateFontColorPointsFromReferencePoints(img, []fontColorReferencePoint{
		{Point: image.Pt(5, 5), Color: red},
		{Point: image.Pt(25, 7), Color: blue},
	})
	if !ok || len(points) != 2 || points[0].Color == points[1].Color {
		t.Fatalf("got reference-derived colors %+v, valid=%v; want two separate colors", points, ok)
	}
}

func TestAutoPreprocessFontImageEstimatesForeground(t *testing.T) {
	cases := []struct {
		name string
		bg   color.NRGBA
		fg   color.NRGBA
	}{
		{name: "light background dark text", bg: color.NRGBA{245, 245, 245, 255}, fg: color.NRGBA{5, 5, 5, 255}},
		{name: "dark background light text", bg: color.NRGBA{12, 12, 12, 255}, fg: color.NRGBA{240, 240, 240, 255}},
		{name: "colored text", bg: color.NRGBA{250, 250, 250, 255}, fg: color.NRGBA{220, 20, 40, 255}},
	}

	textRect := image.Rect(6, 4, 17, 11)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img := newSolidFontTestImage(28, 18, tc.bg)
			fillFontTestRect(img, textRect, tc.fg)

			result, ok := autoPreprocessFontImage(img)
			if !ok {
				t.Fatal("auto preprocess returned false")
			}
			assertColorClose(t, result.Foreground, tc.fg, 20*20)
			assertRectContains(t, result.CropRect, textRect)
			if result.CropRect == img.Bounds() {
				t.Fatalf("expected crop rect smaller than full image, got %v", result.CropRect)
			}
			if result.ForegroundPixels != textRect.Dx()*textRect.Dy() {
				t.Fatalf("foreground pixels = %d, want %d", result.ForegroundPixels, textRect.Dx()*textRect.Dy())
			}
		})
	}
}

func TestAutoPreprocessFontImageRejectsEmptyBackground(t *testing.T) {
	img := newSolidFontTestImage(20, 10, color.NRGBA{255, 255, 255, 255})
	if _, ok := autoPreprocessFontImage(img); ok {
		t.Fatal("expected empty background image to be rejected")
	}
}

func TestAutoPreprocessFontImageClampsBoundaryCrop(t *testing.T) {
	img := newSolidFontTestImage(12, 10, color.NRGBA{255, 255, 255, 255})
	textRect := image.Rect(0, 0, 4, 5)
	fillFontTestRect(img, textRect, color.NRGBA{0, 0, 0, 255})

	result, ok := autoPreprocessFontImage(img)
	if !ok {
		t.Fatal("auto preprocess returned false")
	}
	assertRectContains(t, result.CropRect, textRect)
	if result.CropRect.Min.X != 0 || result.CropRect.Min.Y != 0 {
		t.Fatalf("crop rect should be clamped to image origin, got %v", result.CropRect)
	}
	if !result.CropRect.In(img.Bounds()) {
		t.Fatalf("crop rect %v exceeds bounds %v", result.CropRect, img.Bounds())
	}
}

func TestEstimateFontColorFromReferencePointsCombinesMultipleSamples(t *testing.T) {
	bg := color.NRGBA{245, 245, 245, 255}
	img := newSolidFontTestImage(48, 24, bg)
	fillFontTestRect(img, image.Rect(1, 1, 22, 18), color.NRGBA{0, 0, 0, 255})
	shadeA := color.NRGBA{48, 90, 140, 255}
	shadeB := color.NRGBA{86, 120, 166, 255}
	shadeC := color.NRGBA{62, 105, 152, 255}
	fillFontTestRect(img, image.Rect(28, 4, 34, 10), shadeA)
	fillFontTestRect(img, image.Rect(36, 6, 42, 12), shadeB)
	fillFontTestRect(img, image.Rect(31, 14, 39, 19), shadeC)

	_, automaticForeground, ok := estimateFontForegroundColor(img)
	if !ok {
		t.Fatal("automatic foreground estimation failed")
	}
	if fontColorDistanceSq(automaticForeground, shadeA) <= 20*20 {
		t.Fatalf("test setup invalid: automatic foreground already selected target color %v", automaticForeground)
	}

	_, referencedForeground, tolerance, ok := estimateFontColorFromReferencePoints(img, []fontColorReferencePoint{
		{Point: image.Pt(29, 5), Color: shadeA},
		{Point: image.Pt(37, 7), Color: shadeB},
		{Point: image.Pt(32, 15), Color: shadeC},
	})
	if !ok {
		t.Fatal("reference point color estimation failed")
	}
	for _, sample := range []color.NRGBA{shadeA, shadeB, shadeC} {
		if !isColorMatch(sample, referencedForeground, tolerance) {
			t.Fatalf("sample %v not covered by foreground %v tolerance %v", sample, referencedForeground, tolerance)
		}
	}
	if isColorMatch(bg, referencedForeground, tolerance) {
		t.Fatalf("background %v should not match foreground %v tolerance %v", bg, referencedForeground, tolerance)
	}
}

func TestFindCharacterBBoxesRespondsToGapParams(t *testing.T) {
	img := newSolidFontTestImage(12, 8, color.NRGBA{255, 255, 255, 255})
	fillFontTestRect(img, image.Rect(1, 2, 4, 6), color.NRGBA{0, 0, 0, 255})
	fillFontTestRect(img, image.Rect(6, 2, 9, 6), color.NRGBA{0, 0, 0, 255})

	binary := createBinaryPreview(img, "000000-101010")
	separate := findCharacterBBoxes(binary, 0, 1)
	if len(separate) != 2 {
		t.Fatalf("gap 0 detected %d boxes, want 2", len(separate))
	}
	merged := findCharacterBBoxes(binary, 2, 1)
	if len(merged) != 1 {
		t.Fatalf("gap 2 detected %d boxes, want 1", len(merged))
	}
}

func TestFontLibExportParseRoundTrip(t *testing.T) {
	bitmap := [][]bool{{true, false, true}, {false, true, false}}
	hexData, whitePixels := encodeBitmapHex(bitmap)
	chars := []FontChar{{
		Char:        "字",
		Width:       3,
		Height:      2,
		HexData:     hexData,
		WhitePixels: whitePixels,
		Bitmap:      bitmap,
	}}

	parsed := parseFontLib(exportFontLib(chars, "000000-101010"))
	if len(parsed) != 1 {
		t.Fatalf("parsed %d chars, want 1", len(parsed))
	}
	if parsed[0].Char != chars[0].Char || parsed[0].Width != chars[0].Width || parsed[0].HexData != chars[0].HexData || parsed[0].WhitePixels != chars[0].WhitePixels {
		t.Fatalf("round trip mismatch: got %+v, want %+v", parsed[0], chars[0])
	}
}

func TestFontLibMatchKeyIgnoresWhitespacePadding(t *testing.T) {
	compact := [][]bool{
		{true, false, true},
		{false, true, false},
	}
	padded := [][]bool{
		{false, false, false, false, false},
		{false, true, false, true, false},
		{false, false, true, false, false},
		{false, false, false, false, false},
	}

	compactHex, compactWp := encodeBitmapHex(compact)
	paddedHex, paddedWp := encodeBitmapHex(padded)
	imported := FontChar{
		Char:        "字",
		Width:       len(padded[0]),
		Height:      len(padded),
		HexData:     strings.ToUpper(paddedHex),
		WhitePixels: paddedWp,
		Bitmap:      decodeBitmapHex(strings.ToUpper(paddedHex), len(padded[0]), len(padded)),
	}
	current := FontChar{
		Char:        "字",
		Width:       len(compact[0]),
		Height:      len(compact),
		HexData:     compactHex,
		WhitePixels: compactWp,
		Bitmap:      compact,
	}

	if fontCharMatchKey(imported) != fontCharMatchKey(current) {
		t.Fatalf("match key mismatch: imported %q, current %q", fontCharMatchKey(imported), fontCharMatchKey(current))
	}
}

func TestBitmapSimilarityExactMatchIgnoresWhitespacePadding(t *testing.T) {
	compact := [][]bool{
		{true, false, true},
		{false, true, false},
	}
	padded := [][]bool{
		{false, false, false, false, false},
		{false, true, false, true, false},
		{false, false, true, false, false},
		{false, false, false, false, false},
	}

	got := bitmapSimilarity(compact, padded)
	if got != 1 {
		t.Fatalf("similarity = %v, want 1", got)
	}
}

func TestBitmapSimilarityAllowsSmallDifference(t *testing.T) {
	a := [][]bool{
		{true, false},
		{false, true},
	}
	b := [][]bool{
		{true, false},
		{true, true},
	}

	got := bitmapSimilarity(a, b)
	if got != 0.75 {
		t.Fatalf("similarity = %v, want 0.75", got)
	}
}

func TestBitmapSimilarityResizesTemplate(t *testing.T) {
	cell := [][]bool{
		{true, true, false, false},
		{true, true, false, false},
		{false, false, true, true},
		{false, false, true, true},
	}
	template := [][]bool{
		{true, false},
		{false, true},
	}

	got := bitmapSimilarity(cell, template)
	if got != 1 {
		t.Fatalf("similarity = %v, want 1", got)
	}
}

func TestBestFontCharMatchUsesThreshold(t *testing.T) {
	cell := [][]bool{
		{true, false},
		{false, true},
	}
	near := [][]bool{
		{true, false},
		{true, true},
	}
	nearHex, nearWhite := encodeBitmapHex(near)
	chars := []FontChar{{
		Char:        "近",
		Width:       len(near[0]),
		Height:      len(near),
		HexData:     strings.ToUpper(nearHex),
		WhitePixels: nearWhite,
	}}

	name, score, ok := bestFontCharMatch(cell, chars, 0.7)
	if !ok || name != "近" || score != 0.75 {
		t.Fatalf("match = %q %.2f %v, want 近 0.75 true", name, score, ok)
	}
	if _, _, ok := bestFontCharMatch(cell, chars, 0.8); ok {
		t.Fatal("expected match below threshold to be rejected")
	}
}

func TestFontImageViewerImagePositionRejectsOutsideDisplayedImage(t *testing.T) {
	img := newSolidFontTestImage(4, 3, color.NRGBA{255, 255, 255, 255})
	viewer := newFontImageViewer()
	viewer.SetImage(img)

	point, ok := viewer.imagePosition(fyne.NewPos(2.9, 1.2))
	if !ok || point != image.Pt(2, 1) {
		t.Fatalf("imagePosition inside = %v, %v; want (2,1), true", point, ok)
	}
	if _, ok := viewer.imagePosition(fyne.NewPos(4, 1)); ok {
		t.Fatal("expected x at displayed image edge to be rejected")
	}
	if _, ok := viewer.imagePosition(fyne.NewPos(1, 3)); ok {
		t.Fatal("expected y at displayed image edge to be rejected")
	}
}

func TestFontImageViewerImagePositionUsesRoundedDisplaySize(t *testing.T) {
	img := newSolidFontTestImage(10, 3, color.NRGBA{255, 255, 255, 255})
	viewer := newFontImageViewer()
	viewer.SetImage(img)
	viewer.SetZoom(1.1)

	displayW, displayH := viewer.displayPixelSize()
	if displayW != 11 || displayH != 4 {
		t.Fatalf("display size = %dx%d, want 11x4", displayW, displayH)
	}
	point, ok := viewer.imagePosition(fyne.NewPos(10.9, 3.9))
	if !ok || point != image.Pt(9, 2) {
		t.Fatalf("imagePosition at rounded display edge = %v, %v; want (9,2), true", point, ok)
	}
	if _, ok := viewer.imagePosition(fyne.NewPos(11, 1)); ok {
		t.Fatal("expected x at rounded display edge to be rejected")
	}
}

func TestFontImageViewerUsesCrosshairCursor(t *testing.T) {
	viewer := newFontImageViewer()
	if got := viewer.Cursor(); got != desktop.CrosshairCursor {
		t.Fatalf("cursor = %v, want %v", got, desktop.CrosshairCursor)
	}
}

func TestFontSourceInitialZoomMakesSmallImagesVisible(t *testing.T) {
	zoom := fontSourceInitialZoom(image.Rect(0, 0, 131, 66))
	if zoom <= 1 {
		t.Fatalf("small source zoom = %v, want > 1", zoom)
	}
	if zoom > maxImageZoom {
		t.Fatalf("small source zoom = %v, exceeds max %v", zoom, maxImageZoom)
	}
	if float32(131)*zoom < fontSourceInitialDisplayWidth-0.5 {
		t.Fatalf("scaled width = %v, want at least %d", float32(131)*zoom, fontSourceInitialDisplayWidth)
	}
}

func TestFontSourceInitialZoomKeepsLargeImagesAtOne(t *testing.T) {
	zoom := fontSourceInitialZoom(image.Rect(0, 0, 800, 300))
	if zoom != 1 {
		t.Fatalf("large source zoom = %v, want 1", zoom)
	}
}

func TestFontPreviewZoomMatchesSourcePixelScale(t *testing.T) {
	sourceZoom := float32(2.7)
	got := fontPreviewZoomForSourceZoom(sourceZoom)
	want := sourceZoom / float32(dotCellSize+1)
	if got < want-0.0001 || got > want+0.0001 {
		t.Fatalf("preview zoom = %v, want %v", got, want)
	}
}

func TestFontPreviewZoomClampsToMinimum(t *testing.T) {
	got := fontPreviewZoomForSourceZoom(0.1)
	if got != minImageZoom {
		t.Fatalf("preview zoom = %v, want min %v", got, minImageZoom)
	}
}
