package style

import (
	"strconv"
	"strings"
)

func applyShadowAndOutlineProperties(computed, parent ComputedStyle, winners map[string]winner, custom map[string]string, context LengthContext) ComputedStyle {
	computed.BoxShadows = resolveShadowWinner(computed.BoxShadows, parent.BoxShadows, winners["box-shadow"], custom, context, false, computed.Color)
	computed.TextShadows = resolveShadowWinner(computed.TextShadows, parent.TextShadows, winners["text-shadow"], custom, context, true, computed.Color)
	outlineCandidates := []struct {
		name  string
		apply func(string) bool
	}{
		{"outline-width", func(value string) bool {
			parsed, ok := parseBorderWidth(value, context)
			computed.Outline.Width = parsed
			return ok
		}},
		{"outline-style", func(value string) bool {
			parsed, ok := parseBorderStyle(value)
			computed.Outline.Style = parsed
			return ok
		}},
		{"outline-color", func(value string) bool {
			parsed, ok := parseColor(value, computed.Color)
			computed.Outline.Color = parsed
			return ok
		}},
	}
	for _, entry := range outlineCandidates {
		if candidate, ok := winners[entry.name]; ok {
			if value, valid := winnerValue(candidate, custom); valid {
				if candidate.source == "outline" {
					component := strings.TrimPrefix(entry.name, "outline-")
					if extracted, ok := borderComponentValue("border", component, 0, value); ok {
						value = extracted
					}
				}
				entry.apply(value)
			}
		}
	}
	if computed.Outline.Color == 0 {
		computed.Outline.Color = computed.Color
	}
	if candidate, ok := winners["outline-offset"]; ok {
		if value, valid := winnerValue(candidate, custom); valid {
			if length, ok := ResolveLength(value, context); ok && length.Percentage == 0 {
				computed.OutlineOffset = length.Pixels
			}
		}
	}
	return computed
}

func resolveShadowWinner(current, parent []Shadow, candidate winner, custom map[string]string, context LengthContext, textOnly bool, currentColor uint32) []Shadow {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current
	}
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return append([]Shadow(nil), parent...)
	case globalInitial, globalUnset:
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(value), "none") {
		return nil
	}
	parts := splitBackgroundArguments(value)
	result := make([]Shadow, 0, len(parts))
	for _, part := range parts {
		shadow, valid := parseShadow(part, context, textOnly, currentColor)
		if !valid {
			return current
		}
		result = append(result, shadow)
	}
	return result
}

func parseShadow(value string, context LengthContext, textOnly bool, currentColor uint32) (Shadow, bool) {
	parts, valid := splitCSSSpaceSeparated(value)
	if !valid {
		return Shadow{}, false
	}
	shadow := Shadow{Color: currentColor}
	var lengths []float32
	for _, part := range parts {
		if strings.EqualFold(part, "inset") && !textOnly {
			shadow.Inset = true
			continue
		}
		if color, ok := parseColor(part, currentColor); ok {
			shadow.Color = color
			continue
		}
		length, ok := ResolveLength(part, context)
		if !ok || length.Percentage != 0 {
			return Shadow{}, false
		}
		lengths = append(lengths, length.Pixels)
	}
	maximum := 4
	if textOnly {
		maximum = 3
	}
	if len(lengths) < 2 || len(lengths) > maximum || len(lengths) >= 3 && lengths[2] < 0 {
		return Shadow{}, false
	}
	shadow.OffsetX, shadow.OffsetY = lengths[0], lengths[1]
	if len(lengths) >= 3 {
		shadow.Blur = lengths[2]
	}
	if len(lengths) == 4 {
		shadow.Spread = lengths[3]
	}
	return shadow, true
}

func applyBorderRadii(current, parent BorderRadii, winners map[string]winner, customProperties map[string]string, context LengthContext) BorderRadii {
	properties := []string{"border-top-left-radius", "border-top-right-radius", "border-bottom-right-radius", "border-bottom-left-radius"}
	values := []*RadiusValue{&current.TopLeft, &current.TopRight, &current.BottomRight, &current.BottomLeft}
	parentValues := []RadiusValue{parent.TopLeft, parent.TopRight, parent.BottomRight, parent.BottomLeft}
	for index, property := range properties {
		candidate, ok := winners[property]
		if !ok {
			continue
		}
		resolved, ok := resolveVariables(candidate.value, customProperties)
		if !ok {
			continue
		}
		switch parseGlobalKeyword(resolved) {
		case globalInherit:
			*values[index] = parentValues[index]
		case globalInitial, globalUnset:
			*values[index] = RadiusValue{}
		default:
			if parsed, valid := parseRadiusValue(candidate.source, index, resolved, context); valid {
				*values[index] = parsed
			}
		}
	}
	return current
}

func parseRadiusValue(source string, corner int, value string, context LengthContext) (RadiusValue, bool) {
	if source != "border-radius" {
		return parseCornerRadius(value, context)
	}
	parts := strings.Split(value, "/")
	if len(parts) > 2 {
		return RadiusValue{}, false
	}
	horizontal := strings.Fields(parts[0])
	vertical := horizontal
	if len(parts) == 2 {
		vertical = strings.Fields(parts[1])
	}
	x, okX := radiusComponentForCorner(horizontal, corner, context)
	y, okY := radiusComponentForCorner(vertical, corner, context)
	return RadiusValue{X: x, Y: y}, okX && okY
}

func parseCornerRadius(value string, context LengthContext) (RadiusValue, bool) {
	parts := strings.Fields(value)
	if len(parts) == 0 || len(parts) > 2 {
		return RadiusValue{}, false
	}
	x, okX := parseRadiusLength(parts[0], context)
	y := x
	okY := okX
	if len(parts) == 2 {
		y, okY = parseRadiusLength(parts[1], context)
	}
	return RadiusValue{X: x, Y: y}, okX && okY
}

func radiusComponentForCorner(parts []string, corner int, context LengthContext) (LengthPercentage, bool) {
	if len(parts) == 0 || len(parts) > 4 {
		return LengthPercentage{}, false
	}
	indices := [4]int{0, 0, 0, 0}
	switch len(parts) {
	case 2:
		indices = [4]int{0, 1, 0, 1}
	case 3:
		indices = [4]int{0, 1, 2, 1}
	case 4:
		indices = [4]int{0, 1, 2, 3}
	}
	return parseRadiusLength(parts[indices[corner]], context)
}

func parseRadiusLength(value string, context LengthContext) (LengthPercentage, bool) {
	length, ok := ResolveLength(value, context)
	return length, ok && length.Pixels >= 0 && length.Percentage >= 0
}

func parseTextDecorationLine(value string) (TextDecorationLine, bool) {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(value)))
	if len(parts) == 0 {
		return TextDecorationNone, false
	}
	var result TextDecorationLine
	for _, part := range parts {
		switch part {
		case "none":
			if len(parts) != 1 {
				return TextDecorationNone, false
			}
		case "underline":
			result |= TextDecorationUnderline
		case "overline":
			result |= TextDecorationOverline
		case "line-through":
			result |= TextDecorationLineThrough
		default:
			return TextDecorationNone, false
		}
	}
	return result, true
}

func parseOpacity(value string) (float32, bool) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 32)
	return float32(parsed), err == nil && parsed >= 0 && parsed <= 1
}
