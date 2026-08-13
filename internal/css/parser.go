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
	parts, ok := splitSelectorList(value)
	if !ok {
		return nil
	}
	selectors := make([]Selector, 0, len(parts))
	for _, part := range parts {
		if selector, ok := parseSelector(strings.TrimSpace(part)); ok {
			selectors = append(selectors, selector)
		}
	}
	return selectors
}

func parseSelector(value string) (Selector, bool) {
	parts, combinators, ok := splitComplexSelector(value)
	if !ok {
		return Selector{}, false
	}
	compounds := make([]CompoundSelector, 0, len(parts))
	for _, part := range parts {
		compound, ok := parseCompoundSelector(part)
		if !ok {
			return Selector{}, false
		}
		compounds = append(compounds, compound)
	}
	selector := Selector{Kind: SelectorCompound, Compounds: compounds, Combinators: combinators}
	if len(compounds) != 1 {
		return selector, true
	}
	compound := compounds[0]
	selector.Hover = compound.Hover
	if compound.Universal && compound.Type == "" && len(compound.IDs) == 0 && len(compound.Classes) == 0 && len(compound.Attributes) == 0 {
		selector.Kind = SelectorUniversal
	}
	if !compound.Universal && len(compound.IDs) == 0 && len(compound.Classes) == 0 && len(compound.Attributes) == 0 {
		selector.Kind, selector.Tag = SelectorTag, compound.Type
	} else if !compound.Universal && compound.Type == "" && len(compound.IDs) == 0 && len(compound.Classes) == 1 && len(compound.Attributes) == 0 {
		selector.Kind, selector.Class = SelectorClass, compound.Classes[0]
	} else if !compound.Universal && compound.Type == "" && len(compound.IDs) == 1 && len(compound.Classes) == 0 && len(compound.Attributes) == 0 {
		selector.Kind, selector.ID = SelectorID, compound.IDs[0]
	} else if !compound.Universal && compound.Type != "" && len(compound.IDs) == 0 && len(compound.Classes) == 1 && len(compound.Attributes) == 0 {
		selector.Kind, selector.Tag, selector.Class = SelectorTagClass, compound.Type, compound.Classes[0]
	}
	return selector, true
}

func parseCompoundSelector(value string) (CompoundSelector, bool) {
	hover := strings.HasSuffix(value, ":hover")
	if hover {
		value = strings.TrimSuffix(value, ":hover")
	}
	if value == "" {
		return CompoundSelector{}, false
	}

	compound := CompoundSelector{Hover: hover}
	position := 0
	if value[0] == '*' {
		compound.Universal = true
		position++
	} else if value[0] != '.' && value[0] != '#' && value[0] != '[' {
		end := selectorNameEnd(value, position)
		if end == position || !validName(value[position:end]) {
			return CompoundSelector{}, false
		}
		compound.Type = strings.ToLower(value[position:end])
		position = end
	}
	for position < len(value) {
		prefix := value[position]
		switch prefix {
		case '.', '#':
			position++
			end := selectorNameEnd(value, position)
			if end == position || !validName(value[position:end]) {
				return CompoundSelector{}, false
			}
			name := value[position:end]
			if prefix == '.' {
				compound.Classes = append(compound.Classes, name)
			} else {
				compound.IDs = append(compound.IDs, name)
			}
			position = end
		case '[':
			end, ok := attributeEnd(value, position)
			if !ok {
				return CompoundSelector{}, false
			}
			attribute, ok := parseAttributeSelector(value[position+1 : end])
			if !ok {
				return CompoundSelector{}, false
			}
			compound.Attributes = append(compound.Attributes, attribute)
			position = end + 1
		default:
			return CompoundSelector{}, false
		}
	}
	if !compound.Universal && compound.Type == "" && len(compound.IDs) == 0 && len(compound.Classes) == 0 && len(compound.Attributes) == 0 {
		return CompoundSelector{}, false
	}
	return compound, true
}

