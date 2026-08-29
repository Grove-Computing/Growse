package css

import "strings"

func parseMediaQueryList(value string) []MediaQuery {
	parts, ok := splitSelectorList(value)
	if !ok {
		return nil
	}
	queries := make([]MediaQuery, 0, len(parts))
	for _, part := range parts {
		if query, ok := parseMediaQuery(strings.TrimSpace(part)); ok {
			queries = append(queries, query)
		}
	}
	return queries
}

// ParseMediaQueryList parses the subset shared by author CSS and matchMedia.
func ParseMediaQueryList(value string) []MediaQuery { return parseMediaQueryList(value) }

func parseMediaQuery(value string) (MediaQuery, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return MediaQuery{}, false
	}
	query := MediaQuery{Type: "all"}
	if word, rest := takeMediaWord(value); word == "not" || word == "only" {
		if word == "not" {
			query.Modifier = MediaModifierNot
		} else {
			query.Modifier = MediaModifierOnly
		}
		value = strings.TrimSpace(rest)
	}
	if !strings.HasPrefix(value, "(") {
		mediaType, rest := takeMediaWord(value)
		if mediaType == "" || !validName(mediaType) {
			return MediaQuery{}, false
		}
		query.Type = mediaType
		value = strings.TrimSpace(rest)
	}
	for value != "" {
		if len(query.Features) != 0 || query.Type != "all" || query.Modifier != MediaModifierNone {
			word, rest := takeMediaWord(value)
			if word != "and" {
				return MediaQuery{}, false
			}
			value = strings.TrimSpace(rest)
		}
		if !strings.HasPrefix(value, "(") {
			return MediaQuery{}, false
		}
		end, ok := parenthesisEnd(value, 0)
		if !ok {
			return MediaQuery{}, false
		}
		feature, ok := parseMediaFeature(strings.TrimSpace(value[1:end]))
		if !ok {
			return MediaQuery{}, false
		}
		query.Features = append(query.Features, feature)
		value = strings.TrimSpace(value[end+1:])
	}
	return query, true
}

func parseMediaFeature(value string) (MediaFeature, bool) {
	if name, featureValue, found := strings.Cut(value, ":"); found {
		name, featureValue = strings.TrimSpace(name), strings.TrimSpace(featureValue)
		if !validName(name) || featureValue == "" {
			return MediaFeature{}, false
		}
		return MediaFeature{Name: name, Value: featureValue}, true
	}
	for _, comparator := range []string{"<=", ">=", "<", ">", "="} {
		position := strings.Index(value, comparator)
		if position < 0 {
			continue
		}
		left := strings.TrimSpace(value[:position])
		right := strings.TrimSpace(value[position+len(comparator):])
		if left == "" || right == "" || strings.Contains(right, comparator) {
			return MediaFeature{}, false
		}
		if isRangeMediaFeature(left) {
			return MediaFeature{Name: left, Value: right, Comparator: comparator}, true
		}
		if isRangeMediaFeature(right) {
			return MediaFeature{Name: right, Value: left, Comparator: reverseComparator(comparator)}, true
		}
		return MediaFeature{}, false
	}
	if !validName(value) {
		return MediaFeature{}, false
	}
	return MediaFeature{Name: value}, true
}

func isRangeMediaFeature(value string) bool {
	switch strings.ToLower(value) {
	case "width", "height", "resolution":
		return true
	default:
		return false
	}
}

func reverseComparator(comparator string) string {
	switch comparator {
	case "<":
		return ">"
	case "<=":
		return ">="
	case ">":
		return "<"
	case ">=":
		return "<="
	default:
		return comparator
	}
}

func takeMediaWord(value string) (string, string) {
	position := 0
	for position < len(value) && !isSelectorWhitespace(value[position]) && value[position] != '(' {
		position++
	}
	return value[:position], value[position:]
}
