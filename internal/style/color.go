package style

import (
	"math"
	"strconv"
	"strings"

	"github.com/Grove-Computing/Growse/internal/css"
	"golang.org/x/image/colornames"
)

func parseColor(value string, currentColor uint32) (uint32, bool) {
	return parseColorDepth(value, currentColor, 0)
}

func parseColorDepth(value string, currentColor uint32, depth int) (uint32, bool) {
	if depth > css.MaxCSSFunctionDepth {
		return 0, false
	}
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "transparent" {
		return transparent, true
	}
	if value == "currentcolor" {
		return currentColor, true
	}
	if named, ok := colornames.Map[value]; ok {
		return packColor(named.R, named.G, named.B, named.A), true
	}
	if strings.HasPrefix(value, "#") {
		return parseHexColor(value[1:])
	}
	return parseFunctionalColor(value, currentColor, depth)
}

func parseHexColor(value string) (uint32, bool) {
	switch len(value) {
	case 3, 4:
		expanded := make([]byte, 0, len(value)*2)
		for index := range value {
			expanded = append(expanded, value[index], value[index])
		}
		value = string(expanded)
	case 6, 8:
	default:
		return 0, false
	}
	parsed, err := strconv.ParseUint(value, 16, 32)
	if err != nil {
		return 0, false
	}
	if len(value) == 6 {
		return uint32(parsed)<<8 | 0xff, true
	}
	return uint32(parsed), true
}

func parseFunctionalColor(value string, currentColor uint32, depth int) (uint32, bool) {
	open := strings.IndexByte(value, '(')
	if open <= 0 || !strings.HasSuffix(value, ")") {
		return 0, false
	}
	name := strings.TrimSpace(value[:open])
	inner := strings.TrimSpace(value[open+1 : len(value)-1])
	if name == "color-mix" {
		return parseColorMix(inner, currentColor, depth+1)
	}
	if !strings.Contains(inner, ",") {
		return parseModernFunctionalColor(name, inner)
	}
	parts := strings.Split(inner, ",")
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	switch name {
	case "rgb", "rgba":
		want := 3
		if name == "rgba" {
			want = 4
		}
		if len(parts) != want {
			return 0, false
		}
		percentage := strings.HasSuffix(parts[0], "%")
		channels := [3]uint8{}
		for index := range channels {
			if strings.HasSuffix(parts[index], "%") != percentage {
				return 0, false
			}
			channel, ok := parseRGBChannel(parts[index], percentage)
			if !ok {
				return 0, false
			}
			channels[index] = channel
		}
		alpha := uint8(255)
		if name == "rgba" {
			var ok bool
			alpha, ok = parseAlpha(parts[3])
			if !ok {
				return 0, false
			}
		}
		return packColor(channels[0], channels[1], channels[2], alpha), true
	case "hsl", "hsla":
		want := 3
		if name == "hsla" {
			want = 4
		}
		if len(parts) != want || !strings.HasSuffix(parts[1], "%") || !strings.HasSuffix(parts[2], "%") {
			return 0, false
		}
		hue, ok := parseFiniteFloat(strings.TrimSuffix(parts[0], "deg"))
		if !ok {
			return 0, false
		}
		saturation, ok := parseFiniteFloat(strings.TrimSuffix(parts[1], "%"))
		if !ok {
			return 0, false
		}
		lightness, ok := parseFiniteFloat(strings.TrimSuffix(parts[2], "%"))
		if !ok {
			return 0, false
		}
		red, green, blue := hslToRGB(hue, clamp(saturation/100, 0, 1), clamp(lightness/100, 0, 1))
		alpha := uint8(255)
		if name == "hsla" {
			alpha, ok = parseAlpha(parts[3])
			if !ok {
				return 0, false
			}
		}
		return packColor(red, green, blue, alpha), true
	default:
		return 0, false
	}
}

