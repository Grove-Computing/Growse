package style

import (
	"strconv"
	"strings"
)

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
