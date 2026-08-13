// Package css parses CSS syntax into Growse-owned rules.
package css

// Stylesheet is an ordered collection of CSS rules.
type Stylesheet struct {
	Rules []Rule
}

// RuleKind identifies the grammar represented by a rule.
type RuleKind uint8

const (
	// RuleStyle associates selectors with declarations.
	RuleStyle RuleKind = iota
	// RuleAt represents an at-rule. At-rule evaluation is added incrementally.
	RuleAt
)

// Rule associates selectors with declarations.
type Rule struct {
	Kind         RuleKind
	Selectors    []Selector
	Declarations []Declaration
	Order        int
}

// Declaration is one CSS property/value pair.
type Declaration struct {
	Property  string
	Value     Value
	Important bool
}

// Value is a declaration value before property-specific computation.
// Raw preserves the serialized input while Components exposes typed CSS tokens.
type Value struct {
	Raw        string
	Components []ComponentValue
}

// ComponentKind identifies a component value token.
type ComponentKind uint8

const (
	ComponentIdentifier ComponentKind = iota
	ComponentFunction
	ComponentAtKeyword
	ComponentHash
	ComponentString
	ComponentURL
	ComponentNumber
	ComponentPercentage
	ComponentDimension
	ComponentWhitespace
	ComponentDelimiter
	ComponentBlockStart
	ComponentBlockEnd
	ComponentComma
	ComponentBad
)

// ComponentValue is one typed token in a declaration value.
type ComponentValue struct {
	Kind ComponentKind
	Raw  string
}

// SelectorKind identifies the limited selector forms supported by the MVP.
type SelectorKind uint8

const (
	SelectorTag SelectorKind = iota
	SelectorClass
	SelectorID
	SelectorTagClass
)

// Selector is a parsed simple selector.
type Selector struct {
	Kind  SelectorKind
	Tag   string
	Class string
	ID    string
	Hover bool
}

// Specificity returns the selector's (ID, class, tag) specificity tuple.
func (s Selector) Specificity() [3]int {
	var result [3]int
	if s.ID != "" {
		result[0]++
	}
	if s.Class != "" {
		result[1]++
	}
	if s.Hover {
		result[1]++
	}
	if s.Tag != "" {
		result[2]++
	}
	return result
}
