package main

import (
	"testing"

	"fyne.io/fyne/v2/widget"
)

func TestImageCodeGenerationFormats(t *testing.T) {
	withColorPointsForTest(t, []ColorPoint{
		{Position: "125, 814", Color: "#fefeef", Offset: "303030", Selected: true},
		{Position: "135, 819", Color: "#fefbe9", Offset: "303030", Selected: true},
	})
	rectCoordEntry = widget.NewEntry()
	rectCoordEntry.SetText("121,809,151,834")
	apiFormatTemplates["FindMultiColors"] = "custom([颜色参数])"
	tests := []struct {
		function string
		format   string
		want     string
	}{
		{"FindColor", "颜色字符串", "fefeef-303030|fefbe9-303030"},
		{"FindMultiColors", "颜色字符串", "fefeef-303030,10,5,fefbe9-303030"},
		{"FindMultiColorsAll", "颜色字符串", "fefeef-303030,10,5,fefbe9-303030"},
		{"CmpColor", "颜色字符串", "fefeef-303030|fefbe9-303030"},
		{"FindColor", "参数对象", `{121,809,151,834,"fefeef-303030|fefbe9-303030",0.9,0,0}`},
		{"FindMultiColors", "参数对象", `{121,809,151,834,"fefeef-303030,10,5,fefbe9-303030",0.9,0,0}`},
		{"CmpColor", "参数对象", `{125,814,"fefeef-303030|fefbe9-303030",0.9,0}`},
		{"FindMultiColors", "自定义模板", `custom({121,809,151,834,"fefeef-303030,10,5,fefbe9-303030",0.9,0,0})`},
		{"FindMultiColors", "默认调用", `x, y := images.FindMultiColors(121, 809, 151, 834, "fefeef-303030,10,5,fefbe9-303030", 0.9, 0, 0)`},
		{"FindMultiColors", "仅参数", `121, 809, 151, 834, "fefeef-303030,10,5,fefbe9-303030", 0.9, 0, 0`},
	}
	for _, tt := range tests {
		t.Run(tt.function+"/"+tt.format, func(t *testing.T) {
			if got := imageCodeGenerationOutput(tt.function, "0.9", "0", tt.format); got != tt.want {
				t.Fatalf("want %q, got %q", tt.want, got)
			}
		})
	}
}
