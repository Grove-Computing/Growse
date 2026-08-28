package style

import (
	"math"
	"strconv"
	"strings"
)

// LengthContext contains the bases needed to resolve CSS length units.
type LengthContext struct {
	FontSize        float32
	RootFontSize    float32
	ViewportWidth   float32
	ViewportHeight  float32
	PercentageBase  float32
	ContainerWidth  float32
	ContainerHeight float32
}

// LengthPercentage preserves the percentage component until a basis is known.
type LengthPercentage struct {
	Pixels     float32
	Percentage float32
}

// Resolve converts a length-percentage to CSS pixels for a concrete basis.
func (value LengthPercentage) Resolve(percentageBase float32) float32 {
	return value.Pixels + value.Percentage*percentageBase/100
}

// ResolveLength parses and resolves a CSS3 length or percentage.
func ResolveLength(value string, context LengthContext) (LengthPercentage, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.HasPrefix(value, "calc(") {
		return resolveCalculation(value, context)
	}
	return resolveSimpleLength(value, context)
}

func resolveSimpleLength(value string, context LengthContext) (LengthPercentage, bool) {
	number, unit, ok := splitNumberAndUnit(value)
	if !ok || math.IsNaN(float64(number)) || math.IsInf(float64(number), 0) {
		return LengthPercentage{}, false
	}
	result := LengthPercentage{}
	switch unit {
	case "":
		if number != 0 {
			return LengthPercentage{}, false
		}
	case "px":
		result.Pixels = number
	case "in":
		result.Pixels = number * 96
	case "cm":
		result.Pixels = number * 96 / 2.54
	case "mm":
		result.Pixels = number * 96 / 25.4
	case "q":
		result.Pixels = number * 96 / 101.6
	case "pt":
		result.Pixels = number * 96 / 72
	case "pc":
		result.Pixels = number * 16
	case "em":
		result.Pixels = number * context.FontSize
	case "rem":
		result.Pixels = number * context.RootFontSize
	case "ex", "ch":
		// Until font-specific metrics are introduced, use the CSS fallback of
		// half an em for both x-height and zero advance.
		result.Pixels = number * context.FontSize * 0.5
	case "vw":
		result.Pixels = number * context.ViewportWidth / 100
	case "vh":
		result.Pixels = number * context.ViewportHeight / 100
	case "vmin":
		result.Pixels = number * min(context.ViewportWidth, context.ViewportHeight) / 100
	case "vmax":
		result.Pixels = number * max(context.ViewportWidth, context.ViewportHeight) / 100
	case "cqw", "cqi":
		basis := context.ContainerWidth
		if basis <= 0 {
			basis = context.ViewportWidth
		}
		result.Pixels = number * basis / 100
	case "cqh", "cqb":
		basis := context.ContainerHeight
		if basis <= 0 {
			basis = context.ViewportHeight
		}
		result.Pixels = number * basis / 100
	case "cqmin", "cqmax":
		width, height := context.ContainerWidth, context.ContainerHeight
		if width <= 0 {
			width = context.ViewportWidth
		}
		if height <= 0 {
			height = context.ViewportHeight
		}
		basis := min(width, height)
		if unit == "cqmax" {
			basis = max(width, height)
		}
		result.Pixels = number * basis / 100
	case "%":
		result.Percentage = number
	default:
		return LengthPercentage{}, false
	}
	if math.IsNaN(float64(result.Pixels)) || math.IsInf(float64(result.Pixels), 0) {
		return LengthPercentage{}, false
	}
	return result, true
}

func splitNumberAndUnit(value string) (float32, string, bool) {
	if value == "" {
		return 0, "", false
	}
	position := 0
	if value[position] == '+' || value[position] == '-' {
		position++
		if position == len(value) {
			return 0, "", false
		}
	}
	digits := 0
	for position < len(value) && value[position] >= '0' && value[position] <= '9' {
		position++
		digits++
	}
	if position < len(value) && value[position] == '.' {
		position++
		for position < len(value) && value[position] >= '0' && value[position] <= '9' {
			position++
			digits++
		}
	}
	if digits == 0 {
		return 0, "", false
	}
	if position < len(value) && (value[position] == 'e' || value[position] == 'E') {
		exponent := position
		position++
		if position < len(value) && (value[position] == '+' || value[position] == '-') {
			position++
		}
		exponentDigits := 0
		for position < len(value) && value[position] >= '0' && value[position] <= '9' {
			position++
			exponentDigits++
		}
		if exponentDigits == 0 {
			position = exponent
		}
	}
	number, err := strconv.ParseFloat(value[:position], 32)
	return float32(number), value[position:], err == nil
}

func isCSSWhitespaceByte(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r' || value == '\f'
}
