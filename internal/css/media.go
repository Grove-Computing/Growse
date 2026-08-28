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
		content := strings.TrimSpace(value[1:end])
		name, featureValue, found := strings.Cut(content, ":")
		name = strings.TrimSpace(name)
		featureValue = strings.TrimSpace(featureValue)
		if !validName(name) || found && featureValue == "" {
			return MediaQuery{}, false
		}
		query.Features = append(query.Features, MediaFeature{Name: name, Value: featureValue})
		value = strings.TrimSpace(value[end+1:])
	}
	return query, true
}

func takeMediaWord(value string) (string, string) {
	position := 0
	for position < len(value) && !isSelectorWhitespace(value[position]) && value[position] != '(' {
		position++
	}
	return value[:position], value[position:]
}
