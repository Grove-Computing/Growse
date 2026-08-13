package style

import (
	"strconv"
	"strings"

	"github.com/saku0512/growse/internal/css"
	"github.com/saku0512/growse/internal/dom"
)

const (
	defaultTextColor = uint32(0x202124ff)
	transparent      = uint32(0x00000000)
)

type winner struct {
	value       string
	important   bool
	inline      bool
	specificity [3]int
	order       [2]int
}

// Compute applies UA defaults, inheritance, selector matching and cascade.
func Compute(document *dom.Document, stylesheet *css.Stylesheet) Map {
	return ComputeWithState(document, stylesheet, InteractionState{})
}

// Environment contains rendering metrics needed during value computation.
type Environment struct {
	ViewportWidth  float32
	ViewportHeight float32
	RootFontSize   float32
}

func defaultEnvironment() Environment {
	return Environment{ViewportWidth: 1280, ViewportHeight: 720, RootFontSize: 16}
}

// ComputeWithState applies styles using transient browser interaction state.
func ComputeWithState(document *dom.Document, stylesheet *css.Stylesheet, state InteractionState) Map {
	return ComputeWithEnvironment(document, stylesheet, state, defaultEnvironment())
}

// ComputeWithEnvironment applies styles using interaction and viewport state.
func ComputeWithEnvironment(document *dom.Document, stylesheet *css.Stylesheet, state InteractionState, environment Environment) Map {
	result := make(Map)
	if document == nil || document.Root == nil {
		return result
	}
	if environment.ViewportWidth <= 0 {
		environment.ViewportWidth = defaultEnvironment().ViewportWidth
	}
	if environment.ViewportHeight <= 0 {
		environment.ViewportHeight = defaultEnvironment().ViewportHeight
	}
	if environment.RootFontSize <= 0 {
		environment.RootFontSize = 16
	}
	computeNode(document.Root, initialStyle(), stylesheet, state, environment, result)
	return result
}

func computeNode(node *dom.Node, parent ComputedStyle, stylesheet *css.Stylesheet, state InteractionState, environment Environment, result Map) {
	computed := inheritedStyle(parent)
	if node.Type == dom.NodeDocument {
		computed = initialStyle()
	} else if node.Type == dom.NodeElement {
		computed = applyUADefaults(node.TagName, computed)
		computed = applyAuthorRules(node, computed, parent, stylesheet, state, environment)
		computed = applyGeneratedContent(node, computed, stylesheet, state)
		result[node.ID] = computed
	} else if node.Type == dom.NodeText {
		result[node.ID] = computed
	}

	childEnvironment := environment
	if node.Type == dom.NodeElement && node.TagName == "html" {
		childEnvironment.RootFontSize = computed.FontSize
	}
	for _, child := range node.Children {
		computeNode(child, computed, stylesheet, state, childEnvironment, result)
	}
}

func initialStyle() ComputedStyle {
	return ComputedStyle{Color: defaultTextColor, BackgroundColor: transparent, FontSize: 16, FontWeight: 400}
}

func inheritedStyle(parent ComputedStyle) ComputedStyle {
	computed := ComputedStyle{
		Color: parent.Color, FontSize: parent.FontSize, FontWeight: parent.FontWeight,
		BackgroundColor: transparent, Display: DisplayInline,
	}
	if len(parent.CustomProperties) != 0 {
		computed.CustomProperties = make(map[string]string, len(parent.CustomProperties))
		for name, value := range parent.CustomProperties {
			computed.CustomProperties[name] = value
		}
	}
	return computed
}

func applyUADefaults(tag string, computed ComputedStyle) ComputedStyle {
	switch tag {
	case "html", "body", "div", "main", "section", "article", "header", "footer", "nav", "form", "ul", "ol", "input":
		computed.Display = DisplayBlock
	case "h1":
		computed.Display, computed.FontSize, computed.FontWeight = DisplayBlock, 32, 700
		computed.Margin = Edges{Top: 12, Bottom: 12}
	case "h2":
		computed.Display, computed.FontSize, computed.FontWeight = DisplayBlock, 26, 700
		computed.Margin = Edges{Top: 10, Bottom: 10}
	case "h3":
		computed.Display, computed.FontSize, computed.FontWeight = DisplayBlock, 21, 700
		computed.Margin = Edges{Top: 8, Bottom: 8}
	case "h4", "h5", "h6":
		computed.Display, computed.FontWeight = DisplayBlock, 700
		computed.Margin = Edges{Top: 8, Bottom: 8}
	case "p":
		computed.Display = DisplayBlock
		computed.Margin.Bottom = 14
	case "button":
		computed.FontWeight = 700
	case "a":
		computed.Color = 0x0969daff
	case "pre":
		computed.Display, computed.FontSize = DisplayBlock, 15
		computed.Margin.Bottom = 14
	case "li":
		computed.Display = DisplayBlock
		computed.Margin.Bottom = 6
	case "head", "script", "style", "noscript", "template":
		computed.Display = DisplayNone
	}
	return computed
}

