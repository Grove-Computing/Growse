// Package css parses CSS syntax into Growse-owned rules.
package css

const (
	// MaxKeyframesRules bounds named keyframe storage per stylesheet.
	MaxKeyframesRules = 256
	// MaxFramesPerKeyframesRule bounds selector blocks in one @keyframes rule.
	MaxFramesPerKeyframesRule = 256
	// MaxDeclarationsPerKeyframe bounds work performed for one sampled frame.
	MaxDeclarationsPerKeyframe = 64
	// MaxOffsetsPerKeyframe bounds comma-separated selectors in one block.
	MaxOffsetsPerKeyframe = 64
)

// Stylesheet is an ordered collection of CSS rules.
type Stylesheet struct {
	Rules     []Rule
	Imports   []ImportRule
	Keyframes []KeyframesRule
}

// KeyframesRule is one named CSS animation keyframe list.
type KeyframesRule struct {
	Name   string
	Frames []Keyframe
}

// Keyframe contains normalized offsets and declarations for one keyframe block.
type Keyframe struct {
	Offsets      []float64
	Declarations []Declaration
}

// ImportRule references a stylesheet and an optional media query list.
type ImportRule struct {
	URL   string
	Media []MediaQuery
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
	Media        [][]MediaQuery
}

// MediaModifier changes how one media query is interpreted.
type MediaModifier uint8

const (
	MediaModifierNone MediaModifier = iota
	MediaModifierNot
	MediaModifierOnly
)

// MediaQuery is one comma-separated media query. Features are combined by and.
type MediaQuery struct {
	Modifier MediaModifier
	Type     string
	Features []MediaFeature
}

// MediaFeature is a media feature name and its optional value.
type MediaFeature struct {
	Name  string
	Value string
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
	Universal     bool
	Type          string
	IDs           []string
	Classes       []string
	Attributes    []AttributeSelector
	Pseudos       []PseudoClass
	Hover         bool
	PseudoElement PseudoElementKind
}

// PseudoElementKind identifies generated content attached to an element.
type PseudoElementKind uint8

const (
	PseudoElementNone PseudoElementKind = iota
	PseudoElementBefore
	PseudoElementAfter
)

// PseudoClassKind identifies a supported pseudo-class condition.
type PseudoClassKind uint8

const (
	PseudoRoot PseudoClassKind = iota
	PseudoEmpty
	PseudoFirstChild
	PseudoLastChild
	PseudoOnlyChild
	PseudoFirstOfType
	PseudoLastOfType
	PseudoOnlyOfType
	PseudoNthChild
	PseudoNthLastChild
	PseudoNthOfType
	PseudoNthLastOfType
	PseudoNot
	PseudoLink
	PseudoFocus
	PseudoEnabled
	PseudoDisabled
	PseudoChecked
	PseudoValid
	PseudoInvalid
)

// PseudoClass stores a pseudo-class and its parsed arguments. Nth expressions
// use A*n+B, while :not() stores its Level 3 simple-selector argument.
type PseudoClass struct {
	Kind     PseudoClassKind
	A        int
	B        int
	Negation *CompoundSelector
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
			addSpecificity(&result, compoundSpecificity(compound))
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

func compoundSpecificity(compound CompoundSelector) [3]int {
	result := [3]int{len(compound.IDs), len(compound.Classes) + len(compound.Attributes), 0}
	if compound.Hover {
		result[1]++
	}
	if compound.Type != "" {
		result[2]++
	}
	if compound.PseudoElement != PseudoElementNone {
		result[2]++
	}
	for _, pseudo := range compound.Pseudos {
		if pseudo.Kind == PseudoNot && pseudo.Negation != nil {
			addSpecificity(&result, compoundSpecificity(*pseudo.Negation))
		} else {
			result[1]++
		}
	}
	return result
}

func addSpecificity(target *[3]int, addition [3]int) {
	for index := range target {
		target[index] += addition[index]
	}
}
