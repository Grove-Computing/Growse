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
		if (property == "letter-spacing" || property == "word-spacing") && strings.EqualFold(value, "normal") {
			return true
		}
		_, ok := ResolveLength(value, context)
		return ok
	case "font":
		_, ok := parseFontShorthand(value)
		return ok
	case "line-height":
		_, ok := resolveLineHeight(value, 16, 16, context)
		return ok
	case "width", "height", "min-width", "min-height", "max-width", "max-height",
		"inline-size", "block-size", "min-inline-size", "min-block-size", "max-inline-size", "max-block-size", "flex-basis":
		lower := strings.ToLower(value)
		if lower == "min-content" || lower == "max-content" || lower == "fit-content" {
			return true
		}
		if (property == "width" || property == "height" || property == "inline-size" || property == "block-size" || strings.HasPrefix(property, "min-")) && lower == "auto" {
			return true
		}
		if strings.HasPrefix(property, "max-") && lower == "none" {
			return true
		}
		length, ok := ResolveLength(value, context)
		return ok && (length.Pixels >= 0 || length.Percentage != 0)
	case "margin", "margin-top", "margin-right", "margin-bottom", "margin-left",
		"margin-block", "margin-inline", "margin-block-start", "margin-block-end", "margin-inline-start", "margin-inline-end",
		"padding", "padding-top", "padding-right", "padding-bottom", "padding-left",
		"padding-block", "padding-inline", "padding-block-start", "padding-block-end", "padding-inline-start", "padding-inline-end",
		"gap", "row-gap", "column-gap", "top", "right", "bottom", "left",
		"inset-block", "inset-inline", "inset-block-start", "inset-block-end", "inset-inline-start", "inset-inline-end":
		parts, ok := splitCSSSpaceSeparated(value)
		maxParts := 4
		if strings.Contains(property, "-block") || strings.Contains(property, "-inline") {
			maxParts = 1
			if strings.HasSuffix(property, "-block") || strings.HasSuffix(property, "-inline") {
				maxParts = 2
			}
		}
		if !ok || len(parts) == 0 || len(parts) > maxParts {
			return false
		}
		for _, part := range parts {
			if strings.EqualFold(part, "auto") && (strings.HasPrefix(property, "margin") || strings.HasPrefix(property, "inset") || property == "top" || property == "right" || property == "bottom" || property == "left") {
				continue
			}
			if _, valid := ResolveLength(part, context); !valid {
				return false
			}
		}
		return true
	case "border-block", "border-inline", "border-block-start", "border-block-end", "border-inline-start", "border-inline-end",
		"border-block-width", "border-block-style", "border-block-color", "border-inline-width", "border-inline-style", "border-inline-color",
		"border-block-start-width", "border-block-start-style", "border-block-start-color",
		"border-block-end-width", "border-block-end-style", "border-block-end-color",
		"border-inline-start-width", "border-inline-start-style", "border-inline-start-color",
		"border-inline-end-width", "border-inline-end-style", "border-inline-end-color",
		"border-start-start-radius", "border-start-end-radius", "border-end-start-radius", "border-end-end-radius":
		return supportsLogicalBorderDeclaration(property, value, context)
	case "opacity", "flex-grow", "flex-shrink", "order", "z-index":
		number, err := strconv.ParseFloat(value, 64)
		return err == nil && number == number
	case "font-weight":
		_, ok := parseFontWeight(value)
		return ok
	case "box-sizing":
		return value == "content-box" || value == "border-box"
	case "aspect-ratio":
		_, ok := parseAspectRatio(value)
		return ok
	case "table-layout":
		return value == "auto" || value == "fixed"
	case "border-collapse":
		return value == "separate" || value == "collapse"
	case "caption-side":
		return value == "top" || value == "bottom"
	case "border-spacing":
		parts, ok := splitCSSSpaceSeparated(value)
		if !ok || len(parts) < 1 || len(parts) > 2 {
			return false
		}
		for _, part := range parts {
			length, valid := ResolveLength(part, context)
			if !valid || length.Percentage != 0 || length.Pixels < 0 {
				return false
			}
		}
		return true
	case "grid-template-columns", "grid-template-rows":
		if isSubgridValue(value) {
			return true
		}
		_, ok := parseGridTrackList(value, context, true)
		return ok
	case "overflow", "overflow-x", "overflow-y":
		_, ok := resolveOverflow(value, OverflowVisible)
		return ok
	case "position":
		switch strings.ToLower(value) {
		case "static", "relative", "absolute", "fixed", "sticky":
			return true
		}
		return false
	case "float":
		_, ok := resolveFloatSide(value, FloatNone)
		return ok
	case "clear":
		_, ok := resolveClear(value, ClearNone)
		return ok
	case "visibility":
		return value == "visible" || value == "hidden" || value == "collapse"
	case "white-space":
		_, ok := resolveWhiteSpace(value, WhiteSpaceNormal)
		return ok
	case "writing-mode":
		return value == "horizontal-tb" || value == "vertical-rl" || value == "vertical-lr"
	case "direction":
		return value == "ltr" || value == "rtl"
	case "justify-content":
		alignment, _, valid := parseOverflowAlignment(value)
		if !valid {
			return false
		}
		switch alignment {
		case "normal", "flex-start", "flex-end", "start", "end", "left", "right", "center", "space-between", "space-around", "space-evenly":
			return true
		}
		return false
	case "align-content", "align-items", "justify-items", "align-self", "justify-self":
		allowAuto := strings.HasSuffix(property, "-self")
		distributed := property == "align-content"
		_, _, ok := parseAlign(value, allowAuto, distributed)
		return ok
	case "transform":
		_, ok := parseTransform(value, context)
		return ok
	case "background-image":
		_, ok := parseBackgroundImage(value, defaultTextColor)
		return ok
	case "background-origin", "background-clip":
		_, ok := parseBackgroundBox(value)
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
	case "text-transform":
		return value == "none" || value == "uppercase" || value == "lowercase" || value == "capitalize"
	case "word-break":
		return value == "normal" || value == "break-all" || value == "keep-all"
	case "overflow-wrap":
		return value == "normal" || value == "break-word" || value == "anywhere"
	case "vertical-align":
		parsed := parseVerticalAlign(value, VerticalAlign{}, context)
		return parsed.Kind != VerticalAlignBaseline || strings.EqualFold(strings.TrimSpace(value), "baseline") || parsed.Value != 0
	case "text-overflow":
		return value == "clip" || value == "ellipsis"
	case "object-fit":
		_, ok := parseObjectFit(value)
		return ok
	case "object-position":
		_, ok := parseBackgroundPosition(value, context)
		return ok
	case "list-style-type":
		_, ok := parseListStyleType(value)
		return ok
	case "list-style":
		candidate := winner{source: "list-style"}
		return listStyleComponent(candidate, value, "type") != "" && listStyleComponent(candidate, value, "position") != ""
	case "list-style-position":
		return value == "inside" || value == "outside"
	case "list-style-image":
		return value == "none" || strings.HasPrefix(strings.ToLower(value), "url(") && strings.HasSuffix(value, ")")
	case "appearance", "-webkit-appearance":
		_, ok := parseAppearance(value)
		return ok
	case "accent-color":
		if value == "auto" {
			return true
		}
		_, ok := parseColor(value, defaultTextColor)
		return ok
	case "cursor":
		_, ok := parseCursor(value)
		return ok
	case "filter", "backdrop-filter":
		_, ok := parseFilterList(value, context)
		return ok
	case "mix-blend-mode":
		_, ok := parseBlendMode(value)
		return ok
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
	case "display", "color", "background-color", "background-image", "background-origin", "background-clip", "font", "font-size", "font-weight", "font-family", "font-style", "font-stretch", "line-height", "letter-spacing", "word-spacing", "text-indent", "text-align", "text-transform", "word-break", "overflow-wrap", "vertical-align", "text-overflow",
		"object-fit", "object-position", "list-style", "list-style-type", "list-style-position", "list-style-image", "appearance", "-webkit-appearance", "accent-color", "cursor", "filter", "backdrop-filter", "mix-blend-mode",
		"width", "height", "min-width", "min-height", "max-width", "max-height", "inline-size", "block-size", "min-inline-size", "min-block-size", "max-inline-size", "max-block-size", "box-sizing", "aspect-ratio", "position", "top", "right", "bottom", "left", "z-index", "float", "clear",
		"table-layout", "border-collapse", "border-spacing", "caption-side",
		"margin", "margin-top", "margin-right", "margin-bottom", "margin-left", "margin-block", "margin-inline", "margin-block-start", "margin-block-end", "margin-inline-start", "margin-inline-end",
		"padding", "padding-top", "padding-right", "padding-bottom", "padding-left", "padding-block", "padding-inline", "padding-block-start", "padding-block-end", "padding-inline-start", "padding-inline-end",
		"border", "border-width", "border-style", "border-color", "border-top", "border-right", "border-bottom", "border-left", "border-radius", "outline",
		"border-block", "border-inline", "border-block-start", "border-block-end", "border-inline-start", "border-inline-end",
		"border-block-width", "border-block-style", "border-block-color", "border-inline-width", "border-inline-style", "border-inline-color",
		"border-block-start-width", "border-block-start-style", "border-block-start-color", "border-block-end-width", "border-block-end-style", "border-block-end-color",
		"border-inline-start-width", "border-inline-start-style", "border-inline-start-color", "border-inline-end-width", "border-inline-end-style", "border-inline-end-color",
		"border-start-start-radius", "border-start-end-radius", "border-end-start-radius", "border-end-end-radius",
		"inset-block", "inset-inline", "inset-block-start", "inset-block-end", "inset-inline-start", "inset-inline-end",
		"overflow", "overflow-x", "overflow-y", "visibility", "opacity", "white-space", "writing-mode", "direction", "transform",
		"flex", "flex-flow", "flex-basis", "flex-grow", "flex-shrink", "order", "gap", "row-gap", "column-gap", "justify-content", "align-content", "align-items", "justify-items", "align-self", "justify-self",
		"grid-template-columns", "grid-template-rows", "grid-auto-flow", "grid-column", "grid-row", "grid-area", "place-content", "place-items", "place-self", "container-type", "container-name":
		return true
	default:
		return false
	}
}

