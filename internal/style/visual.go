package style

import (
	"math"
	"strconv"
	"strings"
)

const (
	MaxFilterFunctions = 16
	MaxFilterRadius    = float32(100)
	MaxFilterSurface   = float32(16 * 1024 * 1024)
)

func applyVisualProperties(computed, parent ComputedStyle, winners map[string]winner, context LengthContext) ComputedStyle {
	if candidate, ok := winners["object-fit"]; ok {
		if value, valid := resolvedWinner(candidate, computed.CustomProperties); valid {
			switch parseGlobalKeyword(value) {
			case globalInherit:
				computed.ObjectFit = parent.ObjectFit
			case globalInitial, globalUnset:
				computed.ObjectFit = ObjectFitFill
			default:
				if parsed, valid := parseObjectFit(value); valid {
					computed.ObjectFit = parsed
				}
			}
		}
	}
	if candidate, ok := winners["object-position"]; ok {
		if value, valid := resolvedWinner(candidate, computed.CustomProperties); valid {
			switch parseGlobalKeyword(value) {
			case globalInherit:
				computed.ObjectPosition = parent.ObjectPosition
			case globalInitial, globalUnset:
				computed.ObjectPosition = BackgroundPosition{X: LengthPercentage{Percentage: 50}, Y: LengthPercentage{Percentage: 50}}
			default:
				if parsed, valid := parseBackgroundPosition(value, context); valid {
					computed.ObjectPosition = parsed
				}
			}
		}
	}
	computed.ListStyleType = applyListStyleType(computed.ListStyleType, parent.ListStyleType, winners, computed.CustomProperties)
	computed.ListStylePosition = applyListStylePosition(computed.ListStylePosition, parent.ListStylePosition, winners, computed.CustomProperties)
	computed.ListStyleImage = applyListStyleImage(computed.ListStyleImage, parent.ListStyleImage, winners, computed.CustomProperties)
	if candidate, ok := winners["appearance"]; ok {
		if value, valid := resolvedWinner(candidate, computed.CustomProperties); valid {
			switch parseGlobalKeyword(value) {
			case globalInherit:
				computed.Appearance = parent.Appearance
			case globalInitial, globalUnset:
				computed.Appearance = AppearanceAuto
			default:
				if parsed, valid := parseAppearance(value); valid {
					computed.Appearance = parsed
				}
			}
		}
	}
	if candidate, ok := winners["accent-color"]; ok {
		if value, valid := resolvedWinner(candidate, computed.CustomProperties); valid {
			switch parseGlobalKeyword(value) {
			case globalInherit, globalUnset:
				computed.AccentColor, computed.AccentColorAuto = parent.AccentColor, parent.AccentColorAuto
			case globalInitial:
				computed.AccentColor, computed.AccentColorAuto = 0, true
			default:
				if strings.EqualFold(strings.TrimSpace(value), "auto") {
					computed.AccentColor, computed.AccentColorAuto = 0, true
				} else if parsed, valid := parseColor(value, computed.Color); valid {
					computed.AccentColor, computed.AccentColorAuto = parsed, false
				}
			}
		}
	}
	if candidate, ok := winners["cursor"]; ok {
		if value, valid := resolvedWinner(candidate, computed.CustomProperties); valid {
			switch parseGlobalKeyword(value) {
			case globalInherit, globalUnset:
				computed.Cursor = parent.Cursor
			case globalInitial:
				computed.Cursor = CursorAuto
			default:
				if parsed, valid := parseCursor(value); valid {
					computed.Cursor = parsed
				}
			}
		}
	}
	computed.Filters = applyFilterProperty("filter", computed.Filters, parent.Filters, winners, computed.CustomProperties, context, false)
	computed.BackdropFilters = applyFilterProperty("backdrop-filter", computed.BackdropFilters, parent.BackdropFilters, winners, computed.CustomProperties, context, false)
	if candidate, ok := winners["mix-blend-mode"]; ok {
		if value, valid := resolvedWinner(candidate, computed.CustomProperties); valid {
			switch parseGlobalKeyword(value) {
			case globalInherit:
				computed.MixBlendMode = parent.MixBlendMode
			case globalInitial, globalUnset:
				computed.MixBlendMode = BlendNormal
			default:
				if parsed, valid := parseBlendMode(value); valid {
					computed.MixBlendMode = parsed
				}
			}
		}
	}
	return computed
}

