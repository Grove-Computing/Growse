package css

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// DecodeString decodes one quoted CSS string, including hexadecimal escapes.
func DecodeString(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) < 2 || (value[0] != '\'' && value[0] != '"') || value[len(value)-1] != value[0] {
		return "", false
	}
	content := value[1 : len(value)-1]
	var result strings.Builder
	for position := 0; position < len(content); {
		if content[position] != '\\' {
			r, size := utf8.DecodeRuneInString(content[position:])
			result.WriteRune(r)
			position += size
			continue
		}
		position++
		if position >= len(content) {
			return "", false
		}
		if content[position] == '\n' {
			position++
			continue
		}
		if content[position] == '\r' {
			position++
			if position < len(content) && content[position] == '\n' {
				position++
			}
			continue
		}
		start := position
		for position < len(content) && position-start < 6 && isHexDigit(content[position]) {
			position++
		}
		if position > start {
			codePoint, err := strconv.ParseInt(content[start:position], 16, 32)
			if err != nil || codePoint == 0 || codePoint > utf8.MaxRune || codePoint >= 0xd800 && codePoint <= 0xdfff {
				result.WriteRune(utf8.RuneError)
			} else {
				result.WriteRune(rune(codePoint))
			}
			if position < len(content) && isCSSWhitespace(content[position]) {
				position++
			}
			continue
		}
		r, size := utf8.DecodeRuneInString(content[position:])
		result.WriteRune(r)
		position += size
	}
	return result.String(), true
}

func isHexDigit(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f' || value >= 'A' && value <= 'F'
}

func isCSSWhitespace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r' || value == '\f'
}