func splitComplexSelector(value string) ([]string, []Combinator, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil, false
	}
	var parts []string
	var combinators []Combinator
	start, bracketDepth, parenthesisDepth := 0, 0, 0
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
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		case '(':
			parenthesisDepth++
		case ')':
			parenthesisDepth--
		}
		if bracketDepth < 0 || parenthesisDepth < 0 {
			return nil, nil, false
		}
		if bracketDepth != 0 || parenthesisDepth != 0 {
			continue
		}
		if isSelectorWhitespace(character) {
			part := strings.TrimSpace(value[start:position])
			if part == "" {
				return nil, nil, false
			}
			parts = append(parts, part)
			for position+1 < len(value) && isSelectorWhitespace(value[position+1]) {
				position++
			}
			if position+1 >= len(value) {
				return nil, nil, false
			}
			if combinator, ok := explicitCombinator(value[position+1]); ok {
				combinators = append(combinators, combinator)
				position++
				for position+1 < len(value) && isSelectorWhitespace(value[position+1]) {
					position++
				}
			} else {
				combinators = append(combinators, CombinatorDescendant)
			}
			start = position + 1
			continue
		}
		if combinator, ok := explicitCombinator(character); ok {
			part := strings.TrimSpace(value[start:position])
			if part == "" {
				return nil, nil, false
			}
			parts = append(parts, part)
			combinators = append(combinators, combinator)
			for position+1 < len(value) && isSelectorWhitespace(value[position+1]) {
				position++
			}
			start = position + 1
		}
	}
	if quote != 0 || bracketDepth != 0 || parenthesisDepth != 0 {
		return nil, nil, false
	}
	last := strings.TrimSpace(value[start:])
	if last == "" {
		return nil, nil, false
	}
	parts = append(parts, last)
	if len(parts) != len(combinators)+1 {
		return nil, nil, false
	}
	return parts, combinators, true
}

func isSelectorWhitespace(character byte) bool {
	return character == ' ' || character == '\t' || character == '\n' || character == '\r' || character == '\f'
}

func explicitCombinator(character byte) (Combinator, bool) {
	switch character {
	case '>':
		return CombinatorChild, true
	case '+':
		return CombinatorAdjacentSibling, true
	case '~':
		return CombinatorGeneralSibling, true
	default:
		return 0, false
	}
}

func selectorNameEnd(value string, start int) int {
	position := start
	for position < len(value) && value[position] != '.' && value[position] != '#' && value[position] != '[' {
		position++
	}
	return position
}

func splitSelectorList(value string) ([]string, bool) {
	var parts []string
	start, bracketDepth, parenthesisDepth := 0, 0, 0
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
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		case '(':
			parenthesisDepth++
		case ')':
			parenthesisDepth--
		case ',':
			if bracketDepth == 0 && parenthesisDepth == 0 {
				parts = append(parts, value[start:position])
				start = position + 1
			}
		}
		if bracketDepth < 0 || parenthesisDepth < 0 {
			return nil, false
		}
	}
	if quote != 0 || bracketDepth != 0 || parenthesisDepth != 0 {
		return nil, false
	}
	return append(parts, value[start:]), true
}

func attributeEnd(value string, start int) (int, bool) {
	var quote byte
	escaped := false
	for position := start + 1; position < len(value); position++ {
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
		case ']':
			return position, true
		}
	}
	return 0, false
}

func parseAttributeSelector(value string) (AttributeSelector, bool) {
	type attributeToken struct {
		kind parser.TokenType
		raw  string
	}
	var tokens []attributeToken
	lexer := parser.NewLexer(parse.NewInputString(value))
	for {
		kind, data := lexer.Next()
		if kind == parser.ErrorToken {
			break
		}
		if kind != parser.WhitespaceToken && kind != parser.CommentToken {
			tokens = append(tokens, attributeToken{kind: kind, raw: string(data)})
		}
	}
	if len(tokens) == 0 || tokens[0].kind != parser.IdentToken {
		return AttributeSelector{}, false
	}
	attribute := AttributeSelector{Name: strings.ToLower(tokens[0].raw), Matcher: AttributePresent}
	if len(tokens) == 1 {
		return attribute, true
	}
	if len(tokens) != 3 || (tokens[2].kind != parser.IdentToken && tokens[2].kind != parser.StringToken) {
		return AttributeSelector{}, false
	}
	switch {
	case tokens[1].kind == parser.DelimToken && tokens[1].raw == "=":
		attribute.Matcher = AttributeExact
	case tokens[1].kind == parser.IncludeMatchToken:
		attribute.Matcher = AttributeIncludes
	case tokens[1].kind == parser.DashMatchToken:
		attribute.Matcher = AttributeDashMatch
	case tokens[1].kind == parser.PrefixMatchToken:
		attribute.Matcher = AttributePrefix
	case tokens[1].kind == parser.SuffixMatchToken:
		attribute.Matcher = AttributeSuffix
	case tokens[1].kind == parser.SubstringMatchToken:
		attribute.Matcher = AttributeSubstring
	default:
		return AttributeSelector{}, false
	}
	attribute.Value = unquoteCSSString(tokens[2].raw)
	return attribute, true
}

func unquoteCSSString(value string) string {
	if len(value) >= 2 && (value[0] == '\'' || value[0] == '"') && value[len(value)-1] == value[0] {
		return value[1 : len(value)-1]
	}
	return value
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
