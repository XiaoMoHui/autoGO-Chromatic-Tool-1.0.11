package main

import (
	"encoding/json"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	fynetest "fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestImageTemplatesMigrateAndPreserveEdits(t *testing.T) {
	config := normalizeUserConfig(UserConfig{FormatTemplates: map[string]string{"FindMultiColors": "old([参数])"}})
	if len(config.ImageCodeTemplates) != 7 || config.ImageCodeTemplates[0].Content != "old([参数])" {
		t.Fatalf("legacy template not migrated: %#v", config.ImageCodeTemplates)
	}
	config.ImageCodeTemplates = append(config.ImageCodeTemplates[4:], ImageCodeTemplate{ID: "custom", Name: "我的模板", Function: "FindMultiColors", Content: "[颜色值]"})
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	var loaded UserConfig
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	loaded = normalizeUserConfig(loaded)
	if len(loaded.ImageCodeTemplates) != 4 || loaded.ImageCodeTemplates[3].Name != "我的模板" {
		t.Fatalf("edits not retained: %#v", loaded.ImageCodeTemplates)
	}
	loaded.ImageCodeTemplates = []ImageCodeTemplate{}
	if len(normalizeUserConfig(loaded).ImageCodeTemplates) != 0 {
		t.Fatal("deleted templates were restored")
	}
	if err := validateImageCodeTemplates(append(config.ImageCodeTemplates, config.ImageCodeTemplates[0])); err == nil {
		t.Fatal("duplicate template accepted")
	}
}

func TestImageTemplateEditorUpdatesGenerationDropdown(t *testing.T) {
	app := fynetest.NewTempApp(t)
	withColorPointsForTest(t, []ColorPoint{{Position: "10,20", Color: "#ffffff", Selected: true}})
	oldTemplates := imageCodeTemplates
	t.Cleanup(func() { imageCodeTemplates = oldTemplates })
	imageCodeTemplates = normalizeImageCodeTemplates(nil, nil)
	window := app.NewWindow("test")
	window.Resize(fyne.NewSize(1000, 800))
	saved := 0
	showImageCodeGenerationDialog(window, "FindMultiColors", "0.9", "0", func() { saved++ })
	walk := func(visit func(fyne.CanvasObject)) {
		seen := map[fyne.CanvasObject]bool{}
		var walkObject func(fyne.CanvasObject)
		walkObject = func(object fyne.CanvasObject) {
			if object == nil || seen[object] {
				return
			}
			seen[object] = true
			visit(object)
			if c, ok := object.(*fyne.Container); ok {
				for _, child := range c.Objects {
					walkObject(child)
				}
			} else if w, ok := object.(fyne.Widget); ok {
				for _, child := range fynetest.WidgetRenderer(w).Objects() {
					walkObject(child)
				}
			}
		}
		walkObject(window.Canvas().Overlays().Top())
	}
	tap := func(text string) {
		t.Helper()
		var found *widget.Button
		walk(func(o fyne.CanvasObject) {
			if button, ok := o.(*widget.Button); ok && button.Text == text {
				found = button
			}
		})
		if found == nil {
			t.Fatalf("button %q not found", text)
		}
		fynetest.Tap(found)
	}
	tap("格式调整")
	tap("新增")
	walk(func(o fyne.CanvasObject) {
		if entry, ok := o.(*widget.Entry); ok && entry.Text == "新模板" {
			entry.SetText("我的多点模板")
		}
	})
	tap("保存配置")
	if saved != 1 || len(imageCodeTemplates) != 8 {
		t.Fatalf("template not saved: saves=%d, count=%d", saved, len(imageCodeTemplates))
	}
	selected := false
	walk(func(o fyne.CanvasObject) {
		if selectWidget, ok := o.(*widget.Select); ok && selectWidget.Selected == "我的多点模板" {
			selected = true
		}
	})
	if !selected {
		t.Fatal("generation dropdown did not select new template")
	}
	tap("格式调整")
	tap("删除")
	tap("保存配置")
	if saved != 2 || len(imageCodeTemplates) != 7 {
		t.Fatal("template deletion not saved")
	}
	walk(func(o fyne.CanvasObject) {
		if selectWidget, ok := o.(*widget.Select); ok {
			for _, option := range selectWidget.Options {
				if option == "我的多点模板" {
					t.Error("deleted template remains in dropdown")
				}
			}
		}
	})
}

func TestImageTemplatesRenderSamePointsInDifferentFormats(t *testing.T) {
	withColorPointsForTest(t, []ColorPoint{
		{Position: "10,20", Color: "#e91e1e", Selected: true},
		{Position: "7,9", Color: "#fab008", Selected: true},
	})
	templates := normalizeImageCodeTemplates(nil, nil)
	fields := `Name: "QQ", Rect: [4]int{0, 0, 0, 0}, Colors: "e91e1e,-3,-11,fab008", Sim: 0.9`
	wants := []string{"{" + fields + "},", `"QQ": {` + fields + "},", "var Zhaosevar = &Colorcfg{" + fields + "}"}
	for i, template := range templates[4:] {
		if got := renderImageCodeTemplate(template, "0.9", "0", "QQ", "Zhaosevar"); got != wants[i] {
			t.Fatalf("%s: want %s, got %s", template.Name, wants[i], got)
		}
	}
	got := renderImageCodeTemplate(templates[4], "0.9", "0", "[变量名]\"", "example")
	if !strings.Contains(got, `Name: "[变量名]\""`) {
		t.Fatalf("name was not safely rendered: %s", got)
	}
}
