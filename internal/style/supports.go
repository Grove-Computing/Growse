package style

import (
	"strconv"
	"strings"

	"github.com/Grove-Computing/Growse/internal/css"
)

func matchesSupportsGroups(groups []css.SupportsCondition) bool {
	for _, condition := range groups {
		if !matchesSupportsCondition(condition) {
			return false
		}
	}
	return true
}

func matchesSupportsCondition(condition css.SupportsCondition) bool {
	switch condition.Kind {
	case css.SupportsDeclaration:
		return supportsDeclaration(condition.Property, condition.Value)
	case css.SupportsSelector:
		return len(condition.Selectors) != 0
	case css.SupportsNot:
		return len(condition.Children) == 1 && !matchesSupportsCondition(condition.Children[0])
	case css.SupportsAnd:
		if len(condition.Children) == 0 {
			return false
		}
		for _, child := range condition.Children {
			if !matchesSupportsCondition(child) {
				return false
			}
		}
		return true
	case css.SupportsOr:
		for _, child := range condition.Children {
			if matchesSupportsCondition(child) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func supportsDeclaration(property, value string) bool {
	property = strings.ToLower(strings.TrimSpace(property))
	value = strings.TrimSpace(value)
	if property == "" || value == "" {
		return false
	}
	if strings.HasPrefix(property, "--") {
		return len(value) <= 64<<10
	}
	if parseGlobalKeyword(value) != globalNone {
		return supportsProperty(property)
	}
	context := LengthContext{FontSize: 16, RootFontSize: 16, ViewportWidth: 1280, ViewportHeight: 720, PercentageBase: 1280}
	switch property {
	case "display":
		_, ok := parseDisplay(value)
		return ok
	case "color", "background-color", "border-color", "border-top-color", "border-right-color", "border-bottom-color", "border-left-color", "outline-color", "text-decoration-color":
		_, ok := resolveColor(value, defaultTextColor, transparent, true, defaultTextColor)
		return ok
	case "font-size", "letter-spacing", "word-spacing", "text-indent":
		_, ok := ResolveLength(value, context)
		return ok
	case "line-height":
		_, ok := resolveLineHeight(value, 16, 16, context)
		return ok
	case "width", "height", "min-width", "min-height", "max-width", "max-height", "flex-basis":
		lower := strings.ToLower(value)
		if (property == "width" || property == "height" || strings.HasPrefix(property, "min-")) && lower == "auto" {
			return true
		}
		if strings.HasPrefix(property, "max-") && lower == "none" {
			return true
		}
		length, ok := ResolveLength(value, context)
		return ok && (length.Pixels >= 0 || length.Percentage != 0)
	case "margin", "margin-top", "margin-right", "margin-bottom", "margin-left", "padding", "padding-top", "padding-right", "padding-bottom", "padding-left", "gap", "row-gap", "column-gap", "top", "right", "bottom", "left":
		parts, ok := splitCSSSpaceSeparated(value)
		if !ok || len(parts) == 0 || len(parts) > 4 {
			return false
		}
		for _, part := range parts {
			if strings.EqualFold(part, "auto") && strings.HasPrefix(property, "margin") {
				continue
			}
			if _, valid := ResolveLength(part, context); !valid {
				return false
			}
		}
		return true
	case "opacity", "flex-grow", "flex-shrink", "order", "z-index":
		number, err := strconv.ParseFloat(value, 64)
		return err == nil && number == number
	case "font-weight":
		_, ok := parseFontWeight(value)
		return ok
	case "box-sizing":
		return value == "content-box" || value == "border-box"
	case "overflow", "overflow-x", "overflow-y":
		_, ok := resolveOverflow(value, OverflowVisible)
		return ok
	case "position":
		switch strings.ToLower(value) {
		case "static", "relative", "absolute", "fixed", "sticky":
			return true
		}
		return false
	case "visibility":
		return value == "visible" || value == "hidden" || value == "collapse"
	case "white-space":
		_, ok := resolveWhiteSpace(value, WhiteSpaceNormal)
		return ok
	case "transform":
		_, ok := parseTransform(value, context)
		return ok
	case "background-image":
		_, ok := parseBackgroundImage(value, defaultTextColor)
		return ok
	case "font-style":
		_, ok := parseFontStyle(value)
		return ok
	case "font-family":
		_, ok := parseFontFamilies(value)
		return ok
	case "font-stretch":
		_, ok := parseFontStretch(value)
		return ok
	case "text-align":
		return value == "start" || value == "end" || value == "left" || value == "right" || value == "center" || value == "justify"
	case "container-type":
		return value == "normal" || value == "inline-size"
	case "container-name":
		return value == "none" || !strings.ContainsAny(value, " \t\r\n/()")
	default:
		return false
	}
}

func supportsProperty(property string) bool {
	switch property {
	case "display", "color", "background-color", "background-image", "font-size", "font-weight", "font-family", "font-style", "font-stretch", "line-height", "letter-spacing", "word-spacing", "text-indent", "text-align",
		"width", "height", "min-width", "min-height", "max-width", "max-height", "box-sizing", "position", "top", "right", "bottom", "left", "z-index",
		"margin", "margin-top", "margin-right", "margin-bottom", "margin-left", "padding", "padding-top", "padding-right", "padding-bottom", "padding-left",
		"border", "border-width", "border-style", "border-color", "border-top", "border-right", "border-bottom", "border-left", "border-radius", "outline",
		"overflow", "overflow-x", "overflow-y", "visibility", "opacity", "white-space", "transform",
		"flex", "flex-flow", "flex-basis", "flex-grow", "flex-shrink", "order", "gap", "row-gap", "column-gap",
		"grid-template-columns", "grid-template-rows", "grid-auto-flow", "grid-column", "grid-row", "grid-area", "place-content", "place-items", "place-self", "container-type", "container-name":
		return true
	default:
		return false
	}
}
