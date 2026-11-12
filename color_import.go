package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

type parsedColorImport struct {
	Function, Colors, Similarity string
	Rect                         *[4]int
	Direction, DisplayID         *int
	Anchor                       *[2]int
}

func parseColorImport(text string) (parsedColorImport, error) {
	text = strings.TrimSpace(text)
	result := parsedColorImport{}
	// Parse Go syntax as data only: no pasted code is ever executed.
	candidates := []string{
		"package p\nfunc _(){\n" + text + "\n}",
		"package p\nvar _ = " + strings.TrimSuffix(text, ","),
		"package p\nvar _ = Colorcfg" + strings.TrimSuffix(text, ","),
		"package p\nvar _ = map[string]Colorcfg{" + text + "}",
	}
	for _, source := range candidates {
		file, err := parser.ParseFile(token.NewFileSet(), "", source, 0)
		if err != nil {
			continue
		}
		var calls []*ast.CallExpr
		var objects []*ast.CompositeLit
		ast.Inspect(file, func(node ast.Node) bool {
			if call, ok := node.(*ast.CallExpr); ok {
				name := ""
				switch f := call.Fun.(type) {
				case *ast.SelectorExpr:
					name = f.Sel.Name
				case *ast.Ident:
					name = f.Name
				}
				for _, function := range imageTemplateFunctions {
					if function == name {
						calls = append(calls, call)
					}
				}
			}
			if object, ok := node.(*ast.CompositeLit); ok {
				for _, element := range object.Elts {
					if field, ok := element.(*ast.KeyValueExpr); ok {
						if key, ok := field.Key.(*ast.Ident); ok && (key.Name == "Colors" || key.Name == "ColorStr") {
							objects = append(objects, object)
							break
						}
					}
				}
			}
			return true
		})
		if len(calls)+len(objects) > 1 {
			return result, fmt.Errorf("请一次粘贴一组颜色配置")
		}
		if len(calls) == 1 {
			return parseColorCall(calls[0])
		}
		if len(objects) == 1 {
			return parseColorObject(objects[0])
		}
	}
	if unquoted, err := strconv.Unquote(text); err == nil {
		text = unquoted
	}
	// Also accept a bare argument list / compact parameter object.
	args, err := splitCodeTestArgs(text)
	if err == nil && (len(args) == 8 || len(args) == 5) {
		name := "FindMultiColors"
		if len(args) == 8 && !strings.Contains(args[4], ",") {
			name = "FindColor"
		}
		if len(args) == 5 {
			name = "CmpColor"
		}
		expression, err := parser.ParseExpr(name + "(" + strings.Join(args, ",") + ")")
		if err == nil {
			return parseColorCall(expression.(*ast.CallExpr))
		}
	}
	result.Colors = text
	result.Function = inferImportedColorFunction(text)
	_, err = result.points(0, 0)
	return result, err
}

func importLiteral(expr ast.Expr) (string, error) {
	switch value := expr.(type) {
	case *ast.BasicLit:
		if value.Kind == token.STRING {
			return strconv.Unquote(value.Value)
		}
		return value.Value, nil
	case *ast.UnaryExpr:
		if value.Op == token.SUB || value.Op == token.ADD {
			text, err := importLiteral(value.X)
			return value.Op.String() + text, err
		}
	}
	return "", fmt.Errorf("参数必须是数字或字符串常量")
}

func importInt(expr ast.Expr) (int, error) {
	text, err := importLiteral(expr)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(text)
}

func parseColorCall(call *ast.CallExpr) (parsedColorImport, error) {
	result := parsedColorImport{}
	switch function := call.Fun.(type) {
	case *ast.SelectorExpr:
		result.Function = function.Sel.Name
	case *ast.Ident:
		result.Function = function.Name
	}
	count, colorIndex := 8, 4
	if result.Function == "CmpColor" {
		count, colorIndex = 5, 2
	}
	if len(call.Args) != count {
		return result, fmt.Errorf("%s 应有 %d 个参数", result.Function, count)
	}
	coords := [4]int{}
	for i := 0; i < colorIndex; i++ {
		value, err := importInt(call.Args[i])
		if err != nil {
			return result, fmt.Errorf("坐标无效：%w", err)
		}
		coords[i] = value
	}
	if count == 8 {
		result.Rect = &coords
		direction, err := importInt(call.Args[6])
		if err != nil {
			return result, fmt.Errorf("查找方向无效")
		}
		result.Direction = &direction
	} else {
		result.Anchor = &[2]int{coords[0], coords[1]}
	}
	var err error
	result.Colors, err = importLiteral(call.Args[colorIndex])
	if err != nil {
		return result, err
	}
	result.Similarity, err = importLiteral(call.Args[colorIndex+1])
	if err != nil {
		return result, err
	}
	displayID, err := importInt(call.Args[count-1])
	if err != nil {
		return result, fmt.Errorf("屏幕 ID 无效")
	}
	result.DisplayID = &displayID
	return result, result.validate()
}

