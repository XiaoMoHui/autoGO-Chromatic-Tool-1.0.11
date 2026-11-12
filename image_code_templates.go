package main

import (
	"fmt"
	"strconv"
	"strings"
)

// A template has its own identity and display name; multiple templates can use
// the same function's color and coordinate rules.
type ImageCodeTemplate struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Function string `json:"function"`
	Content  string `json:"content"`
}

var imageCodeTemplates []ImageCodeTemplate

var imageTemplateFunctions = []string{"FindMultiColors", "FindColor", "FindMultiColorsAll", "CmpColor"}

func cloneImageCodeTemplates(templates []ImageCodeTemplate) []ImageCodeTemplate {
	result := make([]ImageCodeTemplate, len(templates))
	copy(result, templates)
	return result
}

func normalizeImageCodeTemplates(templates []ImageCodeTemplate, legacy map[string]string) []ImageCodeTemplate {
	if templates != nil {
		return cloneImageCodeTemplates(templates)
	}
	legacy = normalizeAPIFormatTemplates(legacy)
	result := make([]ImageCodeTemplate, 0, 7)
	for _, function := range imageTemplateFunctions {
		result = append(result, ImageCodeTemplate{ID: function, Name: function, Function: function, Content: legacy[function]})
	}
	fields := `Name: [名称], Rect: [4]int{[范围_左], [范围_上], [范围_右], [范围_下]}, Colors: "[颜色值]", Sim: [相似度]`
	return append(result,
		ImageCodeTemplate{ID: "multi_slice", Name: "多点找色切片模板", Function: "FindMultiColors", Content: "{" + fields + "},"},
		ImageCodeTemplate{ID: "multi_map", Name: "多点找色map模板", Function: "FindMultiColors", Content: "[名称]: {" + fields + "},"},
		ImageCodeTemplate{ID: "multi_var", Name: "多点找色变量模板", Function: "FindMultiColors", Content: "var [变量名] = &Colorcfg{" + fields + "}"},
	)
}

func validateImageCodeTemplates(templates []ImageCodeTemplate) error {
	names := make(map[string]bool)
	ids := make(map[string]bool)
	for _, template := range templates {
		if strings.TrimSpace(template.Name) == "" {
			return fmt.Errorf("模板名称不能为空")
		}
		if names[template.Name] {
			return fmt.Errorf("模板名称 %q 重复，请使用不同名称", template.Name)
		}
		if template.ID == "" || ids[template.ID] {
			return fmt.Errorf("模板标识为空或重复")
		}
		if strings.TrimSpace(template.Content) == "" {
			return fmt.Errorf("模板 %q 的内容不能为空", template.Name)
		}
		validFunction := false
		for _, function := range imageTemplateFunctions {
			validFunction = validFunction || function == template.Function
		}
		if !validFunction {
			return fmt.Errorf("模板 %q 的找色函数无效", template.Name)
		}
		names[template.Name], ids[template.ID] = true, true
	}
	return nil
}

func renderImageCodeTemplate(template ImageCodeTemplate, precision, direction, name, variable string) string {
	colors, params, _ := buildImagesAPICode(template.Function, precision, direction)
	if colors == "" {
		return ""
	}
	x1, y1, x2, y2 := regionValuesFromEntry()
	x, y, _ := parsePointPosition(selectedColorPoints()[0].Position)
	sim := strings.TrimSpace(precision)
	if sim == "" {
		sim = "0.9"
	}
	values := apiFormatValues(template.Function, params, imageCodeGenerationOutput(template.Function, precision, direction, "参数对象"), colors, sim, directionValue(direction), x1, y1, x2, y2, x, y)
	// Quote names as Go string literals so quotes, backslashes and newlines remain valid.
	return applyImageCodeTemplate(template.Content, values, name, variable)
}

func applyImageCodeTemplate(content string, values map[string]string, name, variable string) string {
	// Replace only tokens from the template itself, never tokens inside user values.
	pairs := make([]string, 0, 2*len(apiFormatPlaceholders)+4)
	for _, placeholder := range apiFormatPlaceholders {
		pairs = append(pairs, placeholder.token, values[placeholder.token])
	}
	pairs = append(pairs, "[名称]", strconv.Quote(name), "[变量名]", variable)
	return strings.NewReplacer(pairs...).Replace(content)
}
