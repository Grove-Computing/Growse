// Package style matches CSS rules and computes values for DOM elements.
package style

import (
	"github.com/Grove-Computing/Growse/internal/animation"
	"github.com/Grove-Computing/Growse/internal/dom"
)

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
	DisplayContents
	DisplayTable
	DisplayTableRowGroup
	DisplayTableRow
	DisplayTableCell
)

// Float places a box at a side of the current block formatting context.
type Float uint8

const (
	FloatNone Float = iota
	FloatLeft
	FloatRight
)

// Clear moves a box below preceding floats on the selected side.
type Clear uint8

const (
	ClearNone Clear = iota
	ClearLeft
	ClearRight
	ClearBoth
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

// Position selects normal, offset, out-of-flow, viewport, or sticky positioning.
type Position uint8

const (
	PositionStatic Position = iota
	PositionRelative
	PositionAbsolute
	PositionFixed
	PositionSticky
)

// Visibility controls painting and hit testing while preserving layout.
type Visibility uint8

const (
	VisibilityVisible Visibility = iota
	VisibilityHidden
)

// Insets stores the four computed inset properties; SizeAuto means auto.
type Insets struct {
	Top, Right, Bottom, Left SizeValue
}

// Shadow is one box-shadow or text-shadow layer.
type Shadow struct {
	Inset                  bool
	OffsetX, OffsetY, Blur float32
	Spread                 float32
	Color                  uint32
}

// Matrix is a CSS 2D affine matrix [A C E; B D F; 0 0 1].
type Matrix struct {
	A, B, C, D, E, F float32
}

// TransformFunctionKind identifies a parsed CSS 2D transform function.
type TransformFunctionKind uint8

const (
	TransformTranslate TransformFunctionKind = iota
	TransformScale
	TransformRotate
	TransformSkew
	TransformMatrix
)

// TransformFunction retains percentage translations until box geometry is known.
type TransformFunction struct {
	Kind             TransformFunctionKind
	X, Y             LengthPercentage
	A, B, C, D, E, F float32
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
	GridTrackAutoRepeat
)

// GridAutoRepeat identifies repeat(auto-fill, ...) and repeat(auto-fit, ...).
type GridAutoRepeat uint8

const (
	GridAutoRepeatNone GridAutoRepeat = iota
	GridAutoRepeatFill
	GridAutoRepeatFit
)

// GridTrackSize retains track sizing values until the grid container is laid out.
type GridTrackSize struct {
	Kind          GridTrackKind
	Value         LengthPercentage
	Flex          float32
	MinKind       GridTrackKind
	MinValue      LengthPercentage
	MinSet        bool
	FitLimit      *LengthPercentage
	AutoRepeat    GridAutoRepeat
	RepeatPattern []GridTrackSize
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

// GridAutoFlow selects the auto-placement major axis and optional backfilling.
type GridAutoFlow struct {
	Column bool
	Dense  bool
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

// TextAlign controls inline content alignment in a line box.
type TextAlign uint8

const (
	TextAlignStart TextAlign = iota
	TextAlignEnd
	TextAlignLeft
	TextAlignRight
	TextAlignCenter
	TextAlignJustify
)

// TextTransform controls the case transformation applied before shaping.
type TextTransform uint8

const (
	TextTransformNone TextTransform = iota
	TextTransformUppercase
	TextTransformLowercase
	TextTransformCapitalize
)

// WordBreak controls emergency break opportunities inside words.
type WordBreak uint8

const (
	WordBreakNormal WordBreak = iota
	WordBreakBreakAll
	WordBreakKeepAll
)

// OverflowWrap controls whether an otherwise unbreakable word may wrap.
type OverflowWrap uint8

const (
	OverflowWrapNormal OverflowWrap = iota
	OverflowWrapBreakWord
	OverflowWrapAnywhere
)

// VerticalAlignKind identifies the inline-axis alignment mode.
type VerticalAlignKind uint8

const (
	VerticalAlignBaseline VerticalAlignKind = iota
	VerticalAlignSub
	VerticalAlignSuper
	VerticalAlignMiddle
	VerticalAlignTextTop
	VerticalAlignTextBottom
	VerticalAlignTop
	VerticalAlignBottom
	VerticalAlignLength
)

// VerticalAlign stores either a keyword or a computed length. Positive
// lengths raise the inline box, as defined by CSS vertical-align.
type VerticalAlign struct {
	Kind  VerticalAlignKind
	Value float32
}

// TextOverflow controls the marker painted for clipped single-line text.
type TextOverflow uint8

const (
	TextOverflowClip TextOverflow = iota
	TextOverflowEllipsis
)

// ObjectFit selects how replaced content maps into its content box.
type ObjectFit uint8

const (
	ObjectFitFill ObjectFit = iota
	ObjectFitContain
	ObjectFitCover
	ObjectFitNone
	ObjectFitScaleDown
)

// ListStyleType is the bounded marker subset used by framework fixtures.
type ListStyleType uint8

const (
	ListStyleDisc ListStyleType = iota
	ListStyleCircle
	ListStyleSquare
	ListStyleDecimal
	ListStyleNone
)

// ListStylePosition controls whether a marker is inside or outside the item.
type ListStylePosition uint8

const (
	ListStyleOutside ListStylePosition = iota
	ListStyleInside
)

// Appearance controls native form-control chrome.
type Appearance uint8

const (
	AppearanceAuto Appearance = iota
	AppearanceNone
)

// Cursor is the implemented platform cursor subset.
type Cursor uint8

const (
	CursorAuto Cursor = iota
	CursorDefault
	CursorPointer
	CursorText
	CursorCrosshair
	CursorMove
	CursorGrab
	CursorGrabbing
	CursorNotAllowed
	CursorWait
	CursorProgress
	CursorColResize
	CursorRowResize
)

// FilterKind identifies a supported bounded CSS filter function.
type FilterKind uint8

const (
	FilterBlur FilterKind = iota
	FilterBrightness
	FilterContrast
	FilterGrayscale
	FilterHueRotate
	FilterInvert
	FilterOpacity
	FilterSaturate
	FilterSepia
	FilterDropShadow
)

// Filter is one validated filter function. Amount is a normalized scalar,
// Angle uses degrees, Radius uses CSS px, and Shadow is used by drop-shadow.
type Filter struct {
	Kind   FilterKind
	Amount float32
	Angle  float32
	Radius float32
	Shadow Shadow
}

// BlendMode is the fixture-supported mix-blend-mode subset.
type BlendMode uint8

const (
	BlendNormal BlendMode = iota
	BlendMultiply
	BlendScreen
	BlendOverlay
	BlendDarken
	BlendLighten
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
	BackgroundImageRadialGradient
)

// GradientStop is one color stop in a linear gradient. Position is normalized
// to the [0, 1] range after omitted stops have been distributed.
type GradientStop struct {
	Color    uint32
	Position float32
}

// BackgroundImage is either one URL image or one linear gradient.
type BackgroundImage struct {
	Kind           BackgroundImageKind
	URL            string
	GradientAngle  float32
	GradientStops  []GradientStop
	GradientCenter BackgroundPosition
	RadialCircle   bool
}

// BackgroundLayer groups one image with its layer-specific placement values.
type BackgroundLayer struct {
	Image    BackgroundImage
	Repeat   BackgroundRepeat
	Position BackgroundPosition
	Size     BackgroundSize
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
	// BrowserDefaults marks styles computed with the JavaScript-only browser
	// compatibility UA profile. Layout uses it to select the initial
	// containing-block behavior without changing the Go Engine defaults.
	BrowserDefaults     bool
	Color               uint32
	BackgroundColor     uint32
	BackgroundImage     BackgroundImage
	BackgroundRepeat    BackgroundRepeat
	BackgroundPos       BackgroundPosition
	BackgroundSize      BackgroundSize
	BackgroundLayers    []BackgroundLayer
	FontSize            float32
	FontWeight          int
	FontFamilies        []string
	FontStyle           string
	FontStretch         string
	FontFaceIndex       int
	LineHeight          float32
	WhiteSpace          WhiteSpace
	TextAlign           TextAlign
	TextTransform       TextTransform
	TextIndent          LengthPercentage
	LetterSpacing       float32
	WordSpacing         float32
	WordBreak           WordBreak
	OverflowWrap        OverflowWrap
	VerticalAlign       VerticalAlign
	TextOverflow        TextOverflow
	ObjectFit           ObjectFit
	ObjectPosition      BackgroundPosition
	ListStyleType       ListStyleType
	ListStylePosition   ListStylePosition
	ListStyleImage      string
	Appearance          Appearance
	AccentColor         uint32
	AccentColorAuto     bool
	Cursor              Cursor
	Filters             []Filter
	BackdropFilters     []Filter
	MixBlendMode        BlendMode
	OverflowX           Overflow
	OverflowY           Overflow
	Display             Display
	Float               Float
	Clear               Clear
	ContainerType       ContainerType
	ContainerName       string
	Visibility          Visibility
	FlexDirection       FlexDirection
	FlexWrap            FlexWrap
	JustifyContent      JustifyContent
	AlignItems          Align
	JustifyItems        Align
	AlignContent        Align
	Order               int
	FlexGrow            float32
	FlexShrink          float32
	FlexBasis           FlexBasis
	AlignSelf           Align
	JustifySelf         Align
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
	GridAutoFlow        GridAutoFlow
	Position            Position
	Inset               Insets
	ZIndex              int
	ZIndexAuto          bool
	BoxShadows          []Shadow
	TextShadows         []Shadow
	Outline             BorderSide
	OutlineOffset       float32
	Transform           []TransformFunction
	TransformOrigin     BackgroundPosition
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
	Transitions         []Transition
	Animations          []CSSAnimation
	ImportantProperties map[string]bool
	BeforeContent       string
	AfterContent        string
	CustomProperties    map[string]string
}

// ContainerType identifies size containment exposed to @container queries.
type ContainerType uint8

const (
	ContainerTypeNormal ContainerType = iota
	ContainerTypeInlineSize
)

// AnimationDirection controls iteration playback direction.
type AnimationDirection uint8

const (
	AnimationNormal AnimationDirection = iota
	AnimationReverse
	AnimationAlternate
	AnimationAlternateReverse
)

// AnimationFillMode controls the effect before and after active time.
type AnimationFillMode uint8

const (
	AnimationFillNone AnimationFillMode = iota
	AnimationFillForwards
	AnimationFillBackwards
	AnimationFillBoth
)

// AnimationPlayState controls whether an animation advances.
type AnimationPlayState uint8

const (
	AnimationRunning AnimationPlayState = iota
	AnimationPaused
)

// CSSAnimation is one computed entry from the animation property lists.
type CSSAnimation struct {
	Name       string
	Timing     animation.Timing
	Iterations float64
	Direction  AnimationDirection
	FillMode   AnimationFillMode
	PlayState  AnimationPlayState
}

// Transition is one computed CSS transition matched to a property.
type Transition struct {
	Property string
	Timing   animation.Timing
}

// Bold reports whether the computed weight should use a bold face.
func (s ComputedStyle) Bold() bool {
	return s.FontWeight >= 600
}

// Important reports whether the winning author declaration for property was
// marked !important.
func (s ComputedStyle) Important(property string) bool {
	return s.ImportantProperties[property]
}

// Map stores computed styles by DOM NodeID.
type Map map[dom.NodeID]ComputedStyle

// InteractionState contains transient browser state used by selector matching.
type InteractionState struct {
	Hovered      map[dom.NodeID]bool
	Focused      dom.NodeID
	FocusVisible bool
	Scope        dom.NodeID
	Document     *dom.Document
}

// For returns a node's computed style and whether one was calculated.
func (m Map) For(node *dom.Node) (ComputedStyle, bool) {
	if node == nil {
		return ComputedStyle{}, false
	}
	value, ok := m[node.ID]
	return value, ok
}
