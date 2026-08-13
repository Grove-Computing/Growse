// Package style matches CSS rules and computes values for DOM elements.
package style

import "github.com/saku0512/growse/internal/dom"

// Display controls whether an element participates in block or inline layout.
type Display uint8

const (
	DisplayInline Display = iota
	DisplayBlock
	DisplayInlineBlock
	DisplayNone
)

// BoxSizing controls whether declared sizes include padding and border.
type BoxSizing uint8

const (
	BoxSizingContentBox BoxSizing = iota
	BoxSizingBorderBox
)

// SizeKind identifies auto, a length-percentage, or an unbounded maximum.
type SizeKind uint8

const (
	SizeAuto SizeKind = iota
	SizeLength
	SizeNone
)

// SizeValue is a computed sizing property which still may contain a percentage.
type SizeValue struct {
	Kind  SizeKind
	Value LengthPercentage
}

// Edges contains resolved pixel values in CSS clockwise order.
type Edges struct {
	Top    float32
	Right  float32
	Bottom float32
	Left   float32
}

// BorderStyle is the line style of one border side.
type BorderStyle uint8

const (
	BorderNone BorderStyle = iota
	BorderSolid
	BorderDotted
	BorderDashed
	BorderDouble
)

// BorderSide contains the computed width, style and color of one side.
type BorderSide struct {
	Width float32
	Style BorderStyle
	Color uint32
}

// Borders contains border sides in CSS clockwise order.
type Borders struct {
	Top    BorderSide
	Right  BorderSide
	Bottom BorderSide
	Left   BorderSide
}

// WhiteSpace controls collapsing, newline preservation and wrapping.
type WhiteSpace uint8

const (
	WhiteSpaceNormal WhiteSpace = iota
	WhiteSpaceNowrap
	WhiteSpacePre
	WhiteSpacePreWrap
	WhiteSpacePreLine
)

// ComputedStyle contains the MVP properties consumed by layout and paint.
type ComputedStyle struct {
	Color            uint32
	BackgroundColor  uint32
	FontSize         float32
	FontWeight       int
	LineHeight       float32
	WhiteSpace       WhiteSpace
	Display          Display
	BoxSizing        BoxSizing
	Width            SizeValue
	Height           SizeValue
	MinWidth         SizeValue
	MinHeight        SizeValue
	MaxWidth         SizeValue
	MaxHeight        SizeValue
	Margin           Edges
	Padding          Edges
	Border           Borders
	BeforeContent    string
	AfterContent     string
	CustomProperties map[string]string
}

// Bold reports whether the computed weight should use a bold face.
func (s ComputedStyle) Bold() bool {
	return s.FontWeight >= 600
}

// Map stores computed styles by DOM NodeID.
type Map map[dom.NodeID]ComputedStyle

// InteractionState contains transient browser state used by selector matching.
type InteractionState struct {
	Hovered map[dom.NodeID]bool
	Focused dom.NodeID
}

// For returns a node's computed style and whether one was calculated.
func (m Map) For(node *dom.Node) (ComputedStyle, bool) {
	if node == nil {
		return ComputedStyle{}, false
	}
	value, ok := m[node.ID]
	return value, ok
}