func applyAuthorRules(node *dom.Node, computed, parent ComputedStyle, stylesheet *css.Stylesheet, state InteractionState, environment Environment) ComputedStyle {
	if stylesheet == nil {
		stylesheet = &css.Stylesheet{}
	}
	winners := make(map[string]winner)
	for _, rule := range stylesheet.Rules {
		for _, selector := range rule.Selectors {
			if selectorPseudoElement(selector) != css.PseudoElementNone {
				continue
			}
			if !matches(node, selector, state) {
				continue
			}
			for declarationIndex, declaration := range rule.Declarations {
				for _, property := range expandedProperties(declaration.Property) {
					candidate := winner{
						value: declaration.Value.Raw, important: declaration.Important,
						specificity: selector.Specificity(), order: [2]int{rule.Order, declarationIndex},
					}
					current, exists := winners[property]
					if !exists || outranks(candidate, current) {
						winners[property] = candidate
					}
				}
			}
		}
	}
	if inlineValue, ok := node.Attribute("style"); ok {
		declarations, _ := css.ParseDeclarations(inlineValue)
		for declarationIndex, declaration := range declarations {
			for _, property := range expandedProperties(declaration.Property) {
				candidate := winner{
					value: declaration.Value.Raw, important: declaration.Important, inline: true,
					order: [2]int{0, declarationIndex},
				}
				current, exists := winners[property]
				if !exists || outranks(candidate, current) {
					winners[property] = candidate
				}
			}
		}
	}
	computed.CustomProperties = applyCustomProperties(computed.CustomProperties, winners)
	fontContext := LengthContext{
		FontSize: parent.FontSize, RootFontSize: environment.RootFontSize,
		ViewportWidth: environment.ViewportWidth, ViewportHeight: environment.ViewportHeight,
		PercentageBase: parent.FontSize,
	}

	if value, ok := winners["color"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			if parsed, valid := resolveColor(resolved, parent.Color, defaultTextColor, true); valid {
				computed.Color = parsed
			}
		}
	}
	if value, ok := winners["background-color"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			if parsed, valid := resolveColor(resolved, parent.BackgroundColor, transparent, false); valid {
				computed.BackgroundColor = parsed
			}
		}
	}
	if value, ok := winners["font-size"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			parseFontSize := func(value string) (float32, bool) {
				length, valid := ResolveLength(value, fontContext)
				resolved := length.Resolve(fontContext.PercentageBase)
				return resolved, valid && resolved > 0
			}
			if parsed, valid := resolveFloat(resolved, parent.FontSize, 16, true, parseFontSize); valid {
				computed.FontSize = parsed
			}
		}
	}
	if value, ok := winners["font-weight"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			if parsed, valid := resolveInt(resolved, parent.FontWeight, 400, true, parseFontWeight); valid {
				computed.FontWeight = parsed
			}
		}
	}
	if value, ok := winners["display"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			if parsed, valid := resolveDisplay(resolved, parent.Display); valid {
				computed.Display = parsed
			}
		}
	}
	lengthContext := LengthContext{
		FontSize: computed.FontSize, RootFontSize: environment.RootFontSize,
		ViewportWidth: environment.ViewportWidth, ViewportHeight: environment.ViewportHeight,
		PercentageBase: environment.ViewportWidth,
	}
	computed.Margin = applyEdges(computed.Margin, parent.Margin, "margin", winners, computed.CustomProperties, lengthContext)
	computed.Padding = applyEdges(computed.Padding, parent.Padding, "padding", winners, computed.CustomProperties, lengthContext)
	return computed
}

type globalKeyword uint8

const (
	globalNone globalKeyword = iota
	globalInherit
	globalInitial
	globalUnset
)

func parseGlobalKeyword(value string) globalKeyword {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "inherit":
		return globalInherit
	case "initial":
		return globalInitial
	case "unset":
		return globalUnset
	default:
		return globalNone
	}
}