func resolvedWinner(candidate winner, custom map[string]string) (string, bool) {
	return resolveVariables(candidate.value, custom)
}

func parseObjectFit(value string) (ObjectFit, bool) {
	result, ok := map[string]ObjectFit{"fill": ObjectFitFill, "contain": ObjectFitContain, "cover": ObjectFitCover, "none": ObjectFitNone, "scale-down": ObjectFitScaleDown}[strings.ToLower(strings.TrimSpace(value))]
	return result, ok
}

func parseAppearance(value string) (Appearance, bool) {
	result, ok := map[string]Appearance{"auto": AppearanceAuto, "none": AppearanceNone}[strings.ToLower(strings.TrimSpace(value))]
	return result, ok
}

func parseCursor(value string) (Cursor, bool) {
	result, ok := map[string]Cursor{
		"auto": CursorAuto, "default": CursorDefault, "pointer": CursorPointer, "text": CursorText,
		"crosshair": CursorCrosshair, "move": CursorMove, "all-scroll": CursorMove, "grab": CursorGrab,
		"grabbing": CursorGrabbing, "not-allowed": CursorNotAllowed, "wait": CursorWait, "progress": CursorProgress,
		"col-resize": CursorColResize, "ew-resize": CursorColResize, "row-resize": CursorRowResize, "ns-resize": CursorRowResize,
	}[strings.ToLower(strings.TrimSpace(value))]
	return result, ok
}

func parseBlendMode(value string) (BlendMode, bool) {
	result, ok := map[string]BlendMode{"normal": BlendNormal, "multiply": BlendMultiply, "screen": BlendScreen, "overlay": BlendOverlay, "darken": BlendDarken, "lighten": BlendLighten}[strings.ToLower(strings.TrimSpace(value))]
	return result, ok
}

func applyListStyleType(current, parent ListStyleType, winners map[string]winner, custom map[string]string) ListStyleType {
	candidate, ok := winners["list-style-type"]
	if !ok {
		return current
	}
	value, valid := resolvedWinner(candidate, custom)
	if !valid {
		return current
	}
	value = listStyleComponent(candidate, value, "type")
	switch parseGlobalKeyword(value) {
	case globalInherit, globalUnset:
		return parent
	case globalInitial:
		return ListStyleDisc
	}
	parsed, valid := parseListStyleType(value)
	if valid {
		return parsed
	}
	return current
}

func parseListStyleType(value string) (ListStyleType, bool) {
	result, ok := map[string]ListStyleType{"disc": ListStyleDisc, "circle": ListStyleCircle, "square": ListStyleSquare, "decimal": ListStyleDecimal, "none": ListStyleNone}[strings.ToLower(strings.TrimSpace(value))]
	return result, ok
}

func applyListStylePosition(current, parent ListStylePosition, winners map[string]winner, custom map[string]string) ListStylePosition {
	candidate, ok := winners["list-style-position"]
	if !ok {
		return current
	}
	value, valid := resolvedWinner(candidate, custom)
	if !valid {
		return current
	}
	value = listStyleComponent(candidate, value, "position")
	switch parseGlobalKeyword(value) {
	case globalInherit, globalUnset:
		return parent
	case globalInitial:
		return ListStyleOutside
	}
	result, valid := map[string]ListStylePosition{"outside": ListStyleOutside, "inside": ListStyleInside}[strings.ToLower(strings.TrimSpace(value))]
	if valid {
		return result
	}
	return current
}

func applyListStyleImage(current, parent string, winners map[string]winner, custom map[string]string) string {
	candidate, ok := winners["list-style-image"]
	if !ok {
		return current
	}
	value, valid := resolvedWinner(candidate, custom)
	if !valid {
		return current
	}
	value = listStyleComponent(candidate, value, "image")
	switch parseGlobalKeyword(value) {
	case globalInherit, globalUnset:
		return parent
	case globalInitial:
		return ""
	}
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "none") {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(value), "url(") && strings.HasSuffix(value, ")") {
		return strings.Trim(strings.TrimSpace(value[4:len(value)-1]), `"'`)
	}
	return current
}

