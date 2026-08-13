package style

import (
	"math"
	"strconv"
	"strings"

	"golang.org/x/image/colornames"
)

func parseColor(value string, currentColor uint32) (uint32, bool) {
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
	return parseFunctionalColor(value)
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

func parseFunctionalColor(value string) (uint32, bool) {
	open := strings.IndexByte(value, '(')
	if open <= 0 || !strings.HasSuffix(value, ")") {
		return 0, false
	}
	name := strings.TrimSpace(value[:open])
	parts := strings.Split(value[open+1:len(value)-1], ",")
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
	number, ok := parseFiniteFloat(value)
	if !ok || strings.HasSuffix(value, "%") {
		return 0, false
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
