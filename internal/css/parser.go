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
			if p.HasParseError() {
				// CSS Syntax errors invalidate the current construct, not the
				// complete stylesheet. The parser has already consumed through
				// the relevant recovery boundary.
				continue
			}
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
			stylesheet.Rules = append(stylesheet.Rules, Rule{
				Kind: RuleStyle, Selectors: selectors, Order: len(stylesheet.Rules),
			})
			current = &stylesheet.Rules[len(stylesheet.Rules)-1]
		case parser.DeclarationGrammar:
			if current == nil {
				continue
			}
			rawValue := strings.TrimSpace(tokenText(p.Values()))
			rawValue, important := stripImportant(rawValue)
			property := strings.ToLower(strings.TrimSpace(string(data)))
			if property != "" && rawValue != "" {
				current.Declarations = append(current.Declarations, Declaration{
					Property: property, Value: parseValue(rawValue), Important: important,
				})
			}
		case parser.EndRulesetGrammar:
			current = nil
		case parser.AtRuleGrammar, parser.BeginAtRuleGrammar, parser.EndAtRuleGrammar:
			// At-rules are ignored until their individual evaluators are
			// implemented. Do not attach declarations to the preceding rule.
			current = nil
		}
	}
}

func parseValue(raw string) Value {
	result := Value{Raw: raw}
	lexer := parser.NewLexer(parse.NewInputString(raw))
	for {
		tokenType, data := lexer.Next()
		if tokenType == parser.ErrorToken {
			break
		}
		result.Components = append(result.Components, ComponentValue{
			Kind: componentKind(tokenType), Raw: string(data),
		})
	}
	return result
}

func componentKind(tokenType parser.TokenType) ComponentKind {
	switch tokenType {
	case parser.IdentToken, parser.CustomPropertyNameToken, parser.CustomPropertyValueToken:
		return ComponentIdentifier
	case parser.FunctionToken:
		return ComponentFunction
	case parser.AtKeywordToken:
		return ComponentAtKeyword
	case parser.HashToken:
		return ComponentHash
	case parser.StringToken:
		return ComponentString
	case parser.URLToken:
		return ComponentURL
	case parser.NumberToken:
		return ComponentNumber
	case parser.PercentageToken:
		return ComponentPercentage
	case parser.DimensionToken:
		return ComponentDimension
	case parser.WhitespaceToken, parser.CommentToken:
		return ComponentWhitespace
	case parser.CommaToken:
		return ComponentComma
	case parser.LeftBracketToken, parser.LeftParenthesisToken, parser.LeftBraceToken:
		return ComponentBlockStart
	case parser.RightBracketToken, parser.RightParenthesisToken, parser.RightBraceToken:
		return ComponentBlockEnd
	case parser.BadStringToken, parser.BadURLToken:
		return ComponentBad
	default:
		return ComponentDelimiter
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
	hover := strings.HasSuffix(value, ":hover")
	if hover {
		value = strings.TrimSuffix(value, ":hover")
	}
	if value == "" || strings.ContainsAny(value, " >+~[:") {
		return Selector{}, false
	}

	compound := CompoundSelector{Hover: hover}
	position := 0
	if value[0] == '*' {
		compound.Universal = true
		position++
	} else if value[0] != '.' && value[0] != '#' {
		end := selectorNameEnd(value, position)
		if end == position || !validName(value[position:end]) {
			return Selector{}, false
		}
		compound.Type = strings.ToLower(value[position:end])
		position = end
	}
	for position < len(value) {
		prefix := value[position]
		if prefix != '.' && prefix != '#' {
			return Selector{}, false
		}
		position++
		end := selectorNameEnd(value, position)
		if end == position || !validName(value[position:end]) {
			return Selector{}, false
		}
		name := value[position:end]
		if prefix == '.' {
			compound.Classes = append(compound.Classes, name)
		} else {
			compound.IDs = append(compound.IDs, name)
		}
		position = end
	}
	if !compound.Universal && compound.Type == "" && len(compound.IDs) == 0 && len(compound.Classes) == 0 {
		return Selector{}, false
	}

	selector := Selector{Kind: SelectorCompound, Hover: hover, Compounds: []CompoundSelector{compound}}
	if compound.Universal && compound.Type == "" && len(compound.IDs) == 0 && len(compound.Classes) == 0 {
		selector.Kind = SelectorUniversal
	}
	if !compound.Universal && len(compound.IDs) == 0 && len(compound.Classes) == 0 {
		selector.Kind, selector.Tag = SelectorTag, compound.Type
	} else if !compound.Universal && compound.Type == "" && len(compound.IDs) == 0 && len(compound.Classes) == 1 {
		selector.Kind, selector.Class = SelectorClass, compound.Classes[0]
	} else if !compound.Universal && compound.Type == "" && len(compound.IDs) == 1 && len(compound.Classes) == 0 {
		selector.Kind, selector.ID = SelectorID, compound.IDs[0]
	} else if !compound.Universal && compound.Type != "" && len(compound.IDs) == 0 && len(compound.Classes) == 1 {
		selector.Kind, selector.Tag, selector.Class = SelectorTagClass, compound.Type, compound.Classes[0]
	}
	return selector, true
}

func selectorNameEnd(value string, start int) int {
	position := start
	for position < len(value) && value[position] != '.' && value[position] != '#' {
		position++
	}
	return position
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