func resolveColor(value string, parent, initial uint32, inherited bool) (uint32, bool) {
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent, true
	case globalInitial:
		return initial, true
	case globalUnset:
		if inherited {
			return parent, true
		}
		return initial, true
	default:
		return parseColor(value)
	}
}

func resolveFloat(value string, parent, initial float32, inherited bool, parse func(string) (float32, bool)) (float32, bool) {
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent, true
	case globalInitial:
		return initial, true
	case globalUnset:
		if inherited {
			return parent, true
		}
		return initial, true
	default:
		return parse(value)
	}
}

func resolveInt(value string, parent, initial int, inherited bool, parse func(string) (int, bool)) (int, bool) {
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent, true
	case globalInitial:
		return initial, true
	case globalUnset:
		if inherited {
			return parent, true
		}
		return initial, true
	default:
		return parse(value)
	}
}

func resolveDisplay(value string, parent Display) (Display, bool) {
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent, true
	case globalInitial, globalUnset:
		return DisplayInline, true
	default:
		return parseDisplay(value)
	}
}

func parsePositivePixels(value string) (float32, bool) {
	parsed, valid := parsePixels(value)
	return parsed, valid && parsed > 0
}

func applyCustomProperties(inherited map[string]string, winners map[string]winner) map[string]string {
	result := inherited
	for property, candidate := range winners {
		if !strings.HasPrefix(property, "--") {
			continue
		}
		switch parseGlobalKeyword(candidate.value) {
		case globalInitial:
			if result != nil {
				delete(result, property)
			}
		case globalInherit, globalUnset:
			// Custom properties inherit, so the inherited value is retained.
		default:
			if result == nil {
				result = make(map[string]string)
			}
			result[property] = candidate.value
		}
	}
	return result
}

func resolveVariables(value string, customProperties map[string]string) (string, bool) {
	return resolveVariablesWithStack(value, customProperties, make(map[string]bool))
}

func resolveVariablesWithStack(value string, customProperties map[string]string, resolving map[string]bool) (string, bool) {
	var result strings.Builder
	for position := 0; position < len(value); {
		functionStart := findVarFunction(value, position)
		if functionStart < 0 {
			result.WriteString(value[position:])
			break
		}
		result.WriteString(value[position:functionStart])
		open := functionStart + 3
		end, ok := cssFunctionEnd(value, open)
		if !ok {
			return "", false
		}
		name, fallback, hasFallback := splitVarArguments(value[open+1 : end])
		name = strings.TrimSpace(name)
		if !strings.HasPrefix(name, "--") || !validCustomPropertyName(name) {
			return "", false
		}
		replacement, found := customProperties[name]
		resolved := ""
		if found && !resolving[name] {
			resolving[name] = true
			resolved, found = resolveVariablesWithStack(replacement, customProperties, resolving)
			delete(resolving, name)
		} else {
			found = false
		}
		if !found {
			if !hasFallback {
				return "", false
			}
			resolved, found = resolveVariablesWithStack(fallback, customProperties, resolving)
			if !found {
				return "", false
			}
		}
		result.WriteString(resolved)
		position = end + 1
	}
	return result.String(), true
}

func findVarFunction(value string, start int) int {
	var quote byte
	escaped := false
	for position := start; position+4 <= len(value); position++ {
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
		if strings.EqualFold(value[position:position+4], "var(") &&
			(position == 0 || !isCSSNameByte(value[position-1])) {
			return position
		}
	}
	return -1
}

func cssFunctionEnd(value string, open int) (int, bool) {
	if open >= len(value) || value[open] != '(' {
		return 0, false
	}
	depth := 1
	var quote byte
	escaped := false
	for position := open + 1; position < len(value); position++ {
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
			if depth == 0 {
				return position, true
			}
		}
	}
	return 0, false
}

func splitVarArguments(value string) (string, string, bool) {
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
		switch character {
		case '\'', '"':
			quote = character
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				return value[:position], value[position+1:], true
			}
		}
	}
	return value, "", false
}

func validCustomPropertyName(value string) bool {
	if len(value) <= 2 {
		return false
	}
	for position := 2; position < len(value); position++ {
		if !isCSSNameByte(value[position]) {
			return false
		}
	}
	return true
}

func isCSSNameByte(value byte) bool {
	return value == '-' || value == '_' || value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value >= 0x80
}

