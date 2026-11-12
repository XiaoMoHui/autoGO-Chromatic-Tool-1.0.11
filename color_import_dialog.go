package main

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

func showColorImportDialog(parent fyne.Window, apply func(parsedColorImport, []ColorPoint)) {
	input := widget.NewMultiLineEntry()
	input.SetMinRowsVisible(7)
	input.Wrapping = fyne.TextWrapWord
	input.SetPlaceHolder("粘贴颜色字符串、images.FindColor / FindMultiColors / CmpColor 调用，或包含 Rect、Colors、Sim 的配置")
	x, y := 0, 0
	if points := selectedColorPoints(); len(points) > 0 {
		x, y, _ = parsePointPosition(points[0].Position)
	}
	xEntry, yEntry := widget.NewEntry(), widget.NewEntry()
	xEntry.SetText(strconv.Itoa(x))
	yEntry.SetText(strconv.Itoa(y))
	preview := widget.NewLabel("点击“解析预览”查看结果。")
	preview.Wrapping = fyne.TextWrapWord
	parse := func() (parsedColorImport, []ColorPoint, error) {
		parsed, err := parseColorImport(input.Text)
		if err != nil {
			return parsed, nil, err
		}
		baseX, ex := strconv.Atoi(strings.TrimSpace(xEntry.Text))
		baseY, ey := strconv.Atoi(strings.TrimSpace(yEntry.Text))
		if parsed.Anchor == nil && (ex != nil || ey != nil) {
			return parsed, nil, fmt.Errorf("基准 X/Y 请输入整数")
		}
		points, err := parsed.points(baseX, baseY)
		if err != nil {
			return parsed, nil, err
		}
		region, sim, direction, display := "未提供，保留面板原值", "未提供，保留面板原值", "未提供，保留面板原值", "未提供"
		if parsed.Rect != nil {
			r := *parsed.Rect
			region = fmt.Sprintf("%d,%d,%d,%d（右/下为 0 表示屏幕最大值）", r[0], r[1], r[2], r[3])
		}
		if parsed.Similarity != "" {
			sim = parsed.Similarity
		}
		if parsed.Direction != nil {
			direction = strconv.Itoa(*parsed.Direction)
		}
		if parsed.DisplayID != nil {
			display = strconv.Itoa(*parsed.DisplayID)
			if *parsed.DisplayID != 0 {
				display += "（当前图色面板仅支持屏幕 0，不能直接应用）"
			}
		}
		detail := fmt.Sprintf("函数：%s\n范围：%s\n颜色：%s\n相似度：%s\n查找方向：%s\n屏幕 ID：%s\n颜色点数：%d\n", parsed.Function, region, parsed.Colors, sim, direction, display, len(points))
		for _, point := range points {
			detail += fmt.Sprintf("\n%s  %s", point.Position, colorPointValue(point))
		}
		preview.SetText(detail)
		return parsed, points, nil
	}
	previewButton := widget.NewButton("解析预览", func() {
		if _, _, err := parse(); err != nil {
			preview.SetText("解析失败：" + err.Error())
		}
	})
	var window *dialog.CustomDialog
	applyButton := widget.NewButton("应用到面板", func() {
		parsed, points, err := parse()
		if err != nil {
			preview.SetText("解析失败：" + err.Error())
			return
		}
		if parsed.DisplayID != nil && *parsed.DisplayID != 0 {
			dialog.ShowInformation("提示", "当前图色面板仅支持屏幕 0，请使用对应屏幕的图片和配置。", parent)
			return
		}
		apply(parsed, points)
		window.Hide()
	})
	applyButton.Importance = widget.HighImportance
	note := widget.NewLabel("应用将替换当前颜色点。多点串仅含相对坐标，请设置第一个点的基准位置；纯颜色列表没有坐标，各颜色共用基准位置。")
	note.Wrapping = fyne.TextWrapWord
	invalidate := func(string) { preview.SetText("输入已更新，请重新解析预览。") }
	input.OnChanged, xEntry.OnChanged, yEntry.OnChanged = invalidate, invalidate, invalidate
	base := container.NewGridWithColumns(2,
		container.NewBorder(nil, nil, widget.NewLabel("基准 X"), nil, xEntry),
		container.NewBorder(nil, nil, widget.NewLabel("基准 Y"), nil, yEntry))
	top := container.NewVBox(input, base, note)
	closeButton := widget.NewButton("取消", func() { window.Hide() })
	buttons := container.NewHBox(layout.NewSpacer(), closeButton, previewButton, applyButton)
	window = dialog.NewCustomWithoutButtons("颜色解析", container.NewBorder(top, buttons, nil, nil, container.NewVScroll(preview)), parent)
	window.Resize(fyne.NewSize(780, 650))
	window.Show()
}
