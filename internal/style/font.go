package style

import (
	"math"
	"strconv"
	"strings"

	"github.com/Grove-Computing/Growse/internal/css"
)

func parseFontFamilies(value string) ([]string, bool) {
	parts, ok := splitTopLevelComma(value)
	if !ok || len(parts) == 0 {
		return nil, false
	}
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if decoded, valid := css.DecodeString(part); valid {
			part = decoded
		}
		if part == "" || strings.ContainsAny(part, "{};()") {
			return nil, false
		}
		result = append(result, part)
	}
	return result, true
}

func splitTopLevelComma(value string) ([]string, bool) {
	var result []string
	start := 0
	var quote byte
	escaped := false
	for position := 0; position < len(value); position++ {
		character := value[position]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == quote {
				quote = 0
			}
			continue
		}
		if character == '\'' || character == '"' {
			quote = character
		} else if character == ',' {
			result = append(result, value[start:position])
			start = position + 1
		}
	}
	if quote != 0 {
		return nil, false
	}
	return append(result, value[start:]), true
}

func parseFontStyle(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "normal" || value == "italic" || value == "oblique" || strings.HasPrefix(value, "oblique ") && strings.HasSuffix(value, "deg") {
		return value, true
	}
	return "", false
}

func parseFontStretch(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	valid := map[string]bool{
		"ultra-condensed": true, "extra-condensed": true, "condensed": true, "semi-condensed": true,
		"normal": true, "semi-expanded": true, "expanded": true, "extra-expanded": true, "ultra-expanded": true,
	}
	return value, valid[value]
}

func selectFontFace(stylesheet *css.Stylesheet, families []string, fontStyle string, weight int, stretch string) int {
	if stylesheet == nil {
		return -1
	}
	for _, family := range families {
		selected, selectedScore := -1, int(^uint(0)>>1)
		for index, face := range stylesheet.FontFaces {
			if !strings.EqualFold(face.Family, family) {
				continue
			}
			faceWeight, valid := parseFontWeight(face.Weight)
			if !valid {
				faceWeight = 400
			}
			score := absInt(faceWeight - weight)
			if face.Style != fontStyle {
				score += 2000
			}
			if face.Stretch != stretch {
				score += 1000
			}
			if score < selectedScore {
				selected, selectedScore = index, score
			}
		}
		if selected >= 0 {
			return selected
		}
	}
	return -1
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func validRegisteredValue(syntax, value string, context LengthContext) bool {
	value = strings.TrimSpace(value)
	switch syntax {
	case "*":
		return value != "" && len(value) <= 64<<10
	case "<length>":
		_, ok := ResolveLength(value, context)
		return ok
	case "<number>":
		number, err := strconv.ParseFloat(value, 64)
		return err == nil && !math.IsNaN(number) && !math.IsInf(number, 0)
	case "<color>":
		_, ok := parseColor(value, defaultTextColor)
		return ok
	default:
		return false
	}
}