func parseModernFunctionalColor(name, inner string) (uint32, bool) {
	channelsText, alphaText, hasAlpha := splitColorSlash(inner)
	parts := strings.Fields(channelsText)
	alpha := uint8(255)
	var ok bool
	if hasAlpha {
		alpha, ok = parseAlpha(alphaText)
		if !ok {
			return 0, false
		}
	}
	switch name {
	case "rgb", "rgba":
		if len(parts) != 3 {
			return 0, false
		}
		channels := [3]uint8{}
		for index := range channels {
			channels[index], ok = parseRGBChannel(parts[index], strings.HasSuffix(parts[index], "%"))
			if !ok {
				return 0, false
			}
		}
		return packColor(channels[0], channels[1], channels[2], alpha), true
	case "hsl", "hsla":
		if len(parts) != 3 || !strings.HasSuffix(parts[1], "%") || !strings.HasSuffix(parts[2], "%") {
			return 0, false
		}
		hue, okHue := parseHue(parts[0])
		saturation, okSaturation := parseFiniteFloat(strings.TrimSuffix(parts[1], "%"))
		lightness, okLightness := parseFiniteFloat(strings.TrimSuffix(parts[2], "%"))
		if !okHue || !okSaturation || !okLightness {
			return 0, false
		}
		red, green, blue := hslToRGB(hue, clamp(saturation/100, 0, 1), clamp(lightness/100, 0, 1))
		return packColor(red, green, blue, alpha), true
	case "hwb":
		if len(parts) != 3 || !strings.HasSuffix(parts[1], "%") || !strings.HasSuffix(parts[2], "%") {
			return 0, false
		}
		hue, okHue := parseHue(parts[0])
		white, okWhite := parseFiniteFloat(strings.TrimSuffix(parts[1], "%"))
		black, okBlack := parseFiniteFloat(strings.TrimSuffix(parts[2], "%"))
		if !okHue || !okWhite || !okBlack {
			return 0, false
		}
		red, green, blue := hwbToRGB(hue, clamp(white/100, 0, 1), clamp(black/100, 0, 1))
		return packColor(red, green, blue, alpha), true
	case "lab", "lch", "oklab", "oklch":
		if len(parts) != 3 {
			return 0, false
		}
		lightness, okLightness := parsePercentageOrNumber(parts[0], name == "oklab" || name == "oklch")
		second, okSecond := parseFiniteFloat(parts[1])
		third, okThird := parseFiniteFloat(strings.TrimSuffix(parts[2], "deg"))
		if !okLightness || !okSecond || !okThird {
			return 0, false
		}
		a, b := second, third
		if name == "lch" || name == "oklch" {
			radians := third * math.Pi / 180
			a, b = second*math.Cos(radians), second*math.Sin(radians)
		}
		var red, green, blue float64
		if name == "oklab" || name == "oklch" {
			red, green, blue = oklabToSRGB(lightness, a, b)
		} else {
			red, green, blue = labToSRGB(lightness, a, b)
		}
		return packFloatColor(red, green, blue, float64(alpha)/255), true
	default:
		return 0, false
	}
}

func splitColorSlash(value string) (string, string, bool) {
	depth := 0
	for position := 0; position < len(value); position++ {
		switch value[position] {
		case '(':
			depth++
		case ')':
			depth--
		case '/':
			if depth == 0 {
				left, right := strings.TrimSpace(value[:position]), strings.TrimSpace(value[position+1:])
				return left, right, left != "" && right != ""
			}
		}
	}
	return strings.TrimSpace(value), "", false
}

func parseHue(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	switch {
	case strings.HasSuffix(value, "deg"):
		return parseFiniteFloat(strings.TrimSuffix(value, "deg"))
	case strings.HasSuffix(value, "turn"):
		number, ok := parseFiniteFloat(strings.TrimSuffix(value, "turn"))
		return number * 360, ok
	case strings.HasSuffix(value, "rad"):
		number, ok := parseFiniteFloat(strings.TrimSuffix(value, "rad"))
		return number * 180 / math.Pi, ok
	default:
		return parseFiniteFloat(value)
	}
}

func parsePercentageOrNumber(value string, normalized bool) (float64, bool) {
	percentage := strings.HasSuffix(value, "%")
	number, ok := parseFiniteFloat(strings.TrimSuffix(value, "%"))
	if !ok {
		return 0, false
	}
	if percentage && normalized {
		number /= 100
	}
	return number, true
}

func hwbToRGB(hue, white, black float64) (uint8, uint8, uint8) {
	if white+black > 1 {
		total := white + black
		white, black = white/total, black/total
	}
	red, green, blue := hslToRGB(hue, 1, 0.5)
	scale := 1 - white - black
	convert := func(channel uint8) uint8 {
		return uint8(math.Round(clamp(float64(channel)/255*scale+white, 0, 1) * 255))
	}
	return convert(red), convert(green), convert(blue)
}

func labToSRGB(lightness, a, b float64) (float64, float64, float64) {
	fy := (lightness + 16) / 116
	fx, fz := fy+a/500, fy-b/200
	convert := func(value float64) float64 {
		cube := value * value * value
		if cube > 216.0/24389 {
			return cube
		}
		return (116*value - 16) / (24389.0 / 27)
	}
	x50, y50, z50 := 0.96422*convert(fx), convert(fy), 0.82521*convert(fz)
	x := 0.9555766*x50 - 0.0230393*y50 + 0.0631636*z50
	y := -0.0282895*x50 + 1.0099416*y50 + 0.0210077*z50
	z := 0.0122982*x50 - 0.020483*y50 + 1.3299098*z50
	return xyzToSRGB(x, y, z)
}

func xyzToSRGB(x, y, z float64) (float64, float64, float64) {
	red := 3.2406*x - 1.5372*y - 0.4986*z
	green := -0.9689*x + 1.8758*y + 0.0415*z
	blue := 0.0557*x - 0.204*y + 1.057*z
	return srgbGamma(red), srgbGamma(green), srgbGamma(blue)
}

