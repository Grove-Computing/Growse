package css

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	parse "github.com/tdewolff/parse/v2"
	parser "github.com/tdewolff/parse/v2/css"
)

type atRuleFrame struct {
	active         bool
	isMedia        bool
	media          []MediaQuery
	keyframesIndex int
	layer          string
	supports       *SupportsCondition
	container      *ContainerQuery
	fontFaceIndex  int
	propertyIndex  int
}

type rulesetFrame struct {
	ruleIndex int
	selectors []string
}

const (
	maxNestingDepth            = 32
	maxExpandedNestedSelectors = 1024
)

// Parse reads a stylesheet. Unsupported selectors and at-rules are ignored.
func Parse(reader io.Reader) (*Stylesheet, error) {
	input, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read CSS: %w", err)
	}
	input = rewritePropertyAtRules(input)
	input = rewriteContainerAtRules(input)

	p := parser.NewParser(parse.NewInputBytes(input), false)
	stylesheet := &Stylesheet{}
	var current *Rule
	var currentKeyframe *Keyframe
	var atRules []atRuleFrame
	var rulesets []rulesetFrame
	importsAllowed := true

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
			finalizeStylesheetMetadata(stylesheet)
			return stylesheet, nil
		case parser.BeginRulesetGrammar:
			if len(atRules) == 0 {
				importsAllowed = false
			}
			if len(atRules) != 0 && atRules[len(atRules)-1].keyframesIndex >= 0 {
				frame := atRules[len(atRules)-1]
				offsets, valid := parseKeyframeSelectors(string(data) + tokenText(p.Values()))
				if !valid {
					current, currentKeyframe = nil, nil
					continue
				}
				keyframes := &stylesheet.Keyframes[frame.keyframesIndex]
				if len(keyframes.Frames) >= MaxFramesPerKeyframesRule {
					current, currentKeyframe = nil, nil
					continue
				}
				keyframes.Frames = append(keyframes.Frames, Keyframe{Offsets: offsets})
				current, currentKeyframe = nil, &keyframes.Frames[len(keyframes.Frames)-1]
				continue
			}
			active := true
			var mediaGroups [][]MediaQuery
			var supports []SupportsCondition
			var containers []ContainerQuery
			layer := ""
			for _, frame := range atRules {
				if !frame.active {
					active = false
					break
				}
				if frame.isMedia {
					mediaGroups = append(mediaGroups, append([]MediaQuery(nil), frame.media...))
				}
				if frame.layer != "" {
					layer = frame.layer
				}
				if frame.supports != nil {
					supports = append(supports, *frame.supports)
				}
				if frame.container != nil {
					containers = append(containers, *frame.container)
				}
			}
			if !active {
				current = nil
				continue
			}
			selectorText := string(data) + tokenText(p.Values())
			selectorTexts, validList := splitSelectorList(selectorText)
			if len(selectorTexts) > MaxSelectorsPerRule {
				validList = false
			}
			if len(rulesets) != 0 && validList {
				if len(rulesets) >= maxNestingDepth || rulesets[len(rulesets)-1].ruleIndex < 0 {
					validList = false
				} else {
					selectorTexts, validList = expandNestedSelectors(rulesets[len(rulesets)-1].selectors, selectorTexts)
				}
			}
			selectors := []Selector(nil)
			if validList {
				selectors = parseSelectorTexts(selectorTexts, maxExpandedNestedSelectors)
			}
			if len(selectors) == 0 || len(stylesheet.Rules) >= MaxStylesheetRules {
				current = nil
				rulesets = append(rulesets, rulesetFrame{ruleIndex: -1})
				continue
			}
			stylesheet.Rules = append(stylesheet.Rules, Rule{
				Kind: RuleStyle, Selectors: selectors, Order: len(stylesheet.Rules), Media: mediaGroups, Supports: supports, Containers: containers, Layer: layer,
			})
			current = &stylesheet.Rules[len(stylesheet.Rules)-1]
			rulesets = append(rulesets, rulesetFrame{ruleIndex: len(stylesheet.Rules) - 1, selectors: selectorTexts})
		case parser.DeclarationGrammar, parser.CustomPropertyGrammar:
			if len(atRules) != 0 {
				frame := &atRules[len(atRules)-1]
				property := strings.ToLower(strings.TrimSpace(string(data)))
				value := strings.TrimSpace(tokenText(p.Values()))
				if frame.fontFaceIndex >= 0 {
					if property == "growse-property-name" && strings.HasPrefix(value, "--") && validName(strings.TrimPrefix(value, "--")) {
						stylesheet.FontFaces = stylesheet.FontFaces[:frame.fontFaceIndex]
						if len(stylesheet.Properties) >= MaxCustomProperties {
							frame.fontFaceIndex, frame.propertyIndex = -1, -1
							continue
						}
						stylesheet.Properties = append(stylesheet.Properties, PropertyRule{Name: value})
						frame.fontFaceIndex = -1
						frame.propertyIndex = len(stylesheet.Properties) - 1
						continue
					}
					applyFontFaceDescriptor(&stylesheet.FontFaces[frame.fontFaceIndex], property, value)
					continue
				}
				if frame.propertyIndex >= 0 {
					applyPropertyDescriptor(&stylesheet.Properties[frame.propertyIndex], property, value)
					continue
				}
			}
			if current == nil && currentKeyframe == nil {
				continue
			}
			rawValue := strings.TrimSpace(tokenText(p.Values()))
			rawValue, important := stripImportant(rawValue)
			property := strings.TrimSpace(string(data))
			if !strings.HasPrefix(property, "--") {
				property = strings.ToLower(property)
			}
			if property != "" && rawValue != "" && functionDepthWithin(rawValue, MaxCSSFunctionDepth) &&
				(!strings.HasPrefix(property, "--") || len(rawValue) <= MaxCustomPropertyValueBytes) {
				declaration := Declaration{
					Property: property, Value: parseValue(rawValue), Important: important,
				}
				if currentKeyframe != nil {
					if len(currentKeyframe.Declarations) < MaxDeclarationsPerKeyframe {
						currentKeyframe.Declarations = append(currentKeyframe.Declarations, declaration)
					}
				} else {
					current.Declarations = append(current.Declarations, declaration)
				}
			}
		case parser.EndRulesetGrammar:
			if currentKeyframe != nil {
				currentKeyframe = nil
				continue
			}
			if len(rulesets) != 0 {
				rulesets = rulesets[:len(rulesets)-1]
			}
			current = currentRule(stylesheet, rulesets)
		case parser.AtRuleGrammar:
			name := strings.ToLower(strings.TrimSpace(string(data)))
			if len(atRules) == 0 && name == "@import" && importsAllowed {
				if importRule, ok := parseImportRule(tokenText(p.Values())); ok {
					if importRule.Layered && importRule.Layer == "" {
						importRule.Layer = newAnonymousLayer()
					}
					if importRule.Layered {
						if !stylesheet.ensureLayer(importRule.Layer) {
							current = nil
							continue
						}
					}
					stylesheet.Imports = append(stylesheet.Imports, importRule)
				}
			} else if name == "@layer" {
				parent := currentLayer(atRules)
				for _, layerName := range strings.Split(tokenText(p.Values()), ",") {
					layerName = strings.TrimSpace(layerName)
					if validLayerName(layerName) {
						stylesheet.ensureLayer(joinLayer(parent, layerName))
					}
				}
			} else if len(atRules) == 0 && name != "@charset" {
				importsAllowed = false
			}
			current = nil
		case parser.BeginAtRuleGrammar:
			name := strings.ToLower(strings.TrimSpace(string(data)))
			if len(atRules) == 0 {
				importsAllowed = false
			}
			frame := atRuleFrame{active: false, keyframesIndex: -1, fontFaceIndex: -1, propertyIndex: -1}
			if name == "@media" {
				frame.active, frame.isMedia = true, true
				frame.media = parseMediaQueryList(tokenText(p.Values()))
			} else if name == "@supports" {
				prelude := strings.TrimSpace(tokenText(p.Values()))
				if containerPrelude, ok := unwrapContainerPrelude(prelude); ok {
					query := parseContainerQuery(containerPrelude)
					frame.active = true
					frame.container = &query
				} else {
					condition, valid := parseSupportsCondition(prelude)
					if !valid {
						condition = SupportsCondition{Kind: SupportsUnknown}
					}
					frame.active = true
					frame.supports = &condition
				}
			} else if name == "@layer" {
				parent := currentLayer(atRules)
				layerName := strings.TrimSpace(tokenText(p.Values()))
				if layerName == "" {
					layerName = newAnonymousLayer()
				}
				if validLayerName(layerName) || strings.HasPrefix(layerName, "\x00anonymous-") {
					frame.layer = joinLayer(parent, layerName)
					frame.active = stylesheet.ensureLayer(frame.layer)
				}
			} else if len(atRules) == 0 && name == "@keyframes" {
				if keyframesName, valid := parseKeyframesName(tokenText(p.Values())); valid && len(stylesheet.Keyframes) < MaxKeyframesRules {
					stylesheet.Keyframes = append(stylesheet.Keyframes, KeyframesRule{Name: keyframesName})
					frame.active = true
					frame.keyframesIndex = len(stylesheet.Keyframes) - 1
				}
			} else if len(atRules) == 0 && name == "@font-face" && strings.TrimSpace(tokenText(p.Values())) == "" {
				stylesheet.FontFaces = append(stylesheet.FontFaces, FontFaceRule{})
				frame.fontFaceIndex = len(stylesheet.FontFaces) - 1
			} else if len(atRules) == 0 && name == "@property" && len(stylesheet.Properties) < MaxCustomProperties {
				propertyName := strings.TrimSpace(tokenText(p.Values()))
				if strings.HasPrefix(propertyName, "--") && validName(strings.TrimPrefix(propertyName, "--")) {
					stylesheet.Properties = append(stylesheet.Properties, PropertyRule{Name: propertyName})
					frame.propertyIndex = len(stylesheet.Properties) - 1
				}
			}
			atRules = append(atRules, frame)
			current, currentKeyframe = nil, nil
		case parser.EndAtRuleGrammar:
			if len(atRules) != 0 {
				atRules = atRules[:len(atRules)-1]
			}
			current, currentKeyframe = currentRule(stylesheet, rulesets), nil
		}
	}
}

