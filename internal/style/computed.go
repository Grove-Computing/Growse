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
	DisplayFlex
	DisplayInlineFlex
	DisplayGrid
	DisplayInlineGrid
)

// FlexDirection defines the main axis and its direction.
type FlexDirection uint8

const (
	FlexDirectionRow FlexDirection = iota
	FlexDirectionRowReverse
	FlexDirectionColumn
	FlexDirectionColumnReverse
)

// FlexWrap controls whether flex items form multiple lines.
type FlexWrap uint8

const (
	FlexNoWrap FlexWrap = iota
	FlexWrapLines
	FlexWrapReverse
)

// JustifyContent distributes free space on the main axis.
type JustifyContent uint8

const (
	JustifyFlexStart JustifyContent = iota
	JustifyFlexEnd
	JustifyCenter
	JustifySpaceBetween
	JustifySpaceAround
	JustifySpaceEvenly
)

// Align controls cross-axis alignment. AlignAuto is only valid for align-self.
type Align uint8

const (
	AlignStretch Align = iota
	AlignFlexStart
	AlignFlexEnd
	AlignCenter
	AlignBaseline
	AlignSpaceBetween
	AlignSpaceAround
	AlignSpaceEvenly
	AlignAuto
)

// FlexBasisKind identifies an automatic, content-based, or sized flex basis.
type FlexBasisKind uint8

const (
	FlexBasisAuto FlexBasisKind = iota
	FlexBasisContent
	FlexBasisLength
)

// FlexBasis retains percentages until the flex container size is known.
type FlexBasis struct {
	Kind  FlexBasisKind
	Value LengthPercentage
}

// AutoEdges records which margins have the auto keyword.
type AutoEdges struct {
	Top, Right, Bottom, Left bool
}

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

// GridTrackKind identifies the sizing function used by one grid track.
type GridTrackKind uint8

const (
	GridTrackAuto GridTrackKind = iota
	GridTrackLength
	GridTrackFraction
	GridTrackMinContent
	GridTrackMaxContent
)

// GridTrackSize retains track sizing values until the grid container is laid out.
type GridTrackSize struct {
	Kind     GridTrackKind
	Value    LengthPercentage
	Flex     float32
	MinKind  GridTrackKind
	MinValue LengthPercentage
	MinSet   bool
	FitLimit *LengthPercentage
}

// GridLine selects a numbered/named line or spans tracks from the opposite edge.
type GridLine struct {
	Index int
	Name  string
	Span  int
}

// GridPlacement stores the two edges of one grid axis.
type GridPlacement struct {
	Start GridLine
	End   GridLine
}

// GridArea is a zero-based half-open rectangle derived from grid-template-areas.
type GridArea struct {
	RowStart, RowEnd, ColumnStart, ColumnEnd int
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

// RadiusValue is one possibly elliptical corner radius.
type RadiusValue struct {
	X LengthPercentage
	Y LengthPercentage
}

// BorderRadii contains corner radii in clockwise order.
type BorderRadii struct {
	TopLeft     RadiusValue
	TopRight    RadiusValue
	BottomRight RadiusValue
	BottomLeft  RadiusValue
}

// TextDecorationLine is a bit set of line decorations.
type TextDecorationLine uint8

const (
	TextDecorationNone        TextDecorationLine = 0
	TextDecorationUnderline   TextDecorationLine = 1 << 0
	TextDecorationOverline    TextDecorationLine = 1 << 1
	TextDecorationLineThrough TextDecorationLine = 1 << 2
)

// WhiteSpace controls collapsing, newline preservation and wrapping.
type WhiteSpace uint8

const (
	WhiteSpaceNormal WhiteSpace = iota
	WhiteSpaceNowrap
	WhiteSpacePre
	WhiteSpacePreWrap
	WhiteSpacePreLine
)

// Overflow controls clipping and scroll-container creation on one axis.
type Overflow uint8

const (
	OverflowVisible Overflow = iota
	OverflowHidden
	OverflowAuto
	OverflowScroll
)

// BackgroundImageKind identifies the single background layer supported by Growse.
type BackgroundImageKind uint8

const (
	BackgroundImageNone BackgroundImageKind = iota
	BackgroundImageURL
	BackgroundImageLinearGradient
)

// GradientStop is one color stop in a linear gradient. Position is normalized
// to the [0, 1] range after omitted stops have been distributed.
type GradientStop struct {
	Color    uint32
	Position float32
}

// BackgroundImage is either one URL image or one linear gradient.
type BackgroundImage struct {
	Kind          BackgroundImageKind
	URL           string
	GradientAngle float32
	GradientStops []GradientStop
}

// BackgroundRepeat stores repetition independently for each axis.
type BackgroundRepeat struct {
	X bool
	Y bool
}

// BackgroundPosition is a position inside the background positioning area.
type BackgroundPosition struct {
	X LengthPercentage
	Y LengthPercentage
}

// BackgroundSizeKind identifies automatic, intrinsic-ratio, or explicit sizing.
type BackgroundSizeKind uint8

const (
	BackgroundSizeAuto BackgroundSizeKind = iota
	BackgroundSizeCover
	BackgroundSizeContain
	BackgroundSizeExplicit
)

// BackgroundSize contains the size of the single background layer.
type BackgroundSize struct {
	Kind   BackgroundSizeKind
	Width  SizeValue
	Height SizeValue
}

// ComputedStyle contains the MVP properties consumed by layout and paint.
type ComputedStyle struct {
	Color               uint32
	BackgroundColor     uint32
	BackgroundImage     BackgroundImage
	BackgroundRepeat    BackgroundRepeat
	BackgroundPos       BackgroundPosition
	BackgroundSize      BackgroundSize
	FontSize            float32
	FontWeight          int
	LineHeight          float32
	WhiteSpace          WhiteSpace
	OverflowX           Overflow
	OverflowY           Overflow
	Display             Display
	FlexDirection       FlexDirection
	FlexWrap            FlexWrap
	JustifyContent      JustifyContent
	AlignItems          Align
	AlignContent        Align
	Order               int
	FlexGrow            float32
	FlexShrink          float32
	FlexBasis           FlexBasis
	AlignSelf           Align
	RowGap              LengthPercentage
	ColumnGap           LengthPercentage
	GridTemplateColumns []GridTrackSize
	GridTemplateRows    []GridTrackSize
	GridAutoColumns     []GridTrackSize
	GridAutoRows        []GridTrackSize
	GridColumnLines     map[string][]int
	GridRowLines        map[string][]int
	GridTemplateAreas   map[string]GridArea
	GridColumn          GridPlacement
	GridRow             GridPlacement
	GridAreaName        string
	AspectRatio         float32
	BoxSizing           BoxSizing
	Width               SizeValue
	Height              SizeValue
	MinWidth            SizeValue
	MinHeight           SizeValue
	MaxWidth            SizeValue
	MaxHeight           SizeValue
	Margin              Edges
	MarginAuto          AutoEdges
	Padding             Edges
	Border              Borders
	BorderRadius        BorderRadii
	TextDecoration      TextDecorationLine
	DecorationColor     uint32
	Opacity             float32
	BeforeContent       string
	AfterContent        string
	CustomProperties    map[string]string
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
