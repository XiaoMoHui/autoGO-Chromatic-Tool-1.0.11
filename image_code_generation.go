package main

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func imageCodeGenerationOutput(functionName, precision, direction, format string) string {
	colors, params, code := buildImagesAPICode(functionName, precision, direction)
	if colors == "" {
		return ""
	}
	switch format {
	case "默认调用":
		return applyFormatTemplate(defaultAPIFormatTemplates()[normalizeImagesFunctionName(functionName)], map[string]string{"[参数]": params})
	case "仅参数":
		return params
	case "参数对象":
		// 参数已按函数签名生成，保留字符串内的逗号及其他内容。
		args, err := splitCodeTestArgs(params)
		if err != nil {
			return ""
		}
		return "{" + strings.Join(args, ",") + "}"
	case "颜色字符串":
		return colors
	default:
		return code
	}
}

func showImageCodeGenerationDialog(parent fyne.Window, functionName, precision, direction string, saveConfig func()) {
	if len(selectedColorPoints()) == 0 {
		dialog.ShowInformation("提示", "请先选取并勾选颜色点", parent)
		return
	}
	codeEntry := widget.NewMultiLineEntry()
	codeEntry.Wrapping = fyne.TextWrapWord
	codeEntry.TextStyle = fyne.TextStyle{Monospace: true}
	codeEntry.SetMinRowsVisible(10)
	if imageCodeTemplates == nil {
		imageCodeTemplates = normalizeImageCodeTemplates(nil, apiFormatTemplates)
	}
	templateSelect := widget.NewSelect(nil, nil)
	formatSelect := widget.NewSelect([]string{"自定义模板", "默认调用", "仅参数", "参数对象", "颜色字符串"}, nil)
	nameEntry := widget.NewEntry()
	nameEntry.SetText("QQ")
	variableEntry := widget.NewEntry()
	variableEntry.SetText("Zhaosevar")
	selectedID := ""
	selectedTemplate := func() *ImageCodeTemplate {
		for i := range imageCodeTemplates {
			if imageCodeTemplates[i].ID == selectedID {
				return &imageCodeTemplates[i]
			}
		}
		return nil
	}
	refresh := func() {
		code := ""
		if template := selectedTemplate(); template != nil {
			if formatSelect.Selected == "自定义模板" {
				code = renderImageCodeTemplate(*template, precision, direction, nameEntry.Text, variableEntry.Text)
			} else {
				code = imageCodeGenerationOutput(template.Function, precision, direction, formatSelect.Selected)
			}
		}
		codeEntry.SetText(code)
		if codeDisplayEntry != nil {
			codeDisplayEntry.SetText(code)
		}
	}
	templateSelect.OnChanged = func(name string) {
		for _, template := range imageCodeTemplates {
			if template.Name == name {
				selectedID = template.ID
				break
			}
		}
		refresh()
	}
	reloadTemplates := func() {
		templateSelect.Options = nil
		for _, template := range imageCodeTemplates {
			templateSelect.Options = append(templateSelect.Options, template.Name)
		}
		if selectedTemplate() == nil {
			selectedID = ""
			if len(imageCodeTemplates) > 0 {
				selectedID = imageCodeTemplates[0].ID
			}
		}
		templateSelect.ClearSelected()
		if template := selectedTemplate(); template != nil {
			templateSelect.SetSelected(template.Name)
		}
		templateSelect.Refresh()
		refresh()
	}
	for _, template := range imageCodeTemplates {
		if template.Function == normalizeImagesFunctionName(functionName) {
			selectedID = template.ID
			break
		}
	}
	formatSelect.OnChanged = func(string) { refresh() }
	nameEntry.OnChanged = func(string) { refresh() }
	variableEntry.OnChanged = func(string) { refresh() }
	formatSelect.SetSelected("自定义模板")
	reloadTemplates()
	editButton := widget.NewButton("格式调整", func() {
		showImageTemplateEditor(parent, selectedID, saveConfig, func(id string) {
			selectedID = id
			reloadTemplates()
		})
	})
	copyButton := widget.NewButton("复制代码", func() {
		parent.Clipboard().SetContent(codeEntry.Text)
	})
	copyButton.Importance = widget.HighImportance
	controls := container.NewVBox(
		container.NewBorder(nil, nil, widget.NewLabel("生成模板"), nil, templateSelect),
		container.NewBorder(nil, nil, widget.NewLabel("输出格式"), editButton, formatSelect),
		container.NewGridWithColumns(2, container.NewBorder(nil, nil, widget.NewLabel("名称"), nil, nameEntry), container.NewBorder(nil, nil, widget.NewLabel("变量名"), nil, variableEntry)),
		widget.NewSeparator(),
	)
	codeDialog := dialog.NewCustom("生成代码", "关闭", container.NewBorder(controls, copyButton, nil, nil, codeEntry), parent)
	codeDialog.Resize(fyne.NewSize(720, 420))
	codeDialog.Show()
}
