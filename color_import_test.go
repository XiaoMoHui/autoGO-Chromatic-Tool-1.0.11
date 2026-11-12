package main

import (
	"testing"

	"fyne.io/fyne/v2"
	fynetest "fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestParseColorImportFormats(t *testing.T) {
	fields := `Name: "QQ", Rect: [4]int{0, 0, 0, 0}, Colors: "e91e1e,-3,-11,fab008,4,-11,fab008", Sim: 0.9`
	tests := []struct {
		name, text, function, colors string
		count                        int
	}{
		{"pure", "fd4158-303030|fff8dc-303030|fffadb-303030", "FindColor", "fd4158-303030|fff8dc-303030|fffadb-303030", 3},
		{"official", `x, y := images.FindMultiColors(303, 813, 341, 851, "fd4158-303030,-6,-6,fff8dc-303030,0,-6,fffadb-303030", 0.9, 2, 0)`, "FindMultiColors", "fd4158-303030,-6,-6,fff8dc-303030,0,-6,fffadb-303030", 3},
		{"slice", "{" + fields + "},", "FindMultiColors", "e91e1e,-3,-11,fab008,4,-11,fab008", 3},
		{"map", `"Tap": {` + fields + "},", "FindMultiColors", "e91e1e,-3,-11,fab008,4,-11,fab008", 3},
		{"var", "var Zhaosevar = &Colorcfg{" + fields + "}", "FindMultiColors", "e91e1e,-3,-11,fab008,4,-11,fab008", 3},
		{"compact", `{121,809,151,834,"fefeef-303030|fefbe9-303030",0.9,0,0}`, "FindColor", "fefeef-303030|fefbe9-303030", 2},
		{"all", `points := images.FindMultiColorsAll(0,0,0,0,"ffffff,2,3,112233",1.0,3,0)`, "FindMultiColorsAll", "ffffff,2,3,112233", 2},
		{"cmp", `matched := images.CmpColor(100,200,"ffffff|112233",0.9,0)`, "CmpColor", "ffffff|112233", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := parseColorImport(tt.text)
			if err != nil {
				t.Fatal(err)
			}
			if parsed.Function != tt.function || parsed.Colors != tt.colors {
				t.Fatalf("unexpected parse: %#v", parsed)
			}
			points, err := parsed.points(320, 830)
			if err != nil || len(points) != tt.count {
				t.Fatalf("points: %v, %v", points, err)
			}
			if tt.name == "official" {
				if *parsed.Rect != [4]int{303, 813, 341, 851} || parsed.Similarity != "0.9" || *parsed.Direction != 2 || *parsed.DisplayID != 0 || points[1].Position != "314, 824" || points[1].Offset != "303030" {
					t.Fatalf("official fields/offsets lost: %#v %v", parsed, points)
				}
			}
			if tt.name == "pure" && (parsed.Rect != nil || parsed.Similarity != "" || parsed.Direction != nil) {
				t.Fatal("pure colors unexpectedly supplied panel settings")
			}
			if tt.name == "cmp" && points[0].Position != "100, 200" {
				t.Fatal("CmpColor anchor lost")
			}
			if tt.name == "var" && points[1].Offset != "" {
				t.Fatal("missing offset was replaced by default tolerance")
			}
		})
	}
}

func TestParseColorImportRejectsInvalidData(t *testing.T) {
	for _, text := range []string{
		"", "xyz123", "ffffff-zzzzzz", "ffffff,1,2", "ffffff,x,2,112233", "ffffff||112233",
		`images.FindColor(0,0,0,0,"ffffff",1.2,0,0)`,
		`images.FindColor(0,0,0,0,"ffffff",0.9,4,0)`,
		`images.FindColor(20,0,10,0,"ffffff",0.9,0,0)`,
		`images.FindColor(0,0,0,0,"ffffff",getSim(),0,0)`,
		`{Colors:"ffffff", Rect:[4]int{0,0,0,0}, Sim: 0.01}`,
		`[]Colorcfg{{Colors:"ffffff"},{Colors:"112233"}}`,
	} {
		if parsed, err := parseColorImport(text); err == nil {
			t.Errorf("accepted invalid input %q: %#v", text, parsed)
		}
	}
}

func TestColorImportTemplateRoundTrip(t *testing.T) {
	withColorPointsForTest(t, []ColorPoint{
		{Position: "323, 834", Color: "#fd4158", Offset: "303030", Selected: true},
		{Position: "317, 828", Color: "#fff8dc", Offset: "303030", Selected: true},
	})
	for _, template := range normalizeImageCodeTemplates(nil, nil) {
		if template.Function != "FindMultiColors" {
			continue
		}
		text := renderImageCodeTemplate(template, "0.9", "3", "QQ", "Example")
		parsed, err := parseColorImport(text)
		if err != nil {
			t.Fatalf("%s: %v", template.Name, err)
		}
		points, err := parsed.points(323, 834)
		if err != nil || len(points) != 2 || points[1].Position != "317, 828" || points[1].Offset != "303030" {
			t.Fatalf("%s did not round trip: %v, %v", template.Name, points, err)
		}
	}
}

func TestColorImportDialogPreviewsAndApplies(t *testing.T) {
	app := fynetest.NewTempApp(t)
	withColorPointsForTest(t, nil)
	window := app.NewWindow("import test")
	window.Resize(fyne.NewSize(1000, 800))
	var applied []ColorPoint
	showColorImportDialog(window, func(parsed parsedColorImport, points []ColorPoint) {
		applied = points
		if parsed.Function != "FindMultiColors" {
			t.Error("incorrect function applied")
		}
	})
	seen := map[fyne.CanvasObject]bool{}
	var input *widget.Entry
	var preview, apply *widget.Button
	var walk func(fyne.CanvasObject)
	walk = func(o fyne.CanvasObject) {
		if o == nil || seen[o] {
			return
		}
		seen[o] = true
		switch object := o.(type) {
		case *widget.Entry:
			if object.MultiLine {
				input = object
			}
		case *widget.Button:
			if object.Text == "解析预览" {
				preview = object
			}
			if object.Text == "应用到面板" {
				apply = object
			}
		}
		if c, ok := o.(*fyne.Container); ok {
			for _, child := range c.Objects {
				walk(child)
			}
		} else if w, ok := o.(fyne.Widget); ok {
			for _, child := range fynetest.WidgetRenderer(w).Objects() {
				walk(child)
			}
		}
	}
	walk(window.Canvas().Overlays().Top())
	if input == nil || preview == nil || apply == nil {
		t.Fatal("import controls missing")
	}
	input.SetText("bad color")
	fynetest.Tap(apply)
	if applied != nil {
		t.Fatal("invalid input applied")
	}
	input.SetText(`{Name:"QQ", Rect:[4]int{0,0,0,0}, Colors:"ffffff,-2,-6,3399ff", Sim:0.9}`)
	fynetest.Tap(preview)
	if applied != nil {
		t.Fatal("preview changed panel")
	}
	fynetest.Tap(apply)
	if len(applied) != 2 || applied[1].Position != "-2, -6" {
		t.Fatalf("incorrect application: %v", applied)
	}
}
