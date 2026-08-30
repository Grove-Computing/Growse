package style

import (
	"math"
	"strconv"
	"strings"

	"github.com/Grove-Computing/Growse/internal/css"
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
	lower := strings.ToLower(value)
	linear := strings.HasPrefix(lower, "linear-gradient(")
	radial := strings.HasPrefix(lower, "radial-gradient(")
	conic := strings.HasPrefix(lower, "conic-gradient(")
	if (!linear && !radial && !conic) || !strings.HasSuffix(value, ")") {
		return BackgroundImage{}, false
	}
	prefixLength := len("linear-gradient(")
	if radial {
		prefixLength = len("radial-gradient(")
	} else if conic {
		prefixLength = len("conic-gradient(")
	}
	parts := splitBackgroundArguments(value[prefixLength : len(value)-1])
	if len(parts) < 2 {
		return BackgroundImage{}, false
	}
	angle := float32(180)
	if conic {
		angle = 0
	}
	center := BackgroundPosition{X: LengthPercentage{Percentage: 50}, Y: LengthPercentage{Percentage: 50}}
	circle := false
	if linear {
		if parsed, ok := parseGradientDirection(parts[0]); ok {
			angle = parsed
			parts = parts[1:]
		}
	} else if conic {
		descriptor := strings.ToLower(strings.TrimSpace(parts[0]))
		if strings.HasPrefix(descriptor, "from ") || strings.HasPrefix(descriptor, "at ") {
			if at := strings.Index(descriptor, " at "); at >= 0 {
				if parsed, valid := parseGradientDirection(strings.TrimSpace(descriptor[len("from "):at])); valid {
					angle = parsed
				}
				if parsed, valid := parseBackgroundPosition(descriptor[at+4:], LengthContext{}); valid {
					center = parsed
				}
			} else if strings.HasPrefix(descriptor, "from ") {
				if parsed, valid := parseGradientDirection(strings.TrimSpace(descriptor[len("from "):])); valid {
					angle = parsed
				}
			} else if parsed, valid := parseBackgroundPosition(descriptor[len("at "):], LengthContext{}); valid {
				center = parsed
			}
			parts = parts[1:]
		}
	} else if descriptor := strings.ToLower(strings.TrimSpace(parts[0])); strings.HasPrefix(descriptor, "circle") || strings.HasPrefix(descriptor, "ellipse") || strings.HasPrefix(descriptor, "at ") {
		circle = strings.HasPrefix(descriptor, "circle")
		if at := strings.Index(descriptor, " at "); at >= 0 {
			if parsed, valid := parseBackgroundPosition(descriptor[at+4:], LengthContext{}); valid {
				center = parsed
			}
		} else if strings.HasPrefix(descriptor, "at ") {
			if parsed, valid := parseBackgroundPosition(descriptor[3:], LengthContext{}); valid {
				center = parsed
			}
		}
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
	kind := BackgroundImageLinearGradient
	if radial {
		kind = BackgroundImageRadialGradient
	} else if conic {
		kind = BackgroundImageConicGradient
	}
	return BackgroundImage{Kind: kind, GradientAngle: angle, GradientStops: stops, GradientCenter: center, RadialCircle: circle}, true
}

func applyBackgroundLayers(computed, parent ComputedStyle, winners map[string]winner, custom map[string]string, context LengthContext) ComputedStyle {
	candidate, hasImages := winners["background-image"]
	if !hasImages {
		return computed
	}
	value, valid := winnerValue(candidate, custom)
	if !valid {
		return computed
	}
	if parseGlobalKeyword(value) == globalInherit {
		computed.BackgroundLayers = cloneBackgroundLayers(parent.BackgroundLayers)
		return computed
	}
	if parseGlobalKeyword(value) != globalNone {
		computed.BackgroundLayers = nil
		return computed
	}
	imageValues := splitBackgroundArguments(value)
	layers := make([]BackgroundLayer, 0, len(imageValues))
	for _, imageValue := range imageValues {
		image, valid := parseBackgroundImage(imageValue, computed.Color)
		if !valid {
			return computed
		}
		layers = append(layers, BackgroundLayer{Image: image, Repeat: BackgroundRepeat{X: true, Y: true}, Origin: computed.BackgroundOrigin, Clip: computed.BackgroundClip})
	}
	applyLayerValues := func(property string, apply func(*BackgroundLayer, string) bool) bool {
		candidate, ok := winners[property]
		if !ok {
			return true
		}
		value, valid := winnerValue(candidate, custom)
		if !valid || parseGlobalKeyword(value) != globalNone {
			return valid
		}
		values := splitBackgroundArguments(value)
		if len(values) == 0 {
			return false
		}
		for index := range layers {
			if !apply(&layers[index], values[index%len(values)]) {
				return false
			}
		}
		return true
	}
	if !applyLayerValues("background-repeat", func(layer *BackgroundLayer, value string) bool {
		parsed, valid := parseBackgroundRepeat(value)
		layer.Repeat = parsed
		return valid
	}) || !applyLayerValues("background-position", func(layer *BackgroundLayer, value string) bool {
		parsed, valid := parseBackgroundPosition(value, context)
		layer.Position = parsed
		return valid
	}) || !applyLayerValues("background-size", func(layer *BackgroundLayer, value string) bool {
		parsed, valid := parseBackgroundSize(value, context)
		layer.Size = parsed
		return valid
	}) || !applyLayerValues("background-origin", func(layer *BackgroundLayer, value string) bool {
		parsed, valid := parseBackgroundBox(value)
		layer.Origin = parsed
		return valid
	}) || !applyLayerValues("background-clip", func(layer *BackgroundLayer, value string) bool {
		parsed, valid := parseBackgroundBox(value)
		layer.Clip = parsed
		return valid
	}) {
		return computed
	}
	computed.BackgroundLayers = layers
	if len(layers) != 0 {
		computed.BackgroundImage, computed.BackgroundRepeat = layers[0].Image, layers[0].Repeat
		computed.BackgroundPos, computed.BackgroundSize = layers[0].Position, layers[0].Size
		computed.BackgroundOrigin, computed.BackgroundClip = layers[0].Origin, layers[0].Clip
	}
	return computed
}

func parseBackgroundBox(value string) (BackgroundBox, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "border-box":
		return BackgroundBoxBorder, true
	case "padding-box":
		return BackgroundBoxPadding, true
	case "content-box":
		return BackgroundBoxContent, true
	default:
		return BackgroundBoxBorder, false
	}
}

func cloneBackgroundLayers(source []BackgroundLayer) []BackgroundLayer {
	result := append([]BackgroundLayer(nil), source...)
	for index := range result {
		result[index].Image.GradientStops = append([]GradientStop(nil), result[index].Image.GradientStops...)
	}
	return result
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
				} else if strings.HasSuffix(positionText, "deg") {
					position, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(positionText, "deg")), 32)
					if err == nil && !math.IsNaN(position) && !math.IsInf(position, 0) {
						return strings.TrimSpace(value[:index]), float32(position) / 360, true
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
