package main

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func showImageTemplateEditor(parent fyne.Window, selectedID string, saveConfig func(), onSaved func(string)) {
	local := cloneImageCodeTemplates(imageCodeTemplates)
	current := -1
	for i := range local {
		if local[i].ID == selectedID {
			current = i
		}
	}
	if current < 0 && len(local) > 0 {
		current = 0
	}
	nameEntry := widget.NewEntry()
	functionSelect := widget.NewSelect(imageTemplateFunctions, nil)
	contentEntry := widget.NewMultiLineEntry()
	contentEntry.Wrapping = fyne.TextWrapWord
	contentEntry.SetMinRowsVisible(8)
	preview := widget.NewLabel("")
	preview.Wrapping = fyne.TextWrapWord
	list := widget.NewSelect(nil, nil)
	loading := false
	refreshPreview := func() {
		if loading || current < 0 {
			return
		}
		local[current].Name = nameEntry.Text
		local[current].Function = functionSelect.Selected
		local[current].Content = contentEntry.Text
		preview.SetText(applyImageCodeTemplate(contentEntry.Text, sampleAPIFormatValues(functionSelect.Selected), "QQ", "Zhaosevar"))
	}
	reloadList := func() {
		list.Options = nil
		for i, template := range local {
			list.Options = append(list.Options, fmt.Sprintf("%d. %s", i+1, template.Name))
		}
		list.Refresh()
	}
	loadCurrent := func() {
		loading = true
		if current < 0 {
			nameEntry.SetText("")
			contentEntry.SetText("")
			nameEntry.Disable()
			contentEntry.Disable()
			functionSelect.Disable()
			preview.SetText("")
			list.ClearSelected()
		} else {
			nameEntry.Enable()
			contentEntry.Enable()
			functionSelect.Enable()
			nameEntry.SetText(local[current].Name)
			functionSelect.SetSelected(local[current].Function)
			contentEntry.SetText(local[current].Content)
			list.SetSelected(list.Options[current])
		}
		loading = false
		refreshPreview()
	}
	list.OnChanged = func(value string) {
		if loading {
			return
		}
		for i, option := range list.Options {
			if option == value {
				current = i
				loadCurrent()
				return
			}
		}
	}
	nameEntry.OnChanged = func(string) {
		if loading {
			return
		}
		refreshPreview()
		loading = true
		reloadList()
		if current >= 0 {
			list.SetSelected(list.Options[current])
		}
		loading = false
	}
	functionSelect.OnChanged = func(string) { refreshPreview() }
	contentEntry.OnChanged = func(string) { refreshPreview() }
	add := widget.NewButtonWithIcon("新增", theme.ContentAddIcon(), func() {
		function, content := "FindMultiColors", defaultAPIFormatTemplates()["FindMultiColors"]
		if current >= 0 {
			function, content = local[current].Function, local[current].Content
		}
		name := "新模板"
		for suffix := 2; ; suffix++ {
			used := false
			for _, template := range local {
				used = used || template.Name == name
			}
			if !used {
				break
			}
			name = fmt.Sprintf("新模板%d", suffix)
		}
		local = append(local, ImageCodeTemplate{ID: fmt.Sprintf("custom_%d", time.Now().UnixNano()), Name: name, Function: function, Content: content})
		current = len(local) - 1
		reloadList()
		loadCurrent()
	})
	remove := widget.NewButtonWithIcon("删除", theme.DeleteIcon(), func() {
		if current < 0 {
			return
		}
		local = append(local[:current], local[current+1:]...)
		if current >= len(local) {
			current = len(local) - 1
		}
		reloadList()
		loadCurrent()
	})
	placeholders := container.NewGridWithColumns(4)
	for _, token := range append([]string{"[名称]", "[变量名]"}, imageTemplateTokens()...) {
		token := token
		placeholders.Add(widget.NewButton(token, func() {
			if current >= 0 {
				insertTextAtEntryCursor(contentEntry, token)
				parent.Canvas().Focus(contentEntry)
			}
		}))
	}
	var editor *dialog.CustomDialog
	save := widget.NewButton("保存配置", func() {
		if err := validateImageCodeTemplates(local); err != nil {
			dialog.ShowError(err, parent)
			return
		}
		imageCodeTemplates = cloneImageCodeTemplates(local)
		if saveConfig != nil {
			saveConfig()
		}
		id := ""
		if current >= 0 {
			id = local[current].ID
		}
		editor.Hide()
		if onSaved != nil {
			onSaved(id)
		}
	})
	save.Importance = widget.HighImportance
	controls := container.NewVBox(
		container.NewBorder(nil, nil, widget.NewLabel("模板"), container.NewHBox(add, remove), list),
		container.NewBorder(nil, nil, widget.NewLabel("模板名称"), nil, nameEntry),
		container.NewBorder(nil, nil, widget.NewLabel("找色函数"), nil, functionSelect),
		widget.NewLabel("新增会复制当前模板；[名称]自带双引号，[变量名]不带引号。"),
		placeholders,
	)
	split := container.NewVSplit(contentEntry, container.NewVScroll(preview))
	split.Offset = 0.65
	close := widget.NewButton("取消", func() { editor.Hide() })
	editor = dialog.NewCustomWithoutButtons("格式调整", container.NewBorder(controls, container.NewHBox(layout.NewSpacer(), close, save), nil, nil, split), parent)
	editor.Resize(fyne.NewSize(820, 650))
	reloadList()
	loadCurrent()
	editor.Show()
}

func imageTemplateTokens() []string {
	tokens := make([]string, 0, len(apiFormatPlaceholders))
	for _, placeholder := range apiFormatPlaceholders {
		tokens = append(tokens, placeholder.token)
	}
	return tokens
}