func applyGeneratedContent(node *dom.Node, computed ComputedStyle, stylesheet *css.Stylesheet, state InteractionState) ComputedStyle {
	if stylesheet == nil {
		return computed
	}
	for _, target := range []struct {
		kind        css.PseudoElementKind
		destination *string
	}{
		{css.PseudoElementBefore, &computed.BeforeContent},
		{css.PseudoElementAfter, &computed.AfterContent},
	} {
		var selected winner
		found := false
		for _, rule := range stylesheet.Rules {
			for _, selector := range rule.Selectors {
				if selectorPseudoElement(selector) != target.kind || !matches(node, selector, state) {
					continue
				}
				for declarationIndex, declaration := range rule.Declarations {
					if declaration.Property != "content" {
						continue
					}
					candidate := winner{
						value: declaration.Value.Raw, important: declaration.Important,
						specificity: selector.Specificity(), order: [2]int{rule.Order, declarationIndex},
					}
					if !found || outranks(candidate, selected) {
						selected, found = candidate, true
					}
				}
			}
		}
		if found {
			resolved, valid := resolveVariables(selected.value, computed.CustomProperties)
			if !valid {
				continue
			}
			if content, valid := parseGeneratedContent(resolved); valid {
				*target.destination = content
			}
		}
	}
	return computed
}

func selectorPseudoElement(selector css.Selector) css.PseudoElementKind {
	if len(selector.Compounds) == 0 {
		return css.PseudoElementNone
	}
	return selector.Compounds[len(selector.Compounds)-1].PseudoElement
}

func parseGeneratedContent(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "none") || strings.EqualFold(value, "normal") || parseGlobalKeyword(value) != globalNone {
		return "", true
	}
	return css.DecodeString(value)
}

func expandedProperties(property string) []string {
	switch property {
	case "margin", "padding":
		return []string{property + "-top", property + "-right", property + "-bottom", property + "-left"}
	default:
		return []string{property}
	}
}

func applyEdges(edges, parent Edges, prefix string, winners map[string]winner, customProperties map[string]string, context LengthContext) Edges {
	properties := []string{prefix + "-top", prefix + "-right", prefix + "-bottom", prefix + "-left"}
	values := []*float32{&edges.Top, &edges.Right, &edges.Bottom, &edges.Left}
	parentValues := []float32{parent.Top, parent.Right, parent.Bottom, parent.Left}
	for index, property := range properties {
		candidate, ok := winners[property]
		if !ok {
			continue
		}
		resolved, ok := resolveVariables(candidate.value, customProperties)
		if !ok {
			continue
		}
		var parsed float32
		var valid bool
		switch parseGlobalKeyword(resolved) {
		case globalInherit:
			parsed, valid = parentValues[index], true
		case globalInitial, globalUnset:
			parsed, valid = 0, true
		default:
			parsed, valid = parseEdgeValue(resolved, index, context)
			if prefix == "padding" && parsed < 0 {
				valid = false
			}
		}
		if valid {
			*values[index] = parsed
		}
	}
	return edges
}

func parseEdgeValue(value string, side int, context LengthContext) (float32, bool) {
	parts := strings.Fields(value)
	if len(parts) < 1 || len(parts) > 4 {
		return 0, false
	}
	resolved := [4]string{}
	switch len(parts) {
	case 1:
		resolved = [4]string{parts[0], parts[0], parts[0], parts[0]}
	case 2:
		resolved = [4]string{parts[0], parts[1], parts[0], parts[1]}
	case 3:
		resolved = [4]string{parts[0], parts[1], parts[2], parts[1]}
	case 4:
		resolved = [4]string{parts[0], parts[1], parts[2], parts[3]}
	}
	length, valid := ResolveLength(resolved[side], context)
	return length.Resolve(context.PercentageBase), valid
}

func parseDisplay(value string) (Display, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "inline":
		return DisplayInline, true
	case "block":
		return DisplayBlock, true
	case "none":
		return DisplayNone, true
	default:
		return DisplayInline, false
	}
}

func matches(node *dom.Node, selector css.Selector, state InteractionState) bool {
	if node == nil || node.Type != dom.NodeElement {
		return false
	}
	if len(selector.Compounds) != 0 {
		return matchesSelectorAt(node, selector, len(selector.Compounds)-1, state)
	}
	if selector.Tag != "" && node.TagName != selector.Tag {
		return false
	}
	if selector.Hover && !state.Hovered[node.ID] {
		return false
	}
	if selector.ID != "" {
		id, _ := node.Attribute("id")
		if id != selector.ID {
			return false
		}
	}
	if selector.Class != "" {
		classes, _ := node.Attribute("class")
		for _, class := range strings.Fields(classes) {
			if class == selector.Class {
				return true
			}
		}
		return false
	}
	return true
}