func supportsLogicalBorderDeclaration(property, value string, context LengthContext) bool {
	parts, ok := splitCSSSpaceSeparated(value)
	if !ok || len(parts) == 0 {
		return false
	}
	if strings.HasSuffix(property, "-radius") {
		if len(parts) > 2 {
			return false
		}
		for _, part := range parts {
			length, valid := ResolveLength(part, context)
			if !valid || length.Pixels < 0 && length.Percentage == 0 {
				return false
			}
		}
		return true
	}
	component := ""
	for _, candidate := range []string{"width", "style", "color"} {
		if strings.HasSuffix(property, "-"+candidate) {
			component = candidate
			break
		}
	}
	if component != "" {
		axisShorthand := property == "border-block-"+component || property == "border-inline-"+component
		if len(parts) > 2 || !axisShorthand && len(parts) != 1 {
			return false
		}
		for _, part := range parts {
			switch component {
			case "width":
				if _, valid := parseBorderWidth(part, context); !valid {
					return false
				}
			case "style":
				if _, valid := parseBorderStyle(part); !valid {
					return false
				}
			case "color":
				if _, valid := parseColor(part, defaultTextColor); !valid {
					return false
				}
			}
		}
		return true
	}
	for _, part := range parts {
		if _, valid := parseBorderWidth(part, context); valid {
			continue
		}
		if _, valid := parseBorderStyle(part); valid {
			continue
		}
		if _, valid := parseColor(part, defaultTextColor); valid {
			continue
		}
		return false
	}
	return len(parts) <= 3
}
