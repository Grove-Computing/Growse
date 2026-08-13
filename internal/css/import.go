package css

import "strings"

func parseImportRule(value string) (ImportRule, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return ImportRule{}, false
	}
	var location, rest string
	if value[0] == '\'' || value[0] == '"' {
		end, ok := quotedEnd(value, 0)
		if !ok {
			return ImportRule{}, false
		}
		var decoded bool
		location, decoded = DecodeString(value[:end+1])
		if !decoded {
			return ImportRule{}, false
		}
		rest = strings.TrimSpace(value[end+1:])
	} else if len(value) >= 4 && strings.EqualFold(value[:4], "url(") {
		end, ok := parenthesisEnd(value, 3)
		if !ok {
			return ImportRule{}, false
		}
		location = strings.TrimSpace(value[4:end])
		if decoded, ok := DecodeString(location); ok {
			location = decoded
		}
		rest = strings.TrimSpace(value[end+1:])
	} else {
		return ImportRule{}, false
	}
	if strings.TrimSpace(location) == "" {
		return ImportRule{}, false
	}
	result := ImportRule{URL: location}
	if rest != "" {
		result.Media = parseMediaQueryList(rest)
		if len(result.Media) == 0 {
			return ImportRule{}, false
		}
	}
	return result, true
}

func quotedEnd(value string, start int) (int, bool) {
	quote := value[start]
	escaped := false
	for position := start + 1; position < len(value); position++ {
		if escaped {
			escaped = false
			continue
		}
		if value[position] == '\\' {
			escaped = true
			continue
		}
		if value[position] == quote {
			return position, true
		}
	}
	return 0, false
}
