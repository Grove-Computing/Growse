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
	result := make(Map)
	if document == nil || document.Root == nil {
		return result
	}
	computeNode(document.Root, initialStyle(), stylesheet, result)
	return result
}

func computeNode(node *dom.Node, parent ComputedStyle, stylesheet *css.Stylesheet, result Map) {
	computed := parent
	computed.BackgroundColor = transparent
	if node.Type == dom.NodeDocument {
		computed = initialStyle()
	} else if node.Type == dom.NodeElement {
		computed = applyUADefaults(node.TagName, computed)
		computed = applyAuthorRules(node, computed, stylesheet)
		result[node.ID] = computed
	} else if node.Type == dom.NodeText {
		result[node.ID] = computed
	}

	for _, child := range node.Children {
		computeNode(child, computed, stylesheet, result)
	}
}

func initialStyle() ComputedStyle {
	return ComputedStyle{Color: defaultTextColor, BackgroundColor: transparent, FontSize: 16, FontWeight: 400}
}

func applyUADefaults(tag string, computed ComputedStyle) ComputedStyle {
	switch tag {
	case "h1":
		computed.FontSize, computed.FontWeight = 32, 700
	case "h2":
		computed.FontSize, computed.FontWeight = 26, 700
	case "h3":
		computed.FontSize, computed.FontWeight = 21, 700
	case "button":
		computed.FontWeight = 700
	case "a":
		computed.Color = 0x0969daff
	case "pre":
		computed.FontSize = 15
	}
	return computed
}

func applyAuthorRules(node *dom.Node, computed ComputedStyle, stylesheet *css.Stylesheet) ComputedStyle {
	if stylesheet == nil {
		return computed
	}
	winners := make(map[string]winner)
	for _, rule := range stylesheet.Rules {
		for _, selector := range rule.Selectors {
			if !matches(node, selector) {
				continue
			}
			for declarationIndex, declaration := range rule.Declarations {
				candidate := winner{
					value: declaration.Value, important: declaration.Important,
					specificity: selector.Specificity(), order: rule.Order*1000 + declarationIndex,
				}
				current, exists := winners[declaration.Property]
				if !exists || outranks(candidate, current) {
					winners[declaration.Property] = candidate
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
	return computed
}

func matches(node *dom.Node, selector css.Selector) bool {
	if node == nil || node.Type != dom.NodeElement {
		return false
	}
	if selector.Tag != "" && node.TagName != selector.Tag {
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