func oklabToSRGB(lightness, a, b float64) (float64, float64, float64) {
	l := lightness + 0.3963377774*a + 0.2158037573*b
	m := lightness - 0.1055613458*a - 0.0638541728*b
	s := lightness - 0.0894841775*a - 1.291485548*b
	l, m, s = l*l*l, m*m*m, s*s*s
	red := 4.0767416621*l - 3.3077115913*m + 0.2309699292*s
	green := -1.2684380046*l + 2.6097574011*m - 0.3413193965*s
	blue := -0.0041960863*l - 0.7034186147*m + 1.707614701*s
	return srgbGamma(red), srgbGamma(green), srgbGamma(blue)
}

func srgbGamma(value float64) float64 {
	if value <= 0.0031308 {
		return 12.92 * value
	}
	return 1.055*math.Pow(value, 1/2.4) - 0.055
}

func packFloatColor(red, green, blue, alpha float64) uint32 {
	channel := func(value float64) uint8 { return uint8(math.Round(clamp(value, 0, 1) * 255)) }
	return packColor(channel(red), channel(green), channel(blue), channel(alpha))
}

func parseColorMix(value string, currentColor uint32, depth int) (uint32, bool) {
	parts, ok := splitTopLevelComma(value)
	if !ok || len(parts) != 3 || !strings.EqualFold(strings.TrimSpace(parts[0]), "in srgb") {
		return 0, false
	}
	colors := [2]uint32{}
	weights := [2]float64{-1, -1}
	for index := range colors {
		colorText, weight, specified, valid := splitColorMixComponent(parts[index+1])
		if !valid {
			return 0, false
		}
		colors[index], valid = parseColorDepth(colorText, currentColor, depth)
		if !valid {
			return 0, false
		}
		if specified {
			weights[index] = weight
		}
	}
	if weights[0] < 0 && weights[1] < 0 {
		weights = [2]float64{50, 50}
	} else if weights[0] < 0 {
		weights[0] = 100 - weights[1]
	} else if weights[1] < 0 {
		weights[1] = 100 - weights[0]
	}
	total := weights[0] + weights[1]
	if total <= 0 || !finiteFloat(total) {
		return 0, false
	}
	weights[0], weights[1] = weights[0]/total, weights[1]/total
	component := func(shift uint) float64 {
		return float64((colors[0]>>shift)&0xff)*weights[0]/255 + float64((colors[1]>>shift)&0xff)*weights[1]/255
	}
	return packFloatColor(component(24), component(16), component(8), component(0)), true
}

func splitColorMixComponent(value string) (string, float64, bool, bool) {
	parts, ok := splitCSSSpaceSeparated(strings.TrimSpace(value))
	if !ok || len(parts) == 0 {
		return "", 0, false, false
	}
	last := parts[len(parts)-1]
	if !strings.HasSuffix(last, "%") {
		return strings.TrimSpace(value), 0, false, true
	}
	weight, valid := parseFiniteFloat(strings.TrimSuffix(last, "%"))
	if !valid || weight < 0 {
		return "", 0, false, false
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), last)), weight, true, true
}

func finiteFloat(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func parseRGBChannel(value string, percentage bool) (uint8, bool) {
	if percentage {
		value = strings.TrimSuffix(value, "%")
	}
	number, ok := parseFiniteFloat(value)
	if !ok {
		return 0, false
	}
	if percentage {
		number = number / 100 * 255
	}
	return uint8(math.Round(clamp(number, 0, 255))), true
}

func parseAlpha(value string) (uint8, bool) {
	percentage := strings.HasSuffix(value, "%")
	number, ok := parseFiniteFloat(strings.TrimSuffix(value, "%"))
	if !ok {
		return 0, false
	}
	if percentage {
		number /= 100
	}
	return uint8(math.Round(clamp(number, 0, 1) * 255)), true
}

func parseFiniteFloat(value string) (float64, bool) {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return number, err == nil && !math.IsNaN(number) && !math.IsInf(number, 0)
}

func hslToRGB(hue, saturation, lightness float64) (uint8, uint8, uint8) {
	hue = math.Mod(hue, 360)
	if hue < 0 {
		hue += 360
	}
	chroma := (1 - math.Abs(2*lightness-1)) * saturation
	x := chroma * (1 - math.Abs(math.Mod(hue/60, 2)-1))
	var red, green, blue float64
	switch {
	case hue < 60:
		red, green = chroma, x
	case hue < 120:
		red, green = x, chroma
	case hue < 180:
		green, blue = chroma, x
	case hue < 240:
		green, blue = x, chroma
	case hue < 300:
		red, blue = x, chroma
	default:
		red, blue = chroma, x
	}
	match := lightness - chroma/2
	return uint8(math.Round((red + match) * 255)),
		uint8(math.Round((green + match) * 255)),
		uint8(math.Round((blue + match) * 255))
}

func clamp(value, minimum, maximum float64) float64 {
	return min(max(value, minimum), maximum)
}

func packColor(red, green, blue, alpha uint8) uint32 {
	return uint32(red)<<24 | uint32(green)<<16 | uint32(blue)<<8 | uint32(alpha)
}