func currentRule(stylesheet *Stylesheet, rulesets []rulesetFrame) *Rule {
	if stylesheet == nil || len(rulesets) == 0 {
		return nil
	}
	index := rulesets[len(rulesets)-1].ruleIndex
	if index < 0 || index >= len(stylesheet.Rules) {
		return nil
	}
	return &stylesheet.Rules[index]
}

func expandNestedSelectors(parents, nested []string) ([]string, bool) {
	if len(parents) == 0 || len(nested) == 0 || len(parents) > maxExpandedNestedSelectors/len(nested) {
		return nil, false
	}
	result := make([]string, 0, len(parents)*len(nested))
	for _, parent := range parents {
		for _, child := range nested {
			child = strings.TrimSpace(child)
			if child == "" {
				return nil, false
			}
			expanded, found, valid := replaceNestingSelector(child, strings.TrimSpace(parent))
			if !valid {
				return nil, false
			}
			if !found {
				expanded = strings.TrimSpace(parent) + " " + child
			}
			result = append(result, expanded)
		}
	}
	return result, len(result) <= maxExpandedNestedSelectors
}

func replaceNestingSelector(value, parent string) (string, bool, bool) {
	var result strings.Builder
	found := false
	var quote byte
	escaped, bracketDepth := false, 0
	for position := 0; position < len(value); position++ {
		character := value[position]
		if quote != 0 {
			result.WriteByte(character)
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
			result.WriteByte(character)
		case '[':
			bracketDepth++
			result.WriteByte(character)
		case ']':
			bracketDepth--
			result.WriteByte(character)
		case '&':
			if bracketDepth == 0 {
				result.WriteString(parent)
				found = true
			} else {
				result.WriteByte(character)
			}
		default:
			result.WriteByte(character)
		}
		if bracketDepth < 0 {
			return "", false, false
		}
	}
	return result.String(), found, quote == 0 && bracketDepth == 0
}

