package css

import "strings"

const maxSupportsDepth = 32

func parseSupportsCondition(value string) (SupportsCondition, bool) {
	return parseSupportsConditionDepth(strings.TrimSpace(value), 0)
}

func parseSupportsConditionDepth(value string, depth int) (SupportsCondition, bool) {
	if value == "" || depth >= maxSupportsDepth {
		return SupportsCondition{}, false
	}
	if parts, ok := splitSupportsBoolean(value, "or"); ok {
		children := make([]SupportsCondition, 0, len(parts))
		for _, part := range parts {
			child, valid := parseSupportsConditionDepth(part, depth+1)
			if !valid {
				return SupportsCondition{}, false
			}
			children = append(children, child)
		}
		return SupportsCondition{Kind: SupportsOr, Children: children}, true
	}
	if parts, ok := splitSupportsBoolean(value, "and"); ok {
		children := make([]SupportsCondition, 0, len(parts))
		for _, part := range parts {
			child, valid := parseSupportsConditionDepth(part, depth+1)
			if !valid {
				return SupportsCondition{}, false
			}
			children = append(children, child)
		}
		return SupportsCondition{Kind: SupportsAnd, Children: children}, true
	}
	if hasSupportsKeywordPrefix(value, "not") {
		child, ok := parseSupportsConditionDepth(strings.TrimSpace(value[len("not"):]), depth+1)
		if !ok {
			return SupportsCondition{}, false
		}
		return SupportsCondition{Kind: SupportsNot, Children: []SupportsCondition{child}}, true
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "selector(") {
		end, ok := parenthesisEnd(value, len("selector"))
		if !ok || strings.TrimSpace(value[end+1:]) != "" {
			return SupportsCondition{}, false
		}
		selectors := parseSelectorList(strings.TrimSpace(value[len("selector("):end]))
		if len(selectors) == 0 {
			return SupportsCondition{}, false
		}
		return SupportsCondition{Kind: SupportsSelector, Selectors: selectors}, true
	}
	if inner, ok := unwrapSupportsParentheses(value); ok {
		if property, raw, declaration := splitSupportsDeclaration(inner); declaration {
			return SupportsCondition{Kind: SupportsDeclaration, Property: property, Value: raw}, true
		}
		return parseSupportsConditionDepth(inner, depth+1)
	}
	return SupportsCondition{}, false
}

func splitSupportsBoolean(value, keyword string) ([]string, bool) {
	var parts []string
	start, depth := 0, 0
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
		switch character {
		case '\'', '"':
			quote = character
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth != 0 || position+len(keyword) > len(value) || !strings.EqualFold(value[position:position+len(keyword)], keyword) {
			continue
		}
		beforeSpace := position > 0 && isSelectorWhitespace(value[position-1])
		after := position + len(keyword)
		afterSpace := after < len(value) && isSelectorWhitespace(value[after])
		if !beforeSpace || !afterSpace {
			continue
		}
		parts = append(parts, strings.TrimSpace(value[start:position]))
		start = after
		position = after - 1
	}
	if len(parts) == 0 {
		return nil, false
	}
	parts = append(parts, strings.TrimSpace(value[start:]))
	for _, part := range parts {
		if part == "" {
			return nil, false
		}
	}
	return parts, true
}

func hasSupportsKeywordPrefix(value, keyword string) bool {
	return len(value) > len(keyword) && strings.EqualFold(value[:len(keyword)], keyword) && isSelectorWhitespace(value[len(keyword)])
}

func unwrapSupportsParentheses(value string) (string, bool) {
	if len(value) < 2 || value[0] != '(' {
		return "", false
	}
	end, ok := parenthesisEnd(value, 0)
	if !ok || end != len(value)-1 {
		return "", false
	}
	return strings.TrimSpace(value[1:end]), true
}

func splitSupportsDeclaration(value string) (string, string, bool) {
	depth := 0
	for position := 0; position < len(value); position++ {
		switch value[position] {
		case '(':
			depth++
		case ')':
			depth--
		case ':':
			if depth == 0 {
				property := strings.ToLower(strings.TrimSpace(value[:position]))
				raw := strings.TrimSpace(value[position+1:])
				if property == "" || raw == "" || (!strings.HasPrefix(property, "--") && !validName(property)) {
					return "", "", false
				}
				return property, raw, true
			}
		}
	}
	return "", "", false
}