func listStyleComponent(candidate winner, value, component string) string {
	if candidate.source != "list-style" || parseGlobalKeyword(value) != globalNone {
		return value
	}
	parts, ok := splitCSSSpaceSeparated(value)
	if !ok || len(parts) == 0 || len(parts) > 3 {
		return ""
	}
	typeValue, position, image := "disc", "outside", "none"
	for _, part := range parts {
		lower := strings.ToLower(strings.TrimSpace(part))
		if lower == "inside" || lower == "outside" {
			position = lower
			continue
		}
		if strings.HasPrefix(lower, "url(") {
			image = part
			continue
		}
		if _, valid := parseListStyleType(lower); valid {
			typeValue = lower
			continue
		}
		return ""
	}
	switch component {
	case "type":
		return typeValue
	case "position":
		return position
	case "image":
		return image
	default:
		return ""
	}
}

func applyFilterProperty(property string, current, parent []Filter, winners map[string]winner, custom map[string]string, context LengthContext, inherited bool) []Filter {
	candidate, ok := winners[property]
	if !ok {
		return current
	}
	value, valid := resolvedWinner(candidate, custom)
	if !valid {
		return current
	}
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return append([]Filter(nil), parent...)
	case globalUnset:
		if inherited {
			return append([]Filter(nil), parent...)
		}
		return nil
	case globalInitial:
		return nil
	}
	parsed, valid := parseFilterList(value, context)
	if !valid {
		return current
	}
	return parsed
}

func parseFilterList(value string, context LengthContext) ([]Filter, bool) {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "none") {
		return nil, true
	}
	parts, ok := splitCSSSpaceSeparated(value)
	if !ok || len(parts) == 0 || len(parts) > MaxFilterFunctions {
		return nil, false
	}
	result := make([]Filter, 0, len(parts))
	for _, part := range parts {
		open := strings.IndexByte(part, '(')
		if open <= 0 || !strings.HasSuffix(part, ")") {
			continue
		}
		name, argument := strings.ToLower(strings.TrimSpace(part[:open])), strings.TrimSpace(part[open+1:len(part)-1])
		filter, valid := parseFilterFunction(name, argument, context)
		if valid {
			result = append(result, filter)
		}
	}
	return result, len(result) != 0
}

func parseFilterFunction(name, argument string, context LengthContext) (Filter, bool) {
	if name == "blur" {
		length, ok := ResolveLength(argument, context)
		if !ok || length.Percentage != 0 || length.Pixels < 0 || length.Pixels > MaxFilterRadius {
			return Filter{}, false
		}
		return Filter{Kind: FilterBlur, Radius: length.Pixels}, true
	}
	if name == "hue-rotate" {
		angle, ok := parseAngleDegrees(argument)
		return Filter{Kind: FilterHueRotate, Angle: angle}, ok
	}
	if name == "drop-shadow" {
		shadow, ok := parseShadow(argument, context, true, defaultTextColor)
		if !ok || shadow.Blur > MaxFilterRadius {
			return Filter{}, false
		}
		return Filter{Kind: FilterDropShadow, Shadow: shadow}, true
	}
	kinds := map[string]FilterKind{"brightness": FilterBrightness, "contrast": FilterContrast, "grayscale": FilterGrayscale, "invert": FilterInvert, "opacity": FilterOpacity, "saturate": FilterSaturate, "sepia": FilterSepia}
	kind, ok := kinds[name]
	if !ok {
		return Filter{}, false
	}
	amount, ok := parseFilterAmount(argument)
	if !ok {
		return Filter{}, false
	}
	if kind == FilterGrayscale || kind == FilterInvert || kind == FilterOpacity || kind == FilterSepia {
		amount = min(amount, 1)
	}
	return Filter{Kind: kind, Amount: amount}, true
}

func parseFilterAmount(value string) (float32, bool) {
	value = strings.TrimSpace(value)
	percentage := strings.HasSuffix(value, "%")
	if percentage {
		value = strings.TrimSpace(strings.TrimSuffix(value, "%"))
	}
	number, err := strconv.ParseFloat(value, 32)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
		return 0, false
	}
	result := float32(number)
	if percentage {
		result /= 100
	}
	return result, true
}

func parseAngleDegrees(value string) (float32, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	scale := float64(1)
	switch {
	case strings.HasSuffix(value, "deg"):
		value = strings.TrimSuffix(value, "deg")
	case strings.HasSuffix(value, "turn"):
		value, scale = strings.TrimSuffix(value, "turn"), 360
	case strings.HasSuffix(value, "rad"):
		value, scale = strings.TrimSuffix(value, "rad"), 180/math.Pi
	default:
		return 0, false
	}
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 32)
	return float32(number * scale), err == nil && !math.IsNaN(number) && !math.IsInf(number, 0)
}

