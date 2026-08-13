package style

import (
	"math"
	"strconv"
	"strings"

	"github.com/saku0512/growse/internal/css"
)

func parseBackgroundImage(value string, currentColor uint32) (BackgroundImage, bool) {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "none") {
		return BackgroundImage{}, true
	}
	if strings.HasPrefix(strings.ToLower(value), "url(") && strings.HasSuffix(value, ")") {
		raw := strings.TrimSpace(value[4 : len(value)-1])
		if decoded, ok := css.DecodeString(raw); ok {
			raw = decoded
		}
		if raw == "" || strings.ContainsAny(raw, "\x00\r\n") {
			return BackgroundImage{}, false
		}
		return BackgroundImage{Kind: BackgroundImageURL, URL: raw}, true
	}
	if !strings.HasPrefix(strings.ToLower(value), "linear-gradient(") || !strings.HasSuffix(value, ")") {
		return BackgroundImage{}, false
	}
	parts := splitBackgroundArguments(value[len("linear-gradient(") : len(value)-1])
	if len(parts) < 2 {
		return BackgroundImage{}, false
	}
	angle := float32(180)
	if parsed, ok := parseGradientDirection(parts[0]); ok {
		angle = parsed
		parts = parts[1:]
	}
	stops := make([]GradientStop, 0, len(parts))
	positions := make([]bool, 0, len(parts))
	for _, part := range parts {
		colorText, position, hasPosition := splitColorStop(part)
		color, ok := parseColor(colorText, currentColor)
		if !ok {
			return BackgroundImage{}, false
		}
		stops = append(stops, GradientStop{Color: color, Position: position})
		positions = append(positions, hasPosition)
	}
	if len(stops) < 2 {
		return BackgroundImage{}, false
	}
	distributeGradientStops(stops, positions)
	return BackgroundImage{Kind: BackgroundImageLinearGradient, GradientAngle: angle, GradientStops: stops}, true
}

func splitBackgroundArguments(value string) []string {
	var result []string
	start, depth := 0, 0
	quote := byte(0)
	for index := 0; index < len(value); index++ {
		character := value[index]
		if quote != 0 {
			if character == '\\' {
				index++
			} else if character == quote {
				quote = 0
			}
			continue
		}
		switch character {
		case '\'', '"':
			quote = character
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				result = append(result, strings.TrimSpace(value[start:index]))
				start = index + 1
			}
		}
	}
	result = append(result, strings.TrimSpace(value[start:]))
	return result
}

func parseGradientDirection(value string) (float32, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "to top":
		return 0, true
	case "to right":
		return 90, true
	case "to bottom":
		return 180, true
	case "to left":
		return 270, true
	case "to top right", "to right top":
		return 45, true
	case "to bottom right", "to right bottom":
		return 135, true
	case "to bottom left", "to left bottom":
		return 225, true
	case "to top left", "to left top":
		return 315, true
	}
	if strings.HasSuffix(value, "deg") {
		angle, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, "deg")), 32)
		return normalizeAngle(float32(angle)), err == nil && !math.IsNaN(angle) && !math.IsInf(angle, 0)
	}
	return 0, false
}

func normalizeAngle(value float32) float32 {
	value = float32(math.Mod(float64(value), 360))
	if value < 0 {
		value += 360
	}
	return value
}

func splitColorStop(value string) (string, float32, bool) {
	value = strings.TrimSpace(value)
	depth := 0
	for index := len(value) - 1; index >= 0; index-- {
		switch value[index] {
		case ')':
			depth++
		case '(':
			depth--
		case ' ', '\t':
			if depth == 0 {
				positionText := strings.TrimSpace(value[index+1:])
				if strings.HasSuffix(positionText, "%") {
					position, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(positionText, "%")), 32)
					if err == nil && !math.IsNaN(position) && !math.IsInf(position, 0) {
						return strings.TrimSpace(value[:index]), float32(position) / 100, true
					}
				}
				return value, 0, false
			}
		}
	}
	return value, 0, false
}