func matchesSelectorAt(node *dom.Node, selector css.Selector, index int, state InteractionState) bool {
	if !matchesCompound(node, selector.Compounds[index], state) {
		return false
	}
	if index == 0 {
		return true
	}
	switch selector.Combinators[index-1] {
	case css.CombinatorDescendant:
		for ancestor := node.Parent; ancestor != nil; ancestor = ancestor.Parent {
			if ancestor.Type == dom.NodeElement && matchesSelectorAt(ancestor, selector, index-1, state) {
				return true
			}
		}
		return false
	case css.CombinatorChild:
		return node.Parent != nil && node.Parent.Type == dom.NodeElement &&
			matchesSelectorAt(node.Parent, selector, index-1, state)
	case css.CombinatorAdjacentSibling:
		previous := previousElementSibling(node)
		return previous != nil && matchesSelectorAt(previous, selector, index-1, state)
	case css.CombinatorGeneralSibling:
		for previous := previousElementSibling(node); previous != nil; previous = previousElementSibling(previous) {
			if matchesSelectorAt(previous, selector, index-1, state) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func previousElementSibling(node *dom.Node) *dom.Node {
	if node == nil || node.Parent == nil {
		return nil
	}
	var previous *dom.Node
	for _, sibling := range node.Parent.Children {
		if sibling == node {
			return previous
		}
		if sibling.Type == dom.NodeElement {
			previous = sibling
		}
	}
	return nil
}

func matchesCompound(node *dom.Node, compound css.CompoundSelector, state InteractionState) bool {
	if compound.Type != "" && node.TagName != compound.Type {
		return false
	}
	if compound.Hover && !state.Hovered[node.ID] {
		return false
	}
	id, _ := node.Attribute("id")
	for _, expected := range compound.IDs {
		if id != expected {
			return false
		}
	}
	classes, _ := node.Attribute("class")
	available := make(map[string]bool)
	for _, class := range strings.Fields(classes) {
		available[class] = true
	}
	for _, expected := range compound.Classes {
		if !available[expected] {
			return false
		}
	}
	for _, attribute := range compound.Attributes {
		if !matchesAttribute(node, attribute) {
			return false
		}
	}
	for _, pseudo := range compound.Pseudos {
		if !matchesPseudoClass(node, pseudo, state) {
			return false
		}
	}
	return true
}

func matchesPseudoClass(node *dom.Node, pseudo css.PseudoClass, state InteractionState) bool {
	switch pseudo.Kind {
	case css.PseudoRoot:
		return node.Parent != nil && node.Parent.Type == dom.NodeDocument
	case css.PseudoEmpty:
		for _, child := range node.Children {
			if child.Type == dom.NodeElement || child.Type == dom.NodeText && child.Text != "" {
				return false
			}
		}
		return true
	case css.PseudoFirstChild:
		return elementPosition(node, false, false) == 1
	case css.PseudoLastChild:
		return elementPosition(node, true, false) == 1
	case css.PseudoOnlyChild:
		return elementPosition(node, false, false) == 1 && elementPosition(node, true, false) == 1
	case css.PseudoFirstOfType:
		return elementPosition(node, false, true) == 1
	case css.PseudoLastOfType:
		return elementPosition(node, true, true) == 1
	case css.PseudoOnlyOfType:
		return elementPosition(node, false, true) == 1 && elementPosition(node, true, true) == 1
	case css.PseudoNthChild:
		return matchesNth(elementPosition(node, false, false), pseudo.A, pseudo.B)
	case css.PseudoNthLastChild:
		return matchesNth(elementPosition(node, true, false), pseudo.A, pseudo.B)
	case css.PseudoNthOfType:
		return matchesNth(elementPosition(node, false, true), pseudo.A, pseudo.B)
	case css.PseudoNthLastOfType:
		return matchesNth(elementPosition(node, true, true), pseudo.A, pseudo.B)
	case css.PseudoNot:
		return pseudo.Negation != nil && !matchesCompound(node, *pseudo.Negation, state)
	case css.PseudoLink:
		_, hasReference := node.Attribute("href")
		return (node.TagName == "a" || node.TagName == "area") && hasReference
	case css.PseudoFocus:
		return state.Focused != 0 && state.Focused == node.ID
	case css.PseudoEnabled:
		return isFormControl(node) && !isDisabled(node)
	case css.PseudoDisabled:
		return isFormControl(node) && isDisabled(node)
	case css.PseudoChecked:
		if node.TagName == "option" {
			_, selected := node.Attribute("selected")
			return selected
		}
		inputType, _ := node.Attribute("type")
		_, checked := node.Attribute("checked")
		return node.TagName == "input" && checked && (strings.EqualFold(inputType, "checkbox") || strings.EqualFold(inputType, "radio"))
	default:
		return false
	}
}

func isFormControl(node *dom.Node) bool {
	if node == nil {
		return false
	}
	switch node.TagName {
	case "button", "input", "select", "textarea", "option", "optgroup", "fieldset":
		return true
	default:
		return false
	}
}

func isDisabled(node *dom.Node) bool {
	_, disabled := node.Attribute("disabled")
	if disabled {
		return true
	}
	if node.TagName == "option" && node.Parent != nil && node.Parent.TagName == "optgroup" {
		_, disabled = node.Parent.Attribute("disabled")
	}
	return disabled
}

func elementPosition(node *dom.Node, fromEnd, sameType bool) int {
	if node == nil || node.Parent == nil {
		return 0
	}
	position := 0
	if fromEnd {
		for index := len(node.Parent.Children) - 1; index >= 0; index-- {
			sibling := node.Parent.Children[index]
			if sibling.Type != dom.NodeElement || sameType && sibling.TagName != node.TagName {
				continue
			}
			position++
			if sibling == node {
				return position
			}
		}
		return 0
	}
	for _, sibling := range node.Parent.Children {
		if sibling.Type != dom.NodeElement || sameType && sibling.TagName != node.TagName {
			continue
		}
		position++
		if sibling == node {
			return position
		}
	}
	return 0
}

func matchesNth(position, a, b int) bool {
	if position <= 0 {
		return false
	}
	if a == 0 {
		return position == b
	}
	difference := position - b
	return difference%a == 0 && difference/a >= 0
}

func matchesAttribute(node *dom.Node, selector css.AttributeSelector) bool {
	value, present := node.Attribute(selector.Name)
	if selector.Matcher == css.AttributePresent {
		return present
	}
	if !present {
		return false
	}
	switch selector.Matcher {
	case css.AttributeExact:
		return value == selector.Value
	case css.AttributeIncludes:
		for _, word := range strings.Fields(value) {
			if word == selector.Value {
				return true
			}
		}
		return false
	case css.AttributeDashMatch:
		return value == selector.Value || strings.HasPrefix(value, selector.Value+"-")
	case css.AttributePrefix:
		return selector.Value != "" && strings.HasPrefix(value, selector.Value)
	case css.AttributeSuffix:
		return selector.Value != "" && strings.HasSuffix(value, selector.Value)
	case css.AttributeSubstring:
		return selector.Value != "" && strings.Contains(value, selector.Value)
	default:
		return false
	}
}

func outranks(candidate, current winner) bool {
	if candidate.important != current.important {
		return candidate.important
	}
	if candidate.inline != current.inline {
		return candidate.inline
	}
	for index := range candidate.specificity {
		if candidate.specificity[index] != current.specificity[index] {
			return candidate.specificity[index] > current.specificity[index]
		}
	}
	if candidate.order[0] != current.order[0] {
		return candidate.order[0] > current.order[0]
	}
	return candidate.order[1] >= current.order[1]
}

func parsePixels(value string) (float32, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !strings.HasSuffix(value, "px") {
		return 0, false
	}
	number, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, "px")), 32)
	return float32(number), err == nil
}

func parseLength(value string) (float32, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "0" {
		return 0, true
	}
	parsed, ok := parsePixels(value)
	return parsed, ok && parsed >= 0
}

func parseFontWeight(value string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "normal":
		return 400, true
	case "bold":
		return 700, true
	}
	weight, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || weight < 1 || weight > 1000 {
		return 0, false
	}
	return weight, true
}

func parseColor(value string) (uint32, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	named := map[string]uint32{
		"black": 0x000000ff, "white": 0xffffffff, "red": 0xff0000ff,
		"green": 0x008000ff, "blue": 0x0000ffff, "gray": 0x808080ff,
		"grey": 0x808080ff, "transparent": transparent,
	}
	if color, ok := named[value]; ok {
		return color, true
	}
	if !strings.HasPrefix(value, "#") {
		return 0, false
	}
	hex := value[1:]
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 {
		return 0, false
	}
	parsed, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, false
	}
	return uint32(parsed)<<8 | 0xff, true
}