// FilterSurfaceAllowed is shared by layout and paint to avoid allocating or
// requesting unbounded offscreen surfaces.
func FilterSurfaceAllowed(width, height float32) bool {
	return width > 0 && height > 0 && width <= 4096 && height <= 4096 && width*height <= MaxFilterSurface
}

// ApplyColorFilters applies the color-only portion of a filter chain to a
// solid paint color. Geometry filters remain on the display command.
func ApplyColorFilters(color uint32, filters []Filter) uint32 {
	r, g, b, a := float32(color>>24)/255, float32(color>>16&0xff)/255, float32(color>>8&0xff)/255, float32(color&0xff)/255
	mix := func(from, to, amount float32) float32 { return from + (to-from)*amount }
	for _, filter := range filters {
		switch filter.Kind {
		case FilterBrightness:
			r, g, b = r*filter.Amount, g*filter.Amount, b*filter.Amount
		case FilterContrast:
			r, g, b = (r-.5)*filter.Amount+.5, (g-.5)*filter.Amount+.5, (b-.5)*filter.Amount+.5
		case FilterGrayscale:
			gray := .2126*r + .7152*g + .0722*b
			r, g, b = mix(r, gray, filter.Amount), mix(g, gray, filter.Amount), mix(b, gray, filter.Amount)
		case FilterInvert:
			r, g, b = mix(r, 1-r, filter.Amount), mix(g, 1-g, filter.Amount), mix(b, 1-b, filter.Amount)
		case FilterOpacity:
			a *= filter.Amount
		case FilterSaturate:
			gray := .2126*r + .7152*g + .0722*b
			r, g, b = mix(gray, r, filter.Amount), mix(gray, g, filter.Amount), mix(gray, b, filter.Amount)
		case FilterSepia:
			sr, sg, sb := .393*r+.769*g+.189*b, .349*r+.686*g+.168*b, .272*r+.534*g+.131*b
			r, g, b = mix(r, sr, filter.Amount), mix(g, sg, filter.Amount), mix(b, sb, filter.Amount)
		case FilterHueRotate:
			angle := float64(filter.Angle) * math.Pi / 180
			cosine, sine := float32(math.Cos(angle)), float32(math.Sin(angle))
			nr := (.213+.787*cosine-.213*sine)*r + (.715-.715*cosine-.715*sine)*g + (.072-.072*cosine+.928*sine)*b
			ng := (.213-.213*cosine+.143*sine)*r + (.715+.285*cosine+.140*sine)*g + (.072-.072*cosine-.283*sine)*b
			nb := (.213-.213*cosine-.787*sine)*r + (.715-.715*cosine+.715*sine)*g + (.072+.928*cosine+.072*sine)*b
			r, g, b = nr, ng, nb
		}
	}
	byteValue := func(value float32) uint32 { return uint32(math.Round(float64(min(max(value, 0), 1) * 255))) }
	return byteValue(r)<<24 | byteValue(g)<<16 | byteValue(b)<<8 | byteValue(a)
}

// BlendColors composites a solid source against a solid backdrop using the
// supported mix-blend-mode subset. It is also used by deterministic tests and
// the renderer fallback when no offscreen backend is available.
func BlendColors(source, backdrop uint32, mode BlendMode) uint32 {
	sr, sg, sb, sa := float32(source>>24)/255, float32(source>>16&0xff)/255, float32(source>>8&0xff)/255, float32(source&0xff)/255
	br, bg, bb, ba := float32(backdrop>>24)/255, float32(backdrop>>16&0xff)/255, float32(backdrop>>8&0xff)/255, float32(backdrop&0xff)/255
	blend := func(s, b float32) float32 {
		switch mode {
		case BlendMultiply:
			return s * b
		case BlendScreen:
			return s + b - s*b
		case BlendOverlay:
			if b <= .5 {
				return 2 * s * b
			}
			return 1 - 2*(1-s)*(1-b)
		case BlendDarken:
			return min(s, b)
		case BlendLighten:
			return max(s, b)
		default:
			return s
		}
	}
	composite := func(s, b float32) float32 { return blend(s, b)*sa + b*(1-sa) }
	r, g, b := composite(sr, br), composite(sg, bg), composite(sb, bb)
	a := sa + ba*(1-sa)
	byteValue := func(value float32) uint32 { return uint32(math.Round(float64(min(max(value, 0), 1) * 255))) }
	return byteValue(r)<<24 | byteValue(g)<<16 | byteValue(b)<<8 | byteValue(a)
}