func distributeGradientStops(stops []GradientStop, explicit []bool) {
	if !explicit[0] {
		stops[0].Position = 0
		explicit[0] = true
	}
	last := len(stops) - 1
	if !explicit[last] {
		stops[last].Position = 1
		explicit[last] = true
	}
	for index := 1; index < len(stops); {
		if explicit[index] {
			index++
			continue
		}
		start := index - 1
		end := index
		for end < len(stops) && !explicit[end] {
			end++
		}
		span := stops[end].Position - stops[start].Position
		for current := index; current < end; current++ {
			stops[current].Position = stops[start].Position + span*float32(current-start)/float32(end-start)
		}
		index = end + 1
	}
	previous := float32(0)
	for index := range stops {
		stops[index].Position = min(max(stops[index].Position, previous), float32(1))
		previous = stops[index].Position
	}
}

func parseBackgroundRepeat(value string) (BackgroundRepeat, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "repeat":
		return BackgroundRepeat{X: true, Y: true}, true
	case "repeat-x":
		return BackgroundRepeat{X: true}, true
	case "repeat-y":
		return BackgroundRepeat{Y: true}, true
	case "no-repeat":
		return BackgroundRepeat{}, true
	default:
		return BackgroundRepeat{}, false
	}
}

func parseBackgroundPosition(value string, context LengthContext) (BackgroundPosition, bool) {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(value)))
	if len(parts) == 0 || len(parts) > 2 {
		return BackgroundPosition{}, false
	}
	if len(parts) == 1 {
		if parts[0] == "top" || parts[0] == "bottom" {
			parts = []string{"center", parts[0]}
		} else {
			parts = append(parts, "center")
		}
	}
	x, okX := parseBackgroundPositionComponent(parts[0], true, context)
	y, okY := parseBackgroundPositionComponent(parts[1], false, context)
	return BackgroundPosition{X: x, Y: y}, okX && okY
}

func parseBackgroundPositionComponent(value string, horizontal bool, context LengthContext) (LengthPercentage, bool) {
	switch value {
	case "left":
		return LengthPercentage{}, horizontal
	case "right":
		return LengthPercentage{Percentage: 100}, horizontal
	case "top":
		return LengthPercentage{}, !horizontal
	case "bottom":
		return LengthPercentage{Percentage: 100}, !horizontal
	case "center":
		return LengthPercentage{Percentage: 50}, true
	default:
		return ResolveLength(value, context)
	}
}

func parseBackgroundSize(value string, context LengthContext) (BackgroundSize, bool) {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(value)))
	if len(parts) == 1 {
		switch parts[0] {
		case "auto":
			return BackgroundSize{}, true
		case "cover":
			return BackgroundSize{Kind: BackgroundSizeCover}, true
		case "contain":
			return BackgroundSize{Kind: BackgroundSizeContain}, true
		}
	}
	if len(parts) == 0 || len(parts) > 2 {
		return BackgroundSize{}, false
	}
	width, ok := parseBackgroundSizeComponent(parts[0], context)
	if !ok {
		return BackgroundSize{}, false
	}
	height := SizeValue{Kind: SizeAuto}
	if len(parts) == 2 {
		height, ok = parseBackgroundSizeComponent(parts[1], context)
	}
	return BackgroundSize{Kind: BackgroundSizeExplicit, Width: width, Height: height}, ok
}

func parseBackgroundSizeComponent(value string, context LengthContext) (SizeValue, bool) {
	if value == "auto" {
		return SizeValue{Kind: SizeAuto}, true
	}
	length, ok := ResolveLength(value, context)
	if !ok || length.Pixels < 0 || length.Percentage < 0 {
		return SizeValue{}, false
	}
	return SizeValue{Kind: SizeLength, Value: length}, true
}
