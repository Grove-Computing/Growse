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
	specificity [3]int
	order       int
}

// Compute applies UA defaults, inheritance, selector matching and cascade.
func Compute(document *dom.Document, stylesheet *css.Stylesheet) Map {
	return ComputeWithState(document, stylesheet, InteractionState{})
}

// ComputeWithState applies styles using transient browser interaction state.
func ComputeWithState(document *dom.Document, stylesheet *css.Stylesheet, state InteractionState) Map {
	result := make(Map)
	if document == nil || document.Root == nil {
		return result
	}
	computeNode(document.Root, initialStyle(), stylesheet, state, result)
	return result
}

func computeNode(node *dom.Node, parent ComputedStyle, stylesheet *css.Stylesheet, state InteractionState, result Map) {
	computed := inheritedStyle(parent)
	if node.Type == dom.NodeDocument {
		computed = initialStyle()
	} else if node.Type == dom.NodeElement {
		computed = applyUADefaults(node.TagName, computed)
		computed = applyAuthorRules(node, computed, stylesheet, state)
		result[node.ID] = computed
	} else if node.Type == dom.NodeText {
		result[node.ID] = computed
	}

	for _, child := range node.Children {
		computeNode(child, computed, stylesheet, state, result)
	}
}

func initialStyle() ComputedStyle {
	return ComputedStyle{Color: defaultTextColor, BackgroundColor: transparent, FontSize: 16, FontWeight: 400}
}

func inheritedStyle(parent ComputedStyle) ComputedStyle {
	return ComputedStyle{
		Color: parent.Color, FontSize: parent.FontSize, FontWeight: parent.FontWeight,
		BackgroundColor: transparent, Display: DisplayInline,
	}
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

func applyAuthorRules(node *dom.Node, computed ComputedStyle, stylesheet *css.Stylesheet, state InteractionState) ComputedStyle {
	if stylesheet == nil {
		return computed
	}
	winners := make(map[string]winner)
	for _, rule := range stylesheet.Rules {
		for _, selector := range rule.Selectors {
			if !matches(node, selector, state) {
				continue
			}
			for declarationIndex, declaration := range rule.Declarations {
				for _, property := range expandedProperties(declaration.Property) {
					candidate := winner{
						value: declaration.Value.Raw, important: declaration.Important,
						specificity: selector.Specificity(), order: rule.Order*1000 + declarationIndex,
					}
					current, exists := winners[property]
					if !exists || outranks(candidate, current) {
						winners[property] = candidate
					}
				}
			}
		}
	}

	if value, ok := winners["color"]; ok {
		if parsed, valid := parseColor(value.value); valid {
			computed.Color = parsed
		}
	}
	if value, ok := winners["background-color"]; ok {
		if parsed, valid := parseColor(value.value); valid {
			computed.BackgroundColor = parsed
		}
	}
	if value, ok := winners["font-size"]; ok {
		if parsed, valid := parsePixels(value.value); valid && parsed > 0 {
			computed.FontSize = parsed
		}
	}
	if value, ok := winners["font-weight"]; ok {
		if parsed, valid := parseFontWeight(value.value); valid {
			computed.FontWeight = parsed
		}
	}
	if value, ok := winners["display"]; ok {
		if parsed, valid := parseDisplay(value.value); valid {
			computed.Display = parsed
		}
	}
	computed.Margin = applyEdges(computed.Margin, "margin", winners)
	computed.Padding = applyEdges(computed.Padding, "padding", winners)
	return computed
}

func expandedProperties(property string) []string {
	switch property {
	case "margin", "padding":
		return []string{property + "-top", property + "-right", property + "-bottom", property + "-left"}
	default:
		return []string{property}
	}
}

func applyEdges(edges Edges, prefix string, winners map[string]winner) Edges {
	properties := []string{prefix + "-top", prefix + "-right", prefix + "-bottom", prefix + "-left"}
	values := []*float32{&edges.Top, &edges.Right, &edges.Bottom, &edges.Left}
	for index, property := range properties {
		candidate, ok := winners[property]
		if !ok {
			continue
		}
		parsed, valid := parseEdgeValue(candidate.value, index)
		if valid {
			*values[index] = parsed
		}
	}
	return edges
}

func parseEdgeValue(value string, side int) (float32, bool) {
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
	return parseLength(resolved[side])
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
		return matchesCompound(node, selector.Compounds[len(selector.Compounds)-1], state)
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
	return true
}

func outranks(candidate, current winner) bool {
	if candidate.important != current.important {
		return candidate.important
	}
	for index := range candidate.specificity {
		if candidate.specificity[index] != current.specificity[index] {
			return candidate.specificity[index] > current.specificity[index]
		}
	}
	return candidate.order >= current.order
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
