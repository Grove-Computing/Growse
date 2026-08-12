package css

import (
	"errors"
	"fmt"
	"io"
	"strings"

	parse "github.com/tdewolff/parse/v2"
	parser "github.com/tdewolff/parse/v2/css"
)

// Parse reads a stylesheet. Unsupported selectors and at-rules are ignored.
func Parse(reader io.Reader) (*Stylesheet, error) {
	input, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read CSS: %w", err)
	}

	p := parser.NewParser(parse.NewInputBytes(input), false)
	stylesheet := &Stylesheet{}
	var current *Rule

	for {
		grammar, _, data := p.Next()
		switch grammar {
		case parser.ErrorGrammar:
			if err := p.Err(); err != nil && !errors.Is(err, io.EOF) {
				return nil, fmt.Errorf("parse CSS: %w", err)
			}
			return stylesheet, nil
		case parser.BeginRulesetGrammar:
			selectorText := string(data) + tokenText(p.Values())
			selectors := parseSelectorList(selectorText)
			if len(selectors) == 0 {
				current = nil
				continue
			}
			stylesheet.Rules = append(stylesheet.Rules, Rule{Selectors: selectors, Order: len(stylesheet.Rules)})
			current = &stylesheet.Rules[len(stylesheet.Rules)-1]
		case parser.DeclarationGrammar:
			if current == nil {
				continue
			}
			value := strings.TrimSpace(tokenText(p.Values()))
			value, important := stripImportant(value)
			property := strings.ToLower(strings.TrimSpace(string(data)))
			if property != "" && value != "" {
				current.Declarations = append(current.Declarations, Declaration{
					Property: property, Value: value, Important: important,
				})
			}
		case parser.EndRulesetGrammar:
			current = nil
		}
	}
}

// Append adds another stylesheet while preserving global source order.
func (s *Stylesheet) Append(other *Stylesheet) {
	if other == nil {
		return
	}
	for _, rule := range other.Rules {
		rule.Order = len(s.Rules)
		s.Rules = append(s.Rules, rule)
	}
}

func tokenText(tokens []parser.Token) string {
	var builder strings.Builder
	for _, token := range tokens {
		builder.Write(token.Data)
	}
	return builder.String()
}

func parseSelectorList(value string) []Selector {
	parts := strings.Split(value, ",")
	selectors := make([]Selector, 0, len(parts))
	for _, part := range parts {
		if selector, ok := parseSelector(strings.TrimSpace(part)); ok {
			selectors = append(selectors, selector)
		}
	}
	return selectors
}

func parseSelector(value string) (Selector, bool) {
	if value == "" || strings.ContainsAny(value, " >+~[:*") {
		return Selector{}, false
	}
	if strings.HasPrefix(value, "#") && validName(value[1:]) {
		return Selector{Kind: SelectorID, ID: value[1:]}, true
	}
	if strings.HasPrefix(value, ".") && validName(value[1:]) {
		return Selector{Kind: SelectorClass, Class: value[1:]}, true
	}
	if index := strings.IndexByte(value, '.'); index > 0 && strings.Count(value, ".") == 1 {
		tag, class := strings.ToLower(value[:index]), value[index+1:]
		if validName(tag) && validName(class) {
			return Selector{Kind: SelectorTagClass, Tag: tag, Class: class}, true
		}
		return Selector{}, false
	}
	if validName(value) {
		return Selector{Kind: SelectorTag, Tag: strings.ToLower(value)}, true
	}
	return Selector{}, false
}

func validName(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character != '-' && character != '_' &&
			(character < 'a' || character > 'z') &&
			(character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func stripImportant(value string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(value))
	if strings.HasSuffix(lower, "!important") {
		return strings.TrimSpace(value[:len(value)-len("!important")]), true
	}
	return value, false
}
