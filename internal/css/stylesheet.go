// Package css parses CSS syntax into Growse-owned rules.
package css

// Stylesheet is an ordered collection of CSS rules.
type Stylesheet struct {
	Rules []Rule
}

// Rule associates selectors with declarations.
type Rule struct {
	Selectors    []Selector
	Declarations []Declaration
	Order        int
}

// Declaration is one CSS property/value pair.
type Declaration struct {
	Property  string
	Value     string
	Important bool
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
	if s.Tag != "" {
		result[2]++
	}
	return result
}