func parseColorObject(object *ast.CompositeLit) (parsedColorImport, error) {
	result := parsedColorImport{}
	for _, element := range object.Elts {
		field, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := field.Key.(*ast.Ident)
		if !ok {
			continue
		}
		var err error
		switch key.Name {
		case "Colors", "ColorStr":
			result.Colors, err = importLiteral(field.Value)
		case "Sim":
			result.Similarity, err = importLiteral(field.Value)
		case "Dir", "Direction", "DisplayId", "DisplayID":
			var value int
			value, err = importInt(field.Value)
			if key.Name == "Dir" || key.Name == "Direction" {
				result.Direction = &value
			} else {
				result.DisplayID = &value
			}
		case "Rect":
			array, ok := field.Value.(*ast.CompositeLit)
			if !ok || len(array.Elts) != 4 {
				return result, fmt.Errorf("Rect 需要四个整数")
			}
			rect := [4]int{}
			for i, expr := range array.Elts {
				rect[i], err = importInt(expr)
				if err != nil {
					break
				}
			}
			result.Rect = &rect
		}
		if err != nil {
			return result, fmt.Errorf("%s 无效：%w", key.Name, err)
		}
	}
	result.Function = inferImportedColorFunction(result.Colors)
	return result, result.validate()
}

func inferImportedColorFunction(colors string) string {
	if strings.Contains(colors, ",") {
		return "FindMultiColors"
	}
	return "FindColor"
}

func (result parsedColorImport) validate() error {
	if result.Similarity != "" {
		sim, err := strconv.ParseFloat(result.Similarity, 64)
		if err != nil || !(sim >= 0.1 && sim <= 1) {
			return fmt.Errorf("相似度应为 0.1 到 1.0")
		}
	}
	if result.Direction != nil && (*result.Direction < 0 || *result.Direction > 3) {
		return fmt.Errorf("查找方向应为 0 到 3")
	}
	if result.DisplayID != nil && *result.DisplayID < 0 {
		return fmt.Errorf("屏幕 ID 不能为负数")
	}
	if result.Rect != nil {
		r := *result.Rect
		if r[0] < 0 || r[1] < 0 || r[2] < 0 || r[3] < 0 || (r[2] != 0 && r[2] <= r[0]) || (r[3] != 0 && r[3] <= r[1]) {
			return fmt.Errorf("范围无效；右、下边界可用 0 表示屏幕最大值")
		}
	}
	_, err := result.points(0, 0)
	return err
}

func (result parsedColorImport) points(baseX, baseY int) ([]ColorPoint, error) {
	if result.Anchor != nil {
		baseX, baseY = result.Anchor[0], result.Anchor[1]
	}
	var points []ColorPoint
	add := func(text string, x, y int) error {
		parts := strings.Split(strings.TrimSpace(text), "-")
		value, ok := normalizeColorOffset(parts[0])
		if !ok || len(parts) > 2 {
			return fmt.Errorf("颜色 %q 应为六位十六进制，可附加 -偏色", text)
		}
		offset := ""
		if len(parts) == 2 {
			var ok bool
			offset, ok = normalizeColorOffset(parts[1])
			if !ok {
				return fmt.Errorf("偏色 %q 无效", parts[1])
			}
		}
		points = append(points, ColorPoint{ID: len(points), Position: fmt.Sprintf("%d, %d", x, y), Color: "#" + value, Offset: offset, Selected: true})
		return nil
	}
	if result.Function == "FindMultiColors" || result.Function == "FindMultiColorsAll" {
		parts := strings.Split(result.Colors, ",")
		if len(parts)%3 != 1 {
			return nil, fmt.Errorf("多点颜色格式应为：基准颜色,dx,dy,颜色…")
		}
		if err := add(parts[0], baseX, baseY); err != nil {
			return nil, err
		}
		for i := 1; i < len(parts); i += 3 {
			dx, ex := strconv.Atoi(strings.TrimSpace(parts[i]))
			dy, ey := strconv.Atoi(strings.TrimSpace(parts[i+1]))
			if ex != nil || ey != nil {
				return nil, fmt.Errorf("相对坐标必须是整数")
			}
			if err := add(parts[i+2], baseX+dx, baseY+dy); err != nil {
				return nil, err
			}
		}
	} else {
		for _, color := range strings.Split(result.Colors, "|") {
			if err := add(color, baseX, baseY); err != nil {
				return nil, err
			}
		}
	}
	return points, nil
}