func currentLayer(frames []atRuleFrame) string {
	for index := len(frames) - 1; index >= 0; index-- {
		if frames[index].layer != "" {
			return frames[index].layer
		}
	}
	return ""
}

func parseKeyframesName(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if decoded, ok := DecodeString(value); ok {
		value = decoded
	}
	if value == "" || strings.EqualFold(value, "none") || strings.ContainsAny(value, "{};,()") {
		return "", false
	}
	return value, true
}

func parseKeyframeSelectors(value string) ([]float64, bool) {
	parts := strings.Split(value, ",")
	if len(parts) > MaxOffsetsPerKeyframe {
		return nil, false
	}
	offsets := make([]float64, 0, len(parts))
	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(part))
		switch part {
		case "from":
			offsets = append(offsets, 0)
		case "to":
			offsets = append(offsets, 1)
		default:
			if !strings.HasSuffix(part, "%") {
				return nil, false
			}
			percentage, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(part, "%")), 64)
			if err != nil || percentage < 0 || percentage > 100 {
				return nil, false
			}
			offsets = append(offsets, percentage/100)
		}
	}
	return offsets, len(offsets) != 0
}

// ParseDeclarations parses the contents of an element's style attribute.
func ParseDeclarations(value string) ([]Declaration, error) {
	stylesheet, err := Parse(strings.NewReader("*{" + value + "}"))
	if err != nil {
		return nil, err
	}
	if len(stylesheet.Rules) == 0 {
		return nil, nil
	}
	return append([]Declaration(nil), stylesheet.Rules[0].Declarations...), nil
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

func functionDepthWithin(value string, limit int) bool {
	depth := 0
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
			continue
		}
		switch character {
		case '(':
			depth++
			if depth > limit {
				return false
			}
		case ')':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return quote == 0 && depth == 0
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
	for _, layer := range other.LayerOrder {
		s.ensureLayer(layer)
	}
	s.FontFaces = append(s.FontFaces, other.FontFaces...)
	for _, registered := range other.Properties {
		replaced := false
		for index := range s.Properties {
			if s.Properties[index].Name == registered.Name {
				s.Properties[index], replaced = registered, true
				break
			}
		}
		if !replaced {
			if len(s.Properties) >= MaxCustomProperties {
				continue
			}
			s.Properties = append(s.Properties, registered)
		}
	}
	for _, rule := range other.Rules {
		if len(s.Rules) >= MaxStylesheetRules {
			break
		}
		rule.Order = len(s.Rules)
		s.Rules = append(s.Rules, rule)
	}
	for _, keyframes := range other.Keyframes {
		copy := KeyframesRule{Name: keyframes.Name, Frames: make([]Keyframe, len(keyframes.Frames))}
		for index, frame := range keyframes.Frames {
			copy.Frames[index] = Keyframe{
				Offsets:      append([]float64(nil), frame.Offsets...),
				Declarations: append([]Declaration(nil), frame.Declarations...),
			}
		}
		s.Keyframes = append(s.Keyframes, copy)
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
	if !ok || len(parts) > MaxSelectorsPerRule {
		return nil
	}
	return parseSelectorTexts(parts, MaxSelectorsPerRule)
}

func parseSelectorTexts(parts []string, limit int) []Selector {
	if len(parts) == 0 || len(parts) > limit {
		return nil
	}
	selectors := make([]Selector, 0, len(parts))
	for _, part := range parts {
		selector, ok := parseSelector(strings.TrimSpace(part))
		if !ok {
			// One invalid selector invalidates the complete selector list.
			return nil
		}
		selectors = append(selectors, selector)
	}
	return selectors
}

// ParseSelectorList parses a bounded DOM/CSS selector list using the same
// grammar accepted by the style engine.
func ParseSelectorList(value string) []Selector {
	return parseSelectorList(value)
}

func parseSelector(value string) (Selector, bool) {
	return parseSelectorDepth(value, 0)
}

func parseSelectorDepth(value string, depth int) (Selector, bool) {
	if depth > MaxFunctionalSelectorDepth {
		return Selector{}, false
	}
	parts, combinators, ok := splitComplexSelector(value)
	if !ok || len(combinators) > MaxSelectorCombinators {
		return Selector{}, false
	}
	compounds := make([]CompoundSelector, 0, len(parts))
	for _, part := range parts {
		compound, ok := parseCompoundSelectorDepth(part, depth)
		if !ok {
			return Selector{}, false
		}
		compounds = append(compounds, compound)
	}
	for _, compound := range compounds[:len(compounds)-1] {
		if compound.PseudoElement != PseudoElementNone {
			return Selector{}, false
		}
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

func parseCompoundSelectorDepth(value string, depth int) (CompoundSelector, bool) {
	if value == "" {
		return CompoundSelector{}, false
	}

	compound := CompoundSelector{}
	position := 0
	if value[0] == '*' {
		compound.Universal = true
		position++
	} else if value[0] != '.' && value[0] != '#' && value[0] != '[' && value[0] != ':' {
		name, end, ok := parseSelectorName(value, position)
		if !ok {
			return CompoundSelector{}, false
		}
		compound.Type = strings.ToLower(name)
		position = end
	}
	for position < len(value) {
		if compound.PseudoElement != PseudoElementNone {
			return CompoundSelector{}, false
		}
		prefix := value[position]
		switch prefix {
		case '.', '#':
			position++
			name, end, ok := parseSelectorName(value, position)
			if !ok {
				return CompoundSelector{}, false
			}
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
		case ':':
			if position+1 < len(value) && value[position+1] == ':' {
				pseudoElement, next, ok := parsePseudoElement(value, position)
				if !ok {
					return CompoundSelector{}, false
				}
				compound.PseudoElement = pseudoElement
				position = next
				continue
			}
			pseudo, next, ok := parsePseudoClassDepth(value, position, depth)
			if !ok {
				return CompoundSelector{}, false
			}
			if pseudo == nil {
				compound.Hover = true
			} else {
				compound.Pseudos = append(compound.Pseudos, *pseudo)
			}
			position = next
		default:
			return CompoundSelector{}, false
		}
	}
	if !compound.Universal && compound.Type == "" && len(compound.IDs) == 0 && len(compound.Classes) == 0 && len(compound.Attributes) == 0 && len(compound.Pseudos) == 0 && !compound.Hover && compound.PseudoElement == PseudoElementNone {
		return CompoundSelector{}, false
	}
	return compound, true
}

func parsePseudoElement(value string, start int) (PseudoElementKind, int, bool) {
	nameStart := start + 2
	if nameStart >= len(value) {
		return PseudoElementNone, 0, false
	}
	name, nameEnd, ok := parseSelectorName(value, nameStart)
	if !ok {
		return PseudoElementNone, 0, false
	}
	switch strings.ToLower(name) {
	case "before":
		return PseudoElementBefore, nameEnd, true
	case "after":
		return PseudoElementAfter, nameEnd, true
	default:
		return PseudoElementNone, 0, false
	}
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
		if character == '\\' {
			_, next, valid := decodeIdentifierEscape(value, position)
			if !valid {
				return nil, nil, false
			}
			position = next - 1
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

func parseSelectorName(value string, start int) (string, int, bool) {
	if start >= len(value) {
		return "", start, false
	}
	var result strings.Builder
	position := start
	for position < len(value) {
		character := value[position]
		if character == '.' || character == '#' || character == '[' || character == ':' || character == '(' ||
			character == ')' || character == ',' || character == '>' || character == '+' || character == '~' ||
			character == '{' || character == '}' || isSelectorWhitespace(character) {
			break
		}
		if character == '\\' {
			decoded, next, ok := decodeIdentifierEscape(value, position)
			if !ok {
				return "", start, false
			}
			result.WriteRune(decoded)
			position = next
			continue
		}
		decoded, size := utf8.DecodeRuneInString(value[position:])
		if decoded == utf8.RuneError && size == 1 || !validIdentifierRune(decoded) {
			return "", start, false
		}
		result.WriteRune(decoded)
		position += size
	}
	return result.String(), position, position > start && result.Len() != 0
}

func validIdentifierRune(character rune) bool {
	return character == '-' || character == '_' ||
		character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9' || character >= 0x80
}

func validName(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if !validIdentifierRune(character) {
			return false
		}
	}
	return true
}

func parsePseudoClassDepth(value string, start, depth int) (*PseudoClass, int, bool) {
	if start+1 >= len(value) || value[start+1] == ':' {
		return nil, 0, false
	}
	nameStart := start + 1
	parsedName, nameEnd, ok := parseSelectorName(value, nameStart)
	if !ok {
		return nil, 0, false
	}
	name := strings.ToLower(parsedName)
	var argument *string
	next := nameEnd
	if nameEnd < len(value) && value[nameEnd] == '(' {
		end, ok := parenthesisEnd(value, nameEnd)
		if !ok {
			return nil, 0, false
		}
		raw := strings.TrimSpace(value[nameEnd+1 : end])
		argument = &raw
		next = end + 1
	}
	if name == "hover" && argument == nil {
		return nil, next, true
	}
	pseudo := &PseudoClass{}
	switch name {
	case "root":
		pseudo.Kind = PseudoRoot
	case "host":
		// :host is valid selector syntax even when matching a light-DOM
		// document. Keep it in selector lists so a sibling :root selector is
		// not discarded; without Shadow DOM it deliberately never matches.
		pseudo.Kind = PseudoHost
	case "open":
		pseudo.Kind = PseudoOpen
	case "empty":
		pseudo.Kind = PseudoEmpty
	case "first-child":
		pseudo.Kind = PseudoFirstChild
	case "last-child":
		pseudo.Kind = PseudoLastChild
	case "only-child":
		pseudo.Kind = PseudoOnlyChild
	case "first-of-type":
		pseudo.Kind = PseudoFirstOfType
	case "last-of-type":
		pseudo.Kind = PseudoLastOfType
	case "only-of-type":
		pseudo.Kind = PseudoOnlyOfType
	case "nth-child", "nth-last-child", "nth-of-type", "nth-last-of-type":
		if argument == nil {
			return nil, 0, false
		}
		a, b, ok := parseNth(*argument)
		if !ok {
			return nil, 0, false
		}
		pseudo.A, pseudo.B = a, b
		switch name {
		case "nth-child":
			pseudo.Kind = PseudoNthChild
		case "nth-last-child":
			pseudo.Kind = PseudoNthLastChild
		case "nth-of-type":
			pseudo.Kind = PseudoNthOfType
		case "nth-last-of-type":
			pseudo.Kind = PseudoNthLastOfType
		}
	case "not":
		if argument == nil {
			return nil, 0, false
		}
		if depth >= MaxFunctionalSelectorDepth {
			return nil, 0, false
		}
		selectors, ok := parseFunctionalSelectorListDepth(*argument, false, false, depth+1)
		if !ok {
			return nil, 0, false
		}
		pseudo.Kind, pseudo.Selectors = PseudoNot, selectors
		if len(selectors) == 1 && len(selectors[0].Compounds) == 1 {
			pseudo.Negation = &selectors[0].Compounds[0]
		}
	case "is", "where", "has":
		if argument == nil {
			return nil, 0, false
		}
		relative := name == "has"
		if depth >= MaxFunctionalSelectorDepth {
			return nil, 0, false
		}
		selectors, ok := parseFunctionalSelectorListDepth(*argument, true, relative, depth+1)
		if !ok {
			return nil, 0, false
		}
		pseudo.Selectors = selectors
		switch name {
		case "is":
			pseudo.Kind = PseudoIs
		case "where":
			pseudo.Kind = PseudoWhere
		case "has":
			pseudo.Kind = PseudoHas
		}
	case "scope":
		pseudo.Kind = PseudoScope
	case "link":
		pseudo.Kind = PseudoLink
	case "focus":
		pseudo.Kind = PseudoFocus
	case "enabled":
		pseudo.Kind = PseudoEnabled
	case "disabled":
		pseudo.Kind = PseudoDisabled
	case "checked":
		pseudo.Kind = PseudoChecked
	case "valid":
		pseudo.Kind = PseudoValid
	case "invalid":
		pseudo.Kind = PseudoInvalid
	case "defined":
		pseudo.Kind = PseudoDefined
	case "placeholder-shown":
		pseudo.Kind = PseudoPlaceholderShown
	case "read-only":
		pseudo.Kind = PseudoReadOnly
	case "read-write":
		pseudo.Kind = PseudoReadWrite
	case "required":
		pseudo.Kind = PseudoRequired
	case "optional":
		pseudo.Kind = PseudoOptional
	case "focus-visible":
		pseudo.Kind = PseudoFocusVisible
	case "focus-within":
		pseudo.Kind = PseudoFocusWithin
	default:
		return nil, 0, false
	}
	if argument != nil && pseudo.Kind != PseudoNthChild && pseudo.Kind != PseudoNthLastChild &&
		pseudo.Kind != PseudoNthOfType && pseudo.Kind != PseudoNthLastOfType && pseudo.Kind != PseudoNot &&
		pseudo.Kind != PseudoIs && pseudo.Kind != PseudoWhere && pseudo.Kind != PseudoHas {
		return nil, 0, false
	}
	if argument == nil && (pseudo.Kind == PseudoNot || pseudo.Kind == PseudoIs || pseudo.Kind == PseudoWhere || pseudo.Kind == PseudoHas) {
		return nil, 0, false
	}
	return pseudo, next, true
}

func parseFunctionalSelectorListDepth(value string, forgiving, relative bool, depth int) ([]Selector, bool) {
	if depth > MaxFunctionalSelectorDepth {
		return nil, false
	}
	parts, ok := splitSelectorList(value)
	if !ok || len(parts) > MaxSelectorsPerRule {
		return nil, false
	}
	selectors := make([]Selector, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if relative {
			if part == "" {
				continue
			}
			if _, explicit := explicitCombinator(part[0]); explicit {
				part = ":scope " + part
			} else {
				part = ":scope " + part
			}
		}
		selector, valid := parseSelectorDepth(part, depth)
		if !valid || selectorPseudoElementKind(selector) != PseudoElementNone {
			if forgiving {
				continue
			}
			return nil, false
		}
		if relative && len(selector.Compounds) != 0 {
			for index := range selector.Compounds[0].Pseudos {
				if selector.Compounds[0].Pseudos[index].Kind == PseudoScope {
					selector.Compounds[0].Pseudos[index].Kind = PseudoRelativeScope
					break
				}
			}
		}
		selectors = append(selectors, selector)
	}
	return selectors, len(selectors) != 0
}

func selectorPseudoElementKind(selector Selector) PseudoElementKind {
	if len(selector.Compounds) == 0 {
		return PseudoElementNone
	}
	return selector.Compounds[len(selector.Compounds)-1].PseudoElement
}

func parenthesisEnd(value string, start int) (int, bool) {
	depth := 1
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
		if character == '\\' {
			_, next, valid := decodeIdentifierEscape(value, position)
			if !valid {
				return 0, false
			}
			position = next - 1
			continue
		}
		switch character {
		case '\'', '"':
			quote = character
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return position, true
			}
		}
	}
	return 0, false
}

func parseNth(value string) (int, int, bool) {
	value = strings.ToLower(value)
	value = strings.Map(func(character rune) rune {
		if character == ' ' || character == '\t' || character == '\n' || character == '\r' || character == '\f' {
			return -1
		}
		return character
	}, value)
	switch value {
	case "odd":
		return 2, 1, true
	case "even":
		return 2, 0, true
	}
	if position := strings.IndexByte(value, 'n'); position >= 0 {
		if strings.Count(value, "n") != 1 {
			return 0, 0, false
		}
		coefficient := value[:position]
		var a int
		var err error
		switch coefficient {
		case "", "+":
			a = 1
		case "-":
			a = -1
		default:
			a, err = strconv.Atoi(coefficient)
		}
		if err != nil {
			return 0, 0, false
		}
		b := 0
		if constant := value[position+1:]; constant != "" {
			if constant[0] != '+' && constant[0] != '-' {
				return 0, 0, false
			}
			b, err = strconv.Atoi(constant)
			if err != nil {
				return 0, 0, false
			}
		}
		return a, b, true
	}
	b, err := strconv.Atoi(value)
	return 0, b, err == nil
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
		if character == '\\' {
			_, next, valid := decodeIdentifierEscape(value, position)
			if !valid {
				return nil, false
			}
			position = next - 1
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
		if character == '\\' {
			_, next, valid := decodeIdentifierEscape(value, position)
			if !valid {
				return 0, false
			}
			position = next - 1
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
	if (len(tokens) != 3 && len(tokens) != 4) || (tokens[2].kind != parser.IdentToken && tokens[2].kind != parser.StringToken) {
		return AttributeSelector{}, false
	}
	if len(tokens) == 4 {
		if tokens[3].kind != parser.IdentToken || !strings.EqualFold(tokens[3].raw, "i") && !strings.EqualFold(tokens[3].raw, "s") {
			return AttributeSelector{}, false
		}
		attribute.CaseInsensitive = strings.EqualFold(tokens[3].raw, "i")
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
	if decoded, ok := DecodeString(value); ok {
		return decoded
	}
	return value
}

func stripImportant(value string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(value))
	if strings.HasSuffix(lower, "!important") {
		return strings.TrimSpace(value[:len(value)-len("!important")]), true
	}
	return value, false
}
