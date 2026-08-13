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
	SelectorUniversal
	SelectorCompound
)

// CompoundSelector is a sequence of simple selectors without a combinator.
type CompoundSelector struct {
	Universal  bool
	Type       string
	IDs        []string
	Classes    []string
	Attributes []AttributeSelector
	Hover      bool
}

// AttributeMatcher identifies how an attribute value is compared.
type AttributeMatcher uint8

const (
	AttributePresent AttributeMatcher = iota
	AttributeExact
	AttributeIncludes
	AttributeDashMatch
	AttributePrefix
	AttributeSuffix
	AttributeSubstring
)

// AttributeSelector is one attribute condition in a compound selector.
type AttributeSelector struct {
	Name    string
	Matcher AttributeMatcher
	Value   string
}

// Combinator describes the relationship between two adjacent compounds.
type Combinator uint8

const (
	CombinatorDescendant Combinator = iota
	CombinatorChild
	CombinatorAdjacentSibling
	CombinatorGeneralSibling
)

// Selector is a parsed selector. The legacy fields remain populated for the
// original four selector forms while Compounds is the canonical AST.
type Selector struct {
	Kind        SelectorKind
	Tag         string
	Class       string
	ID          string
	Hover       bool
	Compounds   []CompoundSelector
	Combinators []Combinator
}

// Specificity returns the selector's (ID, class, tag) specificity tuple.
func (s Selector) Specificity() [3]int {
	var result [3]int
	if len(s.Compounds) != 0 {
		for _, compound := range s.Compounds {
			result[0] += len(compound.IDs)
			result[1] += len(compound.Classes) + len(compound.Attributes)
			if compound.Hover {
				result[1]++
			}
			if compound.Type != "" {
				result[2]++
			}
		}
		return result
	}
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
