package layout

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/forms"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

const (
	pagePadding    = float32(32)
	textColor      = uint32(0x202124ff)
	linkColor      = uint32(0x0969daff)
	inputWidth     = float32(280)
	inputHeight    = float32(40)
	checkableSize  = float32(32)
	buttonWidth    = float32(120)
	textareaHeight = float32(96)
)

type blockStyle struct {
	fonts               *FontSet
	fontSize            float32
	bold                bool
	fontFamilies        []string
	fontStyle           string
	fontStretch         string
	color               uint32
	background          uint32
	image               stylemodel.BackgroundImage
	repeat              stylemodel.BackgroundRepeat
	position            stylemodel.BackgroundPosition
	backgroundSize      stylemodel.BackgroundSize
	backgroundLayers    []stylemodel.BackgroundLayer
	layoutPosition      stylemodel.Position
	inset               stylemodel.Insets
	zIndex              int
	zIndexAuto          bool
	boxShadows          []stylemodel.Shadow
	textShadows         []stylemodel.Shadow
	outline             stylemodel.BorderSide
	outlineOffset       float32
	transform           []stylemodel.TransformFunction
	transformOrigin     stylemodel.BackgroundPosition
	radius              stylemodel.BorderRadii
	decoration          stylemodel.TextDecorationLine
	decorationColor     uint32
	opacity             float32
	display             stylemodel.Display
	float               stylemodel.Float
	clear               stylemodel.Clear
	hidden              bool
	margin              stylemodel.Edges
	padding             stylemodel.Edges
	border              stylemodel.Borders
	boxSizing           stylemodel.BoxSizing
	width               stylemodel.SizeValue
	height              stylemodel.SizeValue
	minWidth            stylemodel.SizeValue
	minHeight           stylemodel.SizeValue
	maxWidth            stylemodel.SizeValue
	maxHeight           stylemodel.SizeValue
	lineHeight          float32
	whiteSpace          stylemodel.WhiteSpace
	textAlign           stylemodel.TextAlign
	textTransform       stylemodel.TextTransform
	textIndent          stylemodel.LengthPercentage
	letterSpacing       float32
	wordSpacing         float32
	wordBreak           stylemodel.WordBreak
	overflowWrap        stylemodel.OverflowWrap
	verticalAlign       stylemodel.VerticalAlign
	textOverflow        stylemodel.TextOverflow
	objectFit           stylemodel.ObjectFit
	objectPosition      stylemodel.BackgroundPosition
	listStyleType       stylemodel.ListStyleType
	listStylePosition   stylemodel.ListStylePosition
	listStyleImage      string
	appearance          stylemodel.Appearance
	accentColor         uint32
	accentColorAuto     bool
	cursor              stylemodel.Cursor
	filters             []stylemodel.Filter
	backdropFilters     []stylemodel.Filter
	mixBlendMode        stylemodel.BlendMode
	overflowX           stylemodel.Overflow
	overflowY           stylemodel.Overflow
	flexDirection       stylemodel.FlexDirection
	flexWrap            stylemodel.FlexWrap
	justifyContent      stylemodel.JustifyContent
	alignItems          stylemodel.Align
	justifyItems        stylemodel.Align
	alignContent        stylemodel.Align
	order               int
	flexGrow            float32
	flexShrink          float32
	flexBasis           stylemodel.FlexBasis
	alignSelf           stylemodel.Align
	justifySelf         stylemodel.Align
	rowGap              stylemodel.LengthPercentage
	columnGap           stylemodel.LengthPercentage
	gridTemplateColumns []stylemodel.GridTrackSize
	gridTemplateRows    []stylemodel.GridTrackSize
	gridAutoColumns     []stylemodel.GridTrackSize
	gridAutoRows        []stylemodel.GridTrackSize
	gridColumnLines     map[string][]int
	gridRowLines        map[string][]int
	gridTemplateAreas   map[string]stylemodel.GridArea
	gridColumn          stylemodel.GridPlacement
	gridRow             stylemodel.GridPlacement
	gridAreaName        string
	gridAutoFlow        stylemodel.GridAutoFlow
	marginAuto          stylemodel.AutoEdges
	aspectRatio         float32
}

type inlineRun struct {
	nodeID      dom.NodeID
	node        *dom.Node
	tag         string
	text        string
	style       blockStyle
	atomic      bool
	flex        bool
	grid        bool
	width       float32
	widthOffset float32
	height      float32
	baseline    float32
	opacity     float32
}

// Build creates a vertical block layout with a minimal inline text flow.
func Build(document *dom.Document, computed stylemodel.Map, viewportWidth float32) *Tree {
	return build(document, computed, nil, nil, viewportWidth, 0, 0, 0)
}

// BuildAtRevision lays out one immutable DOM/style revision.
func BuildAtRevision(document *dom.Document, computed stylemodel.Map, viewportWidth float32, revision uint64) *Tree {
	tree := Build(document, computed, viewportWidth)
	tree.Revision = revision
	return tree
}

// BuildWithViewport lays out a document with definite viewport dimensions.
func BuildWithViewport(document *dom.Document, computed stylemodel.Map, viewportWidth, viewportHeight float32) *Tree {
	return build(document, computed, nil, nil, viewportWidth, viewportHeight, 0, 0)
}

// BuildWithScroll lays out viewport-attached and sticky elements at a scroll offset.
func BuildWithScroll(document *dom.Document, computed stylemodel.Map, viewportWidth, viewportHeight, scrollX, scrollY float32) *Tree {
	return build(document, computed, nil, nil, viewportWidth, viewportHeight, max(scrollX, float32(0)), max(scrollY, float32(0)))
}

// BuildWithScrollAndImages lays out browser-decoded replaced image metadata.
func BuildWithScrollAndImages(document *dom.Document, computed stylemodel.Map, images map[dom.NodeID]ImageResource, viewportWidth, viewportHeight, scrollX, scrollY float32) *Tree {
	return build(document, computed, images, nil, viewportWidth, viewportHeight, max(scrollX, float32(0)), max(scrollY, float32(0)))
}

// BuildWithScrollAndResources lays out decoded images and page-scoped web fonts.
func BuildWithScrollAndResources(document *dom.Document, computed stylemodel.Map, images map[dom.NodeID]ImageResource, fonts *FontSet, viewportWidth, viewportHeight, scrollX, scrollY float32) *Tree {
	return build(document, computed, images, fonts, viewportWidth, viewportHeight, max(scrollX, float32(0)), max(scrollY, float32(0)))
}

// BuildWithScrollAtRevision lays out one immutable DOM/style revision at a scroll offset.
func BuildWithScrollAtRevision(document *dom.Document, computed stylemodel.Map, viewportWidth, viewportHeight, scrollX, scrollY float32, revision uint64) *Tree {
	tree := BuildWithScroll(document, computed, viewportWidth, viewportHeight, scrollX, scrollY)
	tree.Revision = revision
	return tree
}

func build(document *dom.Document, computed stylemodel.Map, images map[dom.NodeID]ImageResource, fonts *FontSet, viewportWidth, viewportHeight, scrollX, scrollY float32) *Tree {
	pageInset := pagePadding
	if usesBrowserViewport(document, computed) {
		pageInset = 0
	}
	if viewportWidth < pageInset*2+1 {
		viewportWidth = pageInset*2 + 1
	}

	tree := &Tree{
		Width: viewportWidth, Background: 0xffffffff, StackingContexts: []StackingContext{{Parent: -1}},
		Parents: make(map[dom.NodeID]dom.NodeID), Bounds: make(map[dom.NodeID]Rect),
	}
	recordNodeParents(tree, document)
	state := engine{
		tree:           tree,
		computed:       computed,
		images:         images,
		fonts:          fonts,
		y:              pageInset,
		opacity:        1,
		viewportWidth:  viewportWidth,
		viewportHeight: viewportHeight,
		scrollX:        scrollX,
		scrollY:        scrollY,
	}
	if document != nil {
		if body := findElement(document.Root, "body"); body != nil {
			if bodyStyle, ok := computed.For(body); ok && bodyStyle.BackgroundColor != 0 {
				tree.Background = bodyStyle.BackgroundColor
			}
		}
		state.walk(document.Root, pageInset, viewportWidth-pageInset*2, viewportHeight, viewportHeight > 0)
	}
	tree.Height = state.y + pageInset
	tree.ScrollWidth, tree.ScrollHeight = tree.Width, tree.Height
	for _, box := range tree.Boxes {
		contentWidth := box.Width
		if len(box.Runs) != 0 {
			contentWidth = 0
			for _, run := range box.Runs {
				contentWidth += run.Width
			}
		}
		tree.ScrollWidth = max(tree.ScrollWidth, box.X+contentWidth+pageInset)
		tree.ScrollHeight = max(tree.ScrollHeight, box.Y+box.Height+pageInset)
	}
	for _, decoration := range tree.Decorations {
		tree.ScrollWidth = max(tree.ScrollWidth, decoration.X+decoration.Width+pageInset)
		tree.ScrollHeight = max(tree.ScrollHeight, decoration.Y+decoration.Height+pageInset)
	}
	return tree
}

func usesBrowserViewport(document *dom.Document, computed stylemodel.Map) bool {
	if document == nil {
		return false
	}
	html := findElement(document.Root, "html")
	if html == nil {
		return false
	}
	computedStyle, ok := computed.For(html)
	return ok && computedStyle.BrowserDefaults
}

type engine struct {
	tree                          *Tree
	computed                      stylemodel.Map
	images                        map[dom.NodeID]ImageResource
	fonts                         *FontSet
	y                             float32
	clip                          *Rect
	clips                         []ClipRegion
	order                         int
	opacity                       float32
	viewportWidth, viewportHeight float32
	scrollX, scrollY              float32
	positionCB                    *Rect
	stackingID                    int
	floats                        []floatRegion
}

func (e *engine) nextOrder() int {
	result := e.order
	e.order++
	return result
}

func (e *engine) walk(node *dom.Node, x, width, containingHeight float32, heightDefinite bool) {
	if node == nil {
		return
	}
	switch node.Type {
	case dom.NodeText:
		text := normalizeWhitespace(node.Text)
		if text != "" {
			textStyle := defaultStyle()
			if computed, ok := e.computed.For(node); ok {
				textStyle = applyComputed(textStyle, computed)
			}
			textStyle.fonts = e.fonts
			e.addText(node.ID, "text", text, textStyle, x, width)
		}
		return
	case dom.NodeElement:
		style := e.styleFor(node)
		if style.display == stylemodel.DisplayNone {
			return
		}
		if style.display == stylemodel.DisplayContents || style.display == stylemodel.DisplayTableRowGroup || style.display == stylemodel.DisplayTableRow {
			for _, child := range node.Children {
				e.walk(child, x, width, containingHeight, heightDefinite)
			}
			return
		}
		// Top-level positioned elements do not pass through a block parent's
		// positioned-child collection. Resolve them against the current
		// containing block (or the initial containing block when it is nil)
		// instead of accidentally laying them out in normal flow.
		if style.layoutPosition == stylemodel.PositionAbsolute || style.layoutPosition == stylemodel.PositionFixed {
			e.renderPositionedChild(node, style)
			return
		}
		if isImageElement(node, e.images) {
			e.addImage(node, style, x, width, containingHeight, heightDefinite)
			return
		}
		if isEditableTextControl(node) {
			e.addInput(node, style, x, width, containingHeight, heightDefinite)
			return
		}
		if isSelectControl(node) {
			e.addSelect(node, style, x, width, containingHeight, heightDefinite)
			return
		}
		if isCheckableControl(node) {
			e.addCheckable(node, style, x, width, containingHeight, heightDefinite)
			return
		}
		if isSubmitButtonControl(node) {
			e.addSubmitButton(node, style, x, width, containingHeight, heightDefinite)
			return
		}
		if style.display == stylemodel.DisplayTable {
			e.addTable(node, style, x, width, containingHeight, heightDefinite)
			return
		}
		if isBlockLevelDisplay(style.display) {
			e.addBlock(node, style, x, width, containingHeight, heightDefinite, nil)
			return
		}
		runs := e.collectInlineRuns(node, node)
		if len(runs) != 0 {
			e.addInlineRuns(node.ID, node.TagName, runs, style, x, width)
		}
		return
	}

	for _, child := range node.Children {
		e.walk(child, x, width, containingHeight, heightDefinite)
	}
}

func (e *engine) addInput(node *dom.Node, style blockStyle, x, width, containingHeight float32, heightDefinite bool) {
	e.y += style.margin.Top
	x += style.margin.Left
	availableWidth := width - style.margin.Left - style.margin.Right
	usedWidth := inputWidth
	if resolved, ok := resolveSize(style.width, availableWidth, true); ok {
		usedWidth = resolved
	}
	usedWidth = constrainSize(usedWidth, style.minWidth, style.maxWidth, availableWidth, true)
	if usedWidth > availableWidth && style.width.Kind == stylemodel.SizeAuto {
		usedWidth = availableWidth
	}
	if usedWidth < 1 {
		usedWidth = 1
	}
	usedHeight := inputHeight
	multiline := node.TagName == "textarea"
	if multiline {
		usedHeight = textareaHeight
	}
	if resolved, ok := resolveSize(style.height, containingHeight, heightDefinite); ok {
		usedHeight = resolved
	}
	usedHeight = constrainSize(usedHeight, style.minHeight, style.maxHeight, containingHeight, heightDefinite)
	value := forms.CurrentValue(node)
	inputType, _ := forms.EditableTextControlType(node)
	e.tree.Boxes = append(e.tree.Boxes, Box{
		Order:       e.nextOrder(),
		StackingID:  e.stackingID,
		NodeID:      node.ID,
		Tag:         node.TagName,
		Text:        value,
		Input:       true,
		Multiline:   multiline,
		InputType:   inputType,
		Disabled:    forms.Disabled(node),
		ReadOnly:    forms.ReadOnly(node),
		Appearance:  style.appearance,
		AccentColor: resolvedAccentColor(style),
		Cursor:      style.cursor,
		X:           x,
		Y:           e.y,
		Width:       usedWidth,
		Height:      usedHeight,
		Color:       style.color,
		Clip:        cloneRect(e.clip),
		Clips:       cloneClipRegions(e.clips),
		Opacity:     e.opacity * style.opacity,
		TextShadows: append([]stylemodel.Shadow(nil), style.textShadows...),
		Transform:   stylemodel.IdentityMatrix(),
		Hidden:      style.hidden,
	})
	e.tree.Bounds[node.ID] = Rect{X: x, Y: e.y, Width: usedWidth, Height: usedHeight}
	e.y += usedHeight + style.margin.Bottom
}

func isEditableTextControl(node *dom.Node) bool {
	return forms.IsEditableTextControl(node)
}

func isImageElement(node *dom.Node, resources map[dom.NodeID]ImageResource) bool {
	return node != nil && resources != nil && (node.TagName == "img" || node.TagName == "svg")
}

func (e *engine) addImage(node *dom.Node, style blockStyle, x, width, containingHeight float32, heightDefinite bool) {
	resource, loaded := e.images[node.ID]
	if !loaded {
		alt, _ := node.Attribute("alt")
		resource = ImageResource{Alt: alt, Error: "image resource is unavailable"}
	}
	e.y += style.margin.Top
	x += style.margin.Left
	availableWidth := max(width-style.margin.Left-style.margin.Right, float32(1))
	attributeWidth, hasAttributeWidth := imageDimensionAttribute(node, "width")
	attributeHeight, hasAttributeHeight := imageDimensionAttribute(node, "height")
	intrinsicWidth, intrinsicHeight := resource.IntrinsicWidth, resource.IntrinsicHeight
	if intrinsicWidth <= 0 || intrinsicHeight <= 0 {
		textWidth, textHeight, _ := measureStyledText(resource.Alt, style)
		intrinsicWidth, intrinsicHeight = max(textWidth+8, float32(16)), max(textHeight, style.lineHeight, float32(16))
	}
	ratio := style.aspectRatio
	if ratio <= 0 && resource.IntrinsicWidth > 0 && resource.IntrinsicHeight > 0 {
		ratio = resource.IntrinsicWidth / resource.IntrinsicHeight
	}
	contentWidth, widthSpecified := resolveSize(style.width, availableWidth, true)
	contentHeight, heightSpecified := resolveSize(style.height, containingHeight, heightDefinite)
	if !widthSpecified {
		if hasAttributeWidth {
			contentWidth = attributeWidth
		} else {
			contentWidth = intrinsicWidth
		}
	}
	if !heightSpecified {
		if hasAttributeHeight {
			contentHeight = attributeHeight
		} else {
			contentHeight = intrinsicHeight
		}
	}
	if ratio > 0 {
		if widthSpecified || hasAttributeWidth {
			if !heightSpecified && !hasAttributeHeight {
				contentHeight = contentWidth / ratio
			}
		} else if heightSpecified || hasAttributeHeight {
			contentWidth = contentHeight * ratio
		}
	}
	contentWidth = constrainSize(contentWidth, style.minWidth, style.maxWidth, availableWidth, true)
	contentHeight = constrainSize(contentHeight, style.minHeight, style.maxHeight, containingHeight, heightDefinite)
	horizontal := style.padding.Left + style.padding.Right + style.border.Left.Width + style.border.Right.Width
	vertical := style.padding.Top + style.padding.Bottom + style.border.Top.Width + style.border.Bottom.Width
	outerWidth, outerHeight := contentWidth, contentHeight
	if style.boxSizing == stylemodel.BoxSizingContentBox {
		outerWidth += horizontal
		outerHeight += vertical
	} else {
		contentWidth = max(outerWidth-horizontal, float32(0))
		contentHeight = max(outerHeight-vertical, float32(0))
	}
	contentX := x + style.border.Left.Width + style.padding.Left
	contentY := e.y + style.border.Top.Width + style.padding.Top
	imageRect := fitImageRect(contentX, contentY, contentWidth, contentHeight, resource.IntrinsicWidth, resource.IntrinsicHeight, style.objectFit, style.objectPosition)
	box := Box{
		Order: e.nextOrder(), StackingID: e.stackingID, NodeID: node.ID, Tag: node.TagName,
		Image: true, ImageURL: resource.URL, Alt: resource.Alt, ImageRect: imageRect,
		ImageClip: Rect{X: contentX, Y: contentY, Width: contentWidth, Height: contentHeight}, ImageFailed: !resource.Loaded,
		ObjectFit: style.objectFit, ObjectPos: style.objectPosition, ImageBorder: style.border, ImageRadius: resolveBorderRadii(style.radius, outerWidth, outerHeight),
		X: x, Y: e.y, Width: max(outerWidth, float32(1)), Height: max(outerHeight, float32(1)),
		FontSize: style.fontSize, FontFamilies: append([]string(nil), style.fontFamilies...), Bold: style.bold, Color: style.color, Background: style.background,
		Clip: cloneRect(e.clip), Clips: cloneClipRegions(e.clips), Opacity: e.opacity * style.opacity, Cursor: style.cursor,
		Transform: stylemodel.IdentityMatrix(), Hidden: style.hidden,
	}
	e.tree.Boxes = append(e.tree.Boxes, box)
	e.tree.Bounds[node.ID] = Rect{X: x, Y: e.y, Width: box.Width, Height: box.Height}
	e.y += box.Height + style.margin.Bottom
}

func imageDimensionAttribute(node *dom.Node, name string) (float32, bool) {
	raw, ok := node.Attribute(name)
	if !ok {
		return 0, false
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 32)
	if err != nil || value <= 0 || value > 32768 {
		return 0, false
	}
	return float32(value), true
}

func fitImageRect(x, y, width, height, intrinsicWidth, intrinsicHeight float32, fit stylemodel.ObjectFit, position stylemodel.BackgroundPosition) Rect {
	if intrinsicWidth <= 0 || intrinsicHeight <= 0 || width <= 0 || height <= 0 {
		return Rect{X: x, Y: y, Width: width, Height: height}
	}
	imageWidth, imageHeight := width, height
	switch fit {
	case stylemodel.ObjectFitContain, stylemodel.ObjectFitCover, stylemodel.ObjectFitScaleDown:
		scale := min(width/intrinsicWidth, height/intrinsicHeight)
		if fit == stylemodel.ObjectFitCover {
			scale = max(width/intrinsicWidth, height/intrinsicHeight)
		} else if fit == stylemodel.ObjectFitScaleDown {
			scale = min(scale, float32(1))
		}
		imageWidth, imageHeight = intrinsicWidth*scale, intrinsicHeight*scale
	case stylemodel.ObjectFitNone:
		imageWidth, imageHeight = intrinsicWidth, intrinsicHeight
	}
	return Rect{
		X: x + position.X.Resolve(width-imageWidth), Y: y + position.Y.Resolve(height-imageHeight),
		Width: imageWidth, Height: imageHeight,
	}
}

func isSelectControl(node *dom.Node) bool {
	return node != nil && node.Type == dom.NodeElement && node.TagName == "select"
}

func isCheckableControl(node *dom.Node) bool {
	_, ok := forms.CheckableState(node)
	return ok
}

func isSubmitButtonControl(node *dom.Node) bool {
	return forms.IsSubmitButton(node)
}

func (e *engine) addSubmitButton(node *dom.Node, style blockStyle, x, width, containingHeight float32, heightDefinite bool) {
	e.y += style.margin.Top
	x += style.margin.Left
	label := strings.TrimSpace(node.TextContent())
	if node.TagName == "input" {
		label, _ = node.Attribute("value")
	}
	if label == "" {
		label = "Submit"
	}
	textWidth, textHeight, _ := measureStyledText(label, style)
	naturalWidth := textWidth + style.padding.Left + style.padding.Right + style.border.Left.Width + style.border.Right.Width
	naturalHeight := textHeight + style.padding.Top + style.padding.Bottom + style.border.Top.Width + style.border.Bottom.Width
	usedWidth, usedHeight := max(buttonWidth, naturalWidth), max(inputHeight, naturalHeight)
	if resolved, ok := resolveSize(style.width, width, true); ok {
		usedWidth = resolved
	}
	if resolved, ok := resolveSize(style.height, containingHeight, heightDefinite); ok {
		usedHeight = resolved
	}
	e.tree.Boxes = append(e.tree.Boxes, Box{
		Order: e.nextOrder(), StackingID: e.stackingID, NodeID: node.ID, Tag: node.TagName,
		Text: label, Button: true, Disabled: forms.Disabled(node),
		Appearance: style.appearance, AccentColor: resolvedAccentColor(style), Cursor: style.cursor,
		X: x, Y: e.y, Width: max(usedWidth, float32(1)), Height: max(usedHeight, float32(1)), Color: style.color,
		Clip: cloneRect(e.clip), Clips: cloneClipRegions(e.clips), Opacity: e.opacity * style.opacity,
		Transform: stylemodel.IdentityMatrix(), Hidden: style.hidden,
	})
	e.tree.Bounds[node.ID] = Rect{X: x, Y: e.y, Width: usedWidth, Height: usedHeight}
	e.y += usedHeight + style.margin.Bottom
}

func (e *engine) addCheckable(node *dom.Node, style blockStyle, x, width, containingHeight float32, heightDefinite bool) {
	e.y += style.margin.Top
	x += style.margin.Left
	usedWidth, usedHeight := checkableSize, checkableSize
	if resolved, ok := resolveSize(style.width, width, true); ok {
		usedWidth = resolved
	}
	if resolved, ok := resolveSize(style.height, containingHeight, heightDefinite); ok {
		usedHeight = resolved
	}
	state, _ := forms.CheckableState(node)
	e.tree.Boxes = append(e.tree.Boxes, Box{
		Order: e.nextOrder(), StackingID: e.stackingID, NodeID: node.ID, Tag: node.TagName,
		Checkable: true, Checked: state.Checked, InputType: state.Kind,
		Appearance: style.appearance, AccentColor: resolvedAccentColor(style), Cursor: style.cursor,
		Disabled: forms.Disabled(node),
		X:        x, Y: e.y, Width: max(usedWidth, float32(1)), Height: max(usedHeight, float32(1)), Color: style.color,
		Clip: cloneRect(e.clip), Clips: cloneClipRegions(e.clips), Opacity: e.opacity * style.opacity,
		Transform: stylemodel.IdentityMatrix(), Hidden: style.hidden,
	})
	e.tree.Bounds[node.ID] = Rect{X: x, Y: e.y, Width: usedWidth, Height: usedHeight}
	e.y += usedHeight + style.margin.Bottom
}

func (e *engine) addSelect(node *dom.Node, style blockStyle, x, width, containingHeight float32, heightDefinite bool) {
	e.y += style.margin.Top
	x += style.margin.Left
	availableWidth := width - style.margin.Left - style.margin.Right
	usedWidth := inputWidth
	if resolved, ok := resolveSize(style.width, availableWidth, true); ok {
		usedWidth = resolved
	}
	usedWidth = min(max(usedWidth, float32(1)), max(availableWidth, float32(1)))
	usedHeight := inputHeight
	if resolved, ok := resolveSize(style.height, containingHeight, heightDefinite); ok {
		usedHeight = resolved
	}
	options := forms.SelectOptions(node)
	selected := forms.SelectedIndex(node, options)
	label := ""
	if selected >= 0 {
		label = options[selected].Label
	}
	e.tree.Boxes = append(e.tree.Boxes, Box{
		Order: e.nextOrder(), StackingID: e.stackingID, NodeID: node.ID, Tag: node.TagName,
		Text: label, Select: true, Options: options, Selected: selected,
		Appearance: style.appearance, AccentColor: resolvedAccentColor(style), Cursor: style.cursor,
		Disabled: forms.Disabled(node),
		X:        x, Y: e.y, Width: usedWidth, Height: usedHeight, Color: style.color,
		Clip: cloneRect(e.clip), Clips: cloneClipRegions(e.clips), Opacity: e.opacity * style.opacity,
		Transform: stylemodel.IdentityMatrix(), Hidden: style.hidden,
	})
	e.tree.Bounds[node.ID] = Rect{X: x, Y: e.y, Width: usedWidth, Height: usedHeight}
	e.y += usedHeight + style.margin.Bottom
}

func (e *engine) addBlock(node *dom.Node, style blockStyle, x, width, containingHeight float32, heightDefinite bool, topMargin *float32) {
	geometryBoxStart, geometryDecorationStart := len(e.tree.Boxes), len(e.tree.Decorations)
	previousStackingID := e.stackingID
	effectRequested := len(style.filters) != 0 || len(style.backdropFilters) != 0 || style.mixBlendMode != stylemodel.BlendNormal
	if style.opacity < 1 || len(style.transform) != 0 || effectRequested || style.layoutPosition != stylemodel.PositionStatic && !style.zIndexAuto {
		e.stackingID = len(e.tree.StackingContexts)
		e.tree.StackingContexts = append(e.tree.StackingContexts, StackingContext{Parent: previousStackingID, NodeID: node.ID, ZIndex: style.zIndex, Order: e.order, Opacity: style.opacity, Offscreen: style.opacity < 1 || effectRequested})
	}
	previousOpacity := e.opacity
	e.opacity *= style.opacity
	if topMargin == nil {
		e.y += style.margin.Top
	} else {
		e.y += *topMargin
	}
	x += style.margin.Left
	availableWidth := width - style.margin.Left - style.margin.Right
	if availableWidth < 1 {
		availableWidth = 1
	}
	horizontalBorder := style.border.Left.Width + style.border.Right.Width
	verticalBorder := style.border.Top.Width + style.border.Bottom.Width
	sizingWidth := availableWidth
	if style.boxSizing == stylemodel.BoxSizingContentBox {
		sizingWidth -= style.padding.Left + style.padding.Right + horizontalBorder
	}
	if resolved, ok := resolveSize(style.width, width, true); ok {
		sizingWidth = resolved
	}
	sizingWidth = constrainSize(sizingWidth, style.minWidth, style.maxWidth, width, true)
	outerWidth := sizingWidth
	if style.boxSizing == stylemodel.BoxSizingContentBox {
		outerWidth += style.padding.Left + style.padding.Right + horizontalBorder
	}
	if outerWidth > availableWidth && style.width.Kind == stylemodel.SizeAuto {
		outerWidth = availableWidth
	}
	if outerWidth < 1 {
		outerWidth = 1
	}
	contentX := x + style.border.Left.Width + style.padding.Left
	contentWidth := outerWidth - style.padding.Left - style.padding.Right - horizontalBorder
	if contentWidth < 1 {
		contentWidth = 1
	}
	boxTop := e.y
	decorationIndex := -1
	if style.background != 0 || style.image.Kind != stylemodel.BackgroundImageNone || hasVisibleBorder(style.border) || len(style.boxShadows) != 0 || style.outline.Style != stylemodel.BorderNone || effectRequested {
		decorationIndex = len(e.tree.Decorations)
		e.tree.Decorations = append(e.tree.Decorations, Decoration{
			Order: e.nextOrder(), StackingID: e.stackingID, NodeID: node.ID,
			Rect:       Rect{X: x, Y: boxTop, Width: outerWidth},
			Background: style.background, Image: cloneBackgroundImage(style.image), Layers: cloneBackgroundLayers(style.backgroundLayers),
			Repeat: style.repeat, Position: style.position, Size: style.backgroundSize, Clip: cloneRect(e.clip),
			Clips:  cloneClipRegions(e.clips),
			Border: style.border, Opacity: e.opacity, BoxShadows: append([]stylemodel.Shadow(nil), style.boxShadows...),
			Outline: style.outline, OutlineOffset: style.outlineOffset,
			Filters: append([]stylemodel.Filter(nil), style.filters...), BackdropFilters: append([]stylemodel.Filter(nil), style.backdropFilters...), BlendMode: style.mixBlendMode, Cursor: style.cursor,
			Transform: stylemodel.IdentityMatrix(),
			Hidden:    style.hidden,
		})
	}
	e.y += style.border.Top.Width + style.padding.Top
	contentTop := e.y
	declaredHeight, declaredHeightDefinite := resolveSize(style.height, containingHeight, heightDefinite)
	childContainingHeight := declaredHeight
	if declaredHeightDefinite && style.boxSizing == stylemodel.BoxSizingBorderBox {
		childContainingHeight -= style.padding.Top + style.padding.Bottom + verticalBorder
		if childContainingHeight < 0 {
			childContainingHeight = 0
		}
	}
	previousClip := e.clip
	previousClips := e.clips
	previousPositionCB := e.positionCB
	if style.layoutPosition != stylemodel.PositionStatic {
		cbHeight := childContainingHeight
		if !declaredHeightDefinite {
			cbHeight = containingHeight
		}
		e.positionCB = &Rect{X: x + style.border.Left.Width, Y: boxTop + style.border.Top.Width, Width: outerWidth - horizontalBorder, Height: max(cbHeight, float32(0))}
	}
	if (style.overflowX != stylemodel.OverflowVisible || style.overflowY != stylemodel.OverflowVisible) && declaredHeightDefinite {
		clipHeight := declaredHeight
		if style.boxSizing == stylemodel.BoxSizingContentBox {
			clipHeight += style.padding.Top + style.padding.Bottom
		}
		e.clip = intersectClip(previousClip, Rect{
			X: x + style.border.Left.Width, Y: boxTop + style.border.Top.Width,
			Width: outerWidth - horizontalBorder, Height: clipHeight,
		})
		clipRect := *e.clip
		e.clips = append(cloneClipRegions(previousClips), ClipRegion{Rect: clipRect, Radius: resolveBorderRadii(style.radius, clipRect.Width, clipRect.Height)})
	}

	var positionedChildren []*dom.Node
	if style.display == stylemodel.DisplayFlex {
		e.addFlexChildren(node, style, contentX, contentWidth, childContainingHeight, declaredHeightDefinite)
	} else if style.display == stylemodel.DisplayGrid {
		e.addGridChildren(node, style, contentX, contentWidth, childContainingHeight, declaredHeightDefinite)
	} else {
		inlineRuns := e.listMarkerRuns(node, style)
		inlineRuns = append(inlineRuns, e.generatedRuns(node, true, style)...)
		previousBlock := false
		previousBottomMargin := float32(0)
		flushInline := func() {
			if len(inlineRuns) != 0 {
				e.addInlineRuns(node.ID, node.TagName, inlineRuns, style, contentX, contentWidth)
				previousBlock = false
			}
			inlineRuns = inlineRuns[:0]
		}

		for _, child := range e.flowChildren(node) {
			if child.Type == dom.NodeElement {
				childStyle := e.styleFor(child)
				if childStyle.display == stylemodel.DisplayNone {
					continue
				}
				if childStyle.clear != stylemodel.ClearNone {
					flushInline()
					e.clearFloats(childStyle.clear)
				}
				if childStyle.float != stylemodel.FloatNone {
					flushInline()
					e.addFloat(child, childStyle, contentX, contentWidth, childContainingHeight, declaredHeightDefinite)
					previousBlock = false
					continue
				}
				if childStyle.display == stylemodel.DisplayTable {
					flushInline()
					e.addTable(child, childStyle, contentX, contentWidth, childContainingHeight, declaredHeightDefinite)
					previousBlock = true
					previousBottomMargin = childStyle.margin.Bottom
					continue
				}
				if childStyle.layoutPosition == stylemodel.PositionAbsolute || childStyle.layoutPosition == stylemodel.PositionFixed {
					flushInline()
					positionedChildren = append(positionedChildren, child)
					continue
				}
				if isImageElement(child, e.images) {
					flushInline()
					e.addImage(child, childStyle, contentX, contentWidth, childContainingHeight, declaredHeightDefinite)
					previousBlock = true
					previousBottomMargin = childStyle.margin.Bottom
					continue
				}
				if isEditableTextControl(child) {
					flushInline()
					e.addInput(child, childStyle, contentX, contentWidth, childContainingHeight, declaredHeightDefinite)
					previousBlock = true
					previousBottomMargin = childStyle.margin.Bottom
					continue
				}
				if isSelectControl(child) {
					flushInline()
					e.addSelect(child, childStyle, contentX, contentWidth, childContainingHeight, declaredHeightDefinite)
					previousBlock = true
					previousBottomMargin = childStyle.margin.Bottom
					continue
				}
				if isCheckableControl(child) {
					flushInline()
					e.addCheckable(child, childStyle, contentX, contentWidth, childContainingHeight, declaredHeightDefinite)
					previousBlock = true
					previousBottomMargin = childStyle.margin.Bottom
					continue
				}
				if isSubmitButtonControl(child) {
					flushInline()
					e.addSubmitButton(child, childStyle, contentX, contentWidth, childContainingHeight, declaredHeightDefinite)
					previousBlock = true
					previousBottomMargin = childStyle.margin.Bottom
					continue
				}
				if isBlockLevelDisplay(childStyle.display) {
					flushInline()
					if previousBlock {
						e.y -= previousBottomMargin
						collapsed := collapseMargins(previousBottomMargin, childStyle.margin.Top)
						e.addBlock(child, childStyle, contentX, contentWidth, childContainingHeight, declaredHeightDefinite, &collapsed)
					} else {
						e.addBlock(child, childStyle, contentX, contentWidth, childContainingHeight, declaredHeightDefinite, nil)
					}
					previousBlock = true
					previousBottomMargin = childStyle.margin.Bottom
					continue
				}
			}
			inlineRuns = append(inlineRuns, e.collectInlineRuns(child, node)...)
		}
		inlineRuns = append(inlineRuns, e.generatedRuns(node, false, style)...)
		flushInline()
	}
	e.clip = previousClip
	e.clips = previousClips

	contentHeight := e.y - contentTop
	sizingHeight := contentHeight
	if declaredHeightDefinite {
		sizingHeight = declaredHeight
	}
	sizingHeight = constrainSize(sizingHeight, style.minHeight, style.maxHeight, containingHeight, heightDefinite)
	outerHeight := sizingHeight
	if style.boxSizing == stylemodel.BoxSizingContentBox {
		outerHeight += style.padding.Top + style.padding.Bottom + verticalBorder
	}
	if outerHeight < 0 {
		outerHeight = 0
	}
	e.tree.Bounds[node.ID] = Rect{X: x, Y: boxTop, Width: outerWidth, Height: outerHeight}
	if decorationIndex >= 0 {
		e.tree.Decorations[decorationIndex].Height = outerHeight
		e.tree.Decorations[decorationIndex].Radius = resolveBorderRadii(style.radius, outerWidth, outerHeight)
		if effectRequested && !stylemodel.FilterSurfaceAllowed(outerWidth, outerHeight) {
			e.tree.Decorations[decorationIndex].Filters = nil
			e.tree.Decorations[decorationIndex].BackdropFilters = nil
			e.tree.Decorations[decorationIndex].BlendMode = stylemodel.BlendNormal
			if e.stackingID > 0 && e.stackingID < len(e.tree.StackingContexts) && style.opacity >= 1 {
				e.tree.StackingContexts[e.stackingID].Offscreen = false
			}
		}
	}
	if e.positionCB != nil && style.layoutPosition != stylemodel.PositionStatic {
		e.positionCB.Height = max(outerHeight-verticalBorder, float32(0))
	}
	for _, child := range positionedChildren {
		e.renderPositionedChild(child, e.styleFor(child))
	}
	e.y = boxTop + outerHeight + style.margin.Bottom
	if style.layoutPosition == stylemodel.PositionRelative || style.layoutPosition == stylemodel.PositionSticky {
		dx, dy := float32(0), float32(0)
		if style.layoutPosition == stylemodel.PositionRelative {
			dx, dy = relativeOffset(style.inset, outerWidth, outerHeight)
		} else {
			if top, ok := resolveSize(style.inset.Top, outerHeight, true); ok {
				dy = max(e.scrollY+top-boxTop, float32(0))
			}
		}
		translateFlexGeometry(e.tree, geometryBoxStart, geometryDecorationStart, dx, dy, nil)
	}
	if len(style.transform) != 0 {
		originX := x + style.transformOrigin.X.Resolve(outerWidth)
		originY := boxTop + style.transformOrigin.Y.Resolve(outerHeight)
		local := stylemodel.Matrix{A: 1, D: 1, E: originX, F: originY}.Multiply(stylemodel.ResolveTransform(style.transform, outerWidth, outerHeight))
		local = local.Multiply(stylemodel.Matrix{A: 1, D: 1, E: -originX, F: -originY})
		for index := geometryBoxStart; index < len(e.tree.Boxes); index++ {
			e.tree.Boxes[index].Transform = local.Multiply(normalizeMatrix(e.tree.Boxes[index].Transform))
		}
		for index := geometryDecorationStart; index < len(e.tree.Decorations); index++ {
			e.tree.Decorations[index].Transform = local.Multiply(normalizeMatrix(e.tree.Decorations[index].Transform))
		}
	}
	e.positionCB = previousPositionCB
	e.stackingID = previousStackingID
	e.opacity = previousOpacity
}

func recordNodeParents(tree *Tree, document *dom.Document) {
	if tree == nil || document == nil || document.Root == nil {
		return
	}
	var walk func(*dom.Node, dom.NodeID)
	walk = func(node *dom.Node, parent dom.NodeID) {
		if node == nil {
			return
		}
		if node.ID != 0 {
			tree.Parents[node.ID] = parent
			parent = node.ID
		}
		for _, child := range node.Children {
			walk(child, parent)
		}
	}
	walk(document.Root, 0)
}

func normalizeMatrix(matrix stylemodel.Matrix) stylemodel.Matrix {
	if matrix == (stylemodel.Matrix{}) {
		return stylemodel.IdentityMatrix()
	}
	return matrix
}

func relativeOffset(inset stylemodel.Insets, width, height float32) (float32, float32) {
	dx, dy := float32(0), float32(0)
	if left, ok := resolveSize(inset.Left, width, true); ok {
		dx = left
	} else if right, ok := resolveSize(inset.Right, width, true); ok {
		dx = -right
	}
	if top, ok := resolveSize(inset.Top, height, true); ok {
		dy = top
	} else if bottom, ok := resolveSize(inset.Bottom, height, true); ok {
		dy = -bottom
	}
	return dx, dy
}

func (e *engine) renderPositionedChild(node *dom.Node, style blockStyle) {
	containingBlock := e.positionCB
	if style.layoutPosition == stylemodel.PositionFixed || containingBlock == nil {
		containingBlock = &Rect{X: e.scrollX, Y: e.scrollY, Width: e.viewportWidth, Height: e.viewportHeight}
	}
	left, hasLeft := resolveSize(style.inset.Left, containingBlock.Width, true)
	right, hasRight := resolveSize(style.inset.Right, containingBlock.Width, true)
	top, hasTop := resolveSize(style.inset.Top, containingBlock.Height, containingBlock.Height > 0)
	bottom, hasBottom := resolveSize(style.inset.Bottom, containingBlock.Height, containingBlock.Height > 0)
	usedWidth, widthDefinite := gridItemOuterSize(style.width, style, containingBlock.Width, true)
	usedHeight, heightDefinite := gridItemOuterSize(style.height, style, containingBlock.Height, false)
	if !widthDefinite && hasLeft && hasRight {
		usedWidth = max(containingBlock.Width-left-right, float32(0))
	}
	if !heightDefinite && hasTop && hasBottom {
		usedHeight = max(containingBlock.Height-top-bottom, float32(0))
	}
	if !widthDefinite && !(hasLeft && hasRight) || !heightDefinite && !(hasTop && hasBottom) {
		intrinsicWidth, intrinsicHeight, _ := e.flexIntrinsicSizes(node, style, flexAxis{horizontal: true}, containingBlock.Width, containingBlock.Width, containingBlock.Height, containingBlock.Height > 0)
		if !widthDefinite && !(hasLeft && hasRight) {
			usedWidth = intrinsicWidth
		}
		if !heightDefinite && !(hasTop && hasBottom) {
			usedHeight = intrinsicHeight
		}
	}
	childX, childY := containingBlock.X, containingBlock.Y
	if hasLeft {
		childX += left
	} else if hasRight {
		childX += containingBlock.Width - right - usedWidth
	}
	if hasTop {
		childY += top
	} else if hasBottom {
		childY += containingBlock.Height - bottom - usedHeight
	}
	if style.display == stylemodel.DisplayInline || style.display == stylemodel.DisplayInlineBlock || style.display == stylemodel.DisplayInlineFlex || style.display == stylemodel.DisplayInlineGrid {
		style.display = stylemodel.DisplayBlock
	}
	e.renderGridItem(node, style, childX, childY, usedWidth, usedHeight)
}

func collapseMargins(first, second float32) float32 {
	positive := max(first, float32(0), second)
	negative := min(first, float32(0), second)
	return positive + negative
}

func resolveSize(value stylemodel.SizeValue, basis float32, basisDefinite bool) (float32, bool) {
	if value.Kind != stylemodel.SizeLength || value.Value.Percentage != 0 && !basisDefinite {
		return 0, false
	}
	resolved := value.Value.Resolve(basis)
	if resolved < 0 {
		resolved = 0
	}
	return resolved, true
}

func constrainSize(value float32, minimum, maximum stylemodel.SizeValue, basis float32, basisDefinite bool) float32 {
	if resolved, ok := resolveSize(minimum, basis, basisDefinite); ok && value < resolved {
		value = resolved
	}
	if resolved, ok := resolveSize(maximum, basis, basisDefinite); ok && value > resolved {
		value = resolved
	}
	return value
}

func (e *engine) collectInlineRuns(node, owner *dom.Node) []inlineRun {
	return e.collectInlineRunsWithOpacity(node, owner, e.opacity)
}

func (e *engine) collectInlineRunsWithOpacity(node, owner *dom.Node, opacity float32) []inlineRun {
	if node == nil {
		return nil
	}
	if node.Type == dom.NodeText {
		style := e.styleFor(owner)
		return []inlineRun{{nodeID: owner.ID, tag: owner.TagName, text: node.Text, style: style, opacity: opacity}}
	}
	if node.Type != dom.NodeElement {
		return nil
	}
	style := e.styleFor(node)
	opacity *= style.opacity
	if style.display == stylemodel.DisplayNone {
		return nil
	}
	if style.display == stylemodel.DisplayContents {
		var result []inlineRun
		for _, child := range node.Children {
			result = append(result, e.collectInlineRunsWithOpacity(child, owner, opacity)...)
		}
		return result
	}
	if style.display == stylemodel.DisplayInlineBlock {
		return []inlineRun{{nodeID: node.ID, tag: node.TagName, text: e.inlineText(node), style: style, atomic: true, opacity: opacity}}
	}
	if style.display == stylemodel.DisplayInlineFlex {
		return []inlineRun{{nodeID: node.ID, node: node, tag: node.TagName, style: style, atomic: true, flex: true, opacity: opacity}}
	}
	if style.display == stylemodel.DisplayInlineGrid {
		return []inlineRun{{nodeID: node.ID, node: node, tag: node.TagName, style: style, atomic: true, grid: true, opacity: opacity}}
	}
	if node.TagName == "br" {
		return []inlineRun{{nodeID: node.ID, tag: node.TagName, text: "\n", style: style, opacity: opacity}}
	}

	result := e.generatedRunsWithOpacity(node, true, style, opacity)
	for _, child := range node.Children {
		result = append(result, e.collectInlineRunsWithOpacity(child, node, opacity)...)
	}
	result = append(result, e.generatedRunsWithOpacity(node, false, style, opacity)...)
	return result
}

func (e *engine) generatedRuns(node *dom.Node, before bool, style blockStyle) []inlineRun {
	return e.generatedRunsWithOpacity(node, before, style, e.opacity)
}

func (e *engine) listMarkerRuns(node *dom.Node, style blockStyle) []inlineRun {
	if node == nil || node.TagName != "li" || style.listStyleType == stylemodel.ListStyleNone {
		return nil
	}
	marker := "•"
	switch style.listStyleType {
	case stylemodel.ListStyleCircle:
		marker = "◦"
	case stylemodel.ListStyleSquare:
		marker = "▪"
	case stylemodel.ListStyleDecimal:
		index := 1
		if node.Parent != nil {
			index = 0
			for _, sibling := range node.Parent.Children {
				if sibling.Type == dom.NodeElement && sibling.TagName == "li" {
					index++
				}
				if sibling == node {
					break
				}
			}
			if index == 0 {
				index = 1
			}
		}
		marker = strconv.Itoa(index) + "."
	}
	if style.listStyleImage != "" {
		marker = "◆"
	}
	return []inlineRun{{nodeID: node.ID, tag: "::marker", text: marker + " ", style: style, opacity: e.opacity}}
}

func resolvedAccentColor(style blockStyle) uint32 {
	if !style.accentColorAuto {
		return style.accentColor
	}
	return 0x0969daff
}

func (e *engine) generatedRunsWithOpacity(node *dom.Node, before bool, style blockStyle, opacity float32) []inlineRun {
	computed, ok := e.computed.For(node)
	if !ok {
		return nil
	}
	text, tag := computed.AfterContent, "::after"
	if before {
		text, tag = computed.BeforeContent, "::before"
	}
	if text == "" {
		return nil
	}
	return []inlineRun{{nodeID: node.ID, tag: tag, text: text, style: style, opacity: opacity}}
}

func (e *engine) inlineText(node *dom.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == dom.NodeText {
		return node.Text
	}
	if node.Type == dom.NodeElement && e.styleFor(node).display == stylemodel.DisplayNone {
		return ""
	}
	var text strings.Builder
	for _, child := range node.Children {
		text.WriteString(e.inlineText(child))
	}
	return text.String()
}

func (e *engine) addText(nodeID dom.NodeID, tag, text string, style blockStyle, x, width float32) {
	e.addInlineRuns(nodeID, tag, []inlineRun{{nodeID: nodeID, tag: tag, text: text, style: style, opacity: e.opacity * style.opacity}}, style, x, width)
}

func (e *engine) addInlineRuns(nodeID dom.NodeID, tag string, runs []inlineRun, container blockStyle, x, width float32) {
	var lineRuns []TextRun
	var flexPlacements []inlineRun
	var lineText strings.Builder
	var usedWidth, lineHeight, lineAscent float32
	var pendingSpace *inlineRun
	lineX, lineWidth := e.floatLineArea(x, width, e.y, container.lineHeight)
	firstLine := true
	if indent := container.textIndent.Resolve(width); indent != 0 {
		lineX += indent
		lineWidth = max(width-indent, float32(1))
	}

	flushLine := func(final bool) {
		if len(lineRuns) == 0 {
			return
		}
		if final && container.textOverflow == stylemodel.TextOverflowEllipsis &&
			container.whiteSpace == stylemodel.WhiteSpaceNowrap && container.overflowX != stylemodel.OverflowVisible && usedWidth > lineWidth {
			lineRuns, usedWidth = truncateTextRuns(lineRuns, lineWidth, e.fonts)
			lineText.Reset()
			for _, run := range lineRuns {
				if run.Tag != "::marker" {
					lineText.WriteString(run.Text)
				}
			}
		}
		if lineHeight == 0 {
			lineHeight = container.fontSize * 1.4
		}
		for index := range lineRuns {
			lineRuns[index].Baseline = e.y + lineAscent - lineRuns[index].VerticalOffset
		}
		alignmentOffset := float32(0)
		free := max(lineWidth-usedWidth, float32(0))
		switch container.textAlign {
		case stylemodel.TextAlignEnd, stylemodel.TextAlignRight:
			alignmentOffset = free
		case stylemodel.TextAlignCenter:
			alignmentOffset = free / 2
		case stylemodel.TextAlignJustify:
			if !final {
				spaces := 0
				for _, run := range lineRuns {
					spaces += strings.Count(run.Text, " ")
				}
				if spaces > 0 {
					extra := free / float32(spaces)
					for index := range lineRuns {
						count := strings.Count(lineRuns[index].Text, " ")
						lineRuns[index].WordSpacing += extra
						lineRuns[index].Width += float32(count) * extra
					}
					usedWidth = lineWidth
				}
			}
		}
		e.tree.Boxes = append(e.tree.Boxes, Box{
			Order: e.nextOrder(), StackingID: e.stackingID,
			NodeID: nodeID, Tag: tag, Text: lineText.String(),
			X: lineX + alignmentOffset, Y: e.y, Width: max(lineWidth-alignmentOffset, usedWidth), Height: lineHeight,
			FontSize: container.fontSize, Bold: container.bold, Color: container.color,
			FontFamilies: append([]string(nil), container.fontFamilies...), FontStyle: container.fontStyle, FontStretch: container.fontStretch,
			LetterSpacing: container.letterSpacing, WordSpacing: container.wordSpacing,
			Cursor:  container.cursor,
			Opacity: e.opacity, Decoration: container.decoration, DecorationColor: container.decorationColor,
			TextShadows: append([]stylemodel.Shadow(nil), container.textShadows...),
			Transform:   stylemodel.IdentityMatrix(),
			Hidden:      container.hidden,
			Runs:        append([]TextRun(nil), lineRuns...),
			Baseline:    e.y + lineAscent, Clip: cloneRect(e.clip), Clips: cloneClipRegions(e.clips),
		})
		for _, placement := range flexPlacements {
			placementX, placementY := lineX+alignmentOffset+placement.widthOffset, e.y+lineAscent-placement.baseline
			if placement.grid {
				e.renderInlineGrid(placement, placementX, placementY)
			} else {
				item := &flexLayoutItem{node: placement.node, style: placement.style, crossSize: placement.height}
				item.algorithm = &flexItem{target: placement.width}
				e.renderFlexItem(item, flexAxis{horizontal: true}, placementX, placementY, placement.width, placement.height)
			}
		}
		e.y += lineHeight
		lineRuns = lineRuns[:0]
		lineText.Reset()
		usedWidth, lineHeight, lineAscent, pendingSpace = 0, 0, 0, nil
		flexPlacements = flexPlacements[:0]
		if firstLine {
			firstLine = false
		}
		lineX, lineWidth = e.floatLineArea(x, width, e.y, container.lineHeight)
	}

	appendPiece := func(run inlineRun, text string, pieceWidth float32) {
		textRun := TextRun{
			NodeID: run.nodeID, Tag: run.tag, Text: text, Width: pieceWidth,
			FontSize: run.style.fontSize, Bold: run.style.bold,
			FontFamilies: append([]string(nil), run.style.fontFamilies...), FontStyle: run.style.fontStyle, FontStretch: run.style.fontStretch,
			LetterSpacing: run.style.letterSpacing, WordSpacing: run.style.wordSpacing, VerticalOffset: verticalAlignOffset(run.style),
			Color: run.style.color, Background: run.style.background,
			Decoration: run.style.decoration, DecorationColor: run.style.decorationColor, Opacity: run.opacity,
			TextShadows: append([]stylemodel.Shadow(nil), run.style.textShadows...),
		}
		runHeight, runAscent := usedLineMetrics(run)
		textRun.Baseline = e.y + runAscent - textRun.VerticalOffset
		if len(lineRuns) > 0 && sameTextStyle(lineRuns[len(lineRuns)-1], textRun) {
			lineRuns[len(lineRuns)-1].Text += text
			lineRuns[len(lineRuns)-1].Width += pieceWidth
		} else {
			lineRuns = append(lineRuns, textRun)
		}
		if run.tag != "::marker" {
			lineText.WriteString(text)
		}
		usedWidth += pieceWidth
		height := runHeight + absFloat32(textRun.VerticalOffset)
		if height > lineHeight {
			lineHeight = height
		}
		if adjusted := runAscent + max(textRun.VerticalOffset, float32(0)); adjusted > lineAscent {
			lineAscent = adjusted
		}
	}

	for _, token := range tokenizeInlineRuns(transformInlineRuns(runs)) {
		if token.atomic {
			if token.flex {
				token.width, token.height, token.baseline = e.resolveInlineFlexSize(token.node, token.style, width)
			} else if token.grid {
				token.width, token.height, token.baseline = e.resolveInlineGridSize(token.node, token.style, width)
			} else {
				token.width, token.height = resolveAtomicSize(token, width)
			}
			if usedWidth > 0 && usedWidth+token.width > lineWidth && wrapsWhitespace(token.style.whiteSpace) {
				flushLine(false)
			}
			if token.flex || token.grid {
				token.widthOffset = usedWidth
				flexPlacements = append(flexPlacements, token)
				appendPiece(token, "", token.width)
			} else {
				appendPiece(token, token.text, token.width)
			}
			continue
		}
		if token.text == "\n" {
			flushLine(false)
			continue
		}
		if token.text == " " {
			if preservesSpaces(token.style.whiteSpace) {
				spaceWidth, _, _ := measureStyledText(" ", token.style)
				if usedWidth > 0 && usedWidth+spaceWidth > lineWidth && wrapsWhitespace(token.style.whiteSpace) {
					flushLine(false)
				}
				appendPiece(token, " ", spaceWidth)
				continue
			}
			if len(lineRuns) > 0 {
				copy := token
				pendingSpace = &copy
			}
			continue
		}

		spaceWidth := float32(0)
		if pendingSpace != nil {
			spaceWidth, _, _ = measureStyledText(" ", pendingSpace.style)
		}
		wordWidth, _, _ := measureStyledText(token.text, token.style)
		if usedWidth > 0 && usedWidth+spaceWidth+wordWidth > lineWidth && wrapsWhitespace(token.style.whiteSpace) {
			flushLine(false)
			spaceWidth = 0
		}
		if pendingSpace != nil && usedWidth > 0 {
			appendPiece(*pendingSpace, " ", spaceWidth)
		}
		pendingSpace = nil

		breakInside := token.style.wordBreak == stylemodel.WordBreakBreakAll || token.style.overflowWrap == stylemodel.OverflowWrapAnywhere ||
			token.style.overflowWrap == stylemodel.OverflowWrapBreakWord && wordWidth > lineWidth
		if !breakInside || !wrapsWhitespace(token.style.whiteSpace) {
			appendPiece(token, token.text, wordWidth)
			continue
		}
		remaining := []rune(token.text)
		for len(remaining) > 0 {
			available := lineWidth - usedWidth
			mWidth, _, _ := measureStyledText("m", token.style)
			characters := int(available / max(mWidth, float32(1)))
			if characters < 1 {
				if wrapsWhitespace(token.style.whiteSpace) && usedWidth > 0 {
					flushLine(false)
					continue
				}
				characters = 1
				if !wrapsWhitespace(token.style.whiteSpace) {
					characters = len(remaining)
				}
			}
			if characters > len(remaining) {
				characters = len(remaining)
			}
			piece := string(remaining[:characters])
			pieceWidth, _, _ := measureStyledText(piece, token.style)
			appendPiece(token, piece, pieceWidth)
			remaining = remaining[characters:]
			if len(remaining) > 0 && wrapsWhitespace(token.style.whiteSpace) {
				flushLine(false)
			}
		}
	}
	flushLine(true)
}

func isBlockLevelDisplay(display stylemodel.Display) bool {
	return display == stylemodel.DisplayBlock || display == stylemodel.DisplayFlex || display == stylemodel.DisplayGrid
}

func tokenizeInlineRuns(runs []inlineRun) []inlineRun {
	var tokens []inlineRun
	for _, run := range runs {
		if run.atomic {
			tokens = append(tokens, run)
			continue
		}
		var word strings.Builder
		flushWord := func() {
			if word.Len() == 0 {
				return
			}
			token := run
			token.text = word.String()
			tokens = append(tokens, token)
			word.Reset()
		}
		for _, character := range run.text {
			if character == '\n' {
				flushWord()
				if preservesNewlines(run.style.whiteSpace) {
					token := run
					token.text = "\n"
					tokens = append(tokens, token)
				} else if len(tokens) == 0 || tokens[len(tokens)-1].text != " " {
					token := run
					token.text = " "
					tokens = append(tokens, token)
				}
			} else if unicode.IsSpace(character) {
				flushWord()
				if preservesSpaces(run.style.whiteSpace) || len(tokens) == 0 || tokens[len(tokens)-1].text != " " {
					token := run
					token.text = " "
					tokens = append(tokens, token)
				}
			} else {
				word.WriteRune(character)
			}
		}
		flushWord()
	}
	return tokens
}

func resolveAtomicSize(run inlineRun, containingWidth float32) (float32, float32) {
	horizontal := run.style.padding.Left + run.style.padding.Right + run.style.border.Left.Width + run.style.border.Right.Width
	vertical := run.style.padding.Top + run.style.padding.Bottom + run.style.border.Top.Width + run.style.border.Bottom.Width
	width, _, _ := measureStyledText(normalizeWhitespace(run.text), run.style)
	if resolved, ok := resolveSize(run.style.width, containingWidth, true); ok {
		width = resolved
	}
	width = constrainSize(width, run.style.minWidth, run.style.maxWidth, containingWidth, true)
	height := run.style.fontSize * 1.4
	if resolved, ok := resolveSize(run.style.height, 0, false); ok {
		height = resolved
	}
	height = constrainSize(height, run.style.minHeight, run.style.maxHeight, 0, false)
	if run.style.boxSizing == stylemodel.BoxSizingContentBox {
		width += horizontal
		height += vertical
	}
	return max(width, float32(0)), max(height, float32(0))
}

func sameTextStyle(left, right TextRun) bool {
	return left.NodeID == right.NodeID && left.Tag == right.Tag && left.FontSize == right.FontSize &&
		left.Bold == right.Bold && left.Color == right.Color && left.Background == right.Background &&
		left.FontStyle == right.FontStyle && left.FontStretch == right.FontStretch && strings.Join(left.FontFamilies, "\x00") == strings.Join(right.FontFamilies, "\x00") &&
		left.LetterSpacing == right.LetterSpacing && left.WordSpacing == right.WordSpacing && left.VerticalOffset == right.VerticalOffset &&
		left.Decoration == right.Decoration && left.DecorationColor == right.DecorationColor && left.Opacity == right.Opacity
}

func transformInlineRuns(runs []inlineRun) []inlineRun {
	result := append([]inlineRun(nil), runs...)
	for index := range result {
		switch result[index].style.textTransform {
		case stylemodel.TextTransformUppercase:
			result[index].text = strings.ToUpper(result[index].text)
		case stylemodel.TextTransformLowercase:
			result[index].text = strings.ToLower(result[index].text)
		case stylemodel.TextTransformCapitalize:
			start := true
			result[index].text = strings.Map(func(character rune) rune {
				if unicode.IsSpace(character) || unicode.IsPunct(character) {
					start = true
					return character
				}
				if start {
					start = false
					return unicode.ToUpper(character)
				}
				return character
			}, result[index].text)
		}
	}
	return result
}

func verticalAlignOffset(style blockStyle) float32 {
	switch style.verticalAlign.Kind {
	case stylemodel.VerticalAlignLength:
		return style.verticalAlign.Value
	case stylemodel.VerticalAlignSuper:
		return style.fontSize * .35
	case stylemodel.VerticalAlignSub:
		return -style.fontSize * .2
	case stylemodel.VerticalAlignMiddle:
		return style.fontSize * .15
	case stylemodel.VerticalAlignTextTop, stylemodel.VerticalAlignTop:
		return style.fontSize * .2
	case stylemodel.VerticalAlignTextBottom, stylemodel.VerticalAlignBottom:
		return -style.fontSize * .2
	default:
		return 0
	}
}

func truncateTextRuns(runs []TextRun, width float32, fonts *FontSet) ([]TextRun, float32) {
	if len(runs) == 0 {
		return runs, 0
	}
	last := runs[len(runs)-1]
	ellipsisWidth := measureTextRun("…", last, fonts)
	limit := max(width-ellipsisWidth, float32(0))
	result := make([]TextRun, 0, len(runs))
	used := float32(0)
	for _, run := range runs {
		kept := run
		kept.Text, kept.Width = "", 0
		for _, character := range run.Text {
			piece := string(character)
			pieceWidth := measureTextRun(piece, run, fonts)
			if kept.Text != "" {
				pieceWidth += run.LetterSpacing
			}
			if used+pieceWidth > limit {
				break
			}
			kept.Text += piece
			kept.Width += pieceWidth
			used += pieceWidth
		}
		if kept.Text != "" {
			result = append(result, kept)
		}
		if kept.Text != run.Text {
			break
		}
	}
	marker := last
	marker.Text, marker.Width = "…", ellipsisWidth
	if len(result) > 0 && sameTextStyle(result[len(result)-1], marker) {
		result[len(result)-1].Text += marker.Text
		result[len(result)-1].Width += marker.Width
	} else {
		result = append(result, marker)
	}
	return result, min(used+ellipsisWidth, width)
}

func measureTextRun(text string, run TextRun, fonts *FontSet) float32 {
	style := blockStyle{fonts: fonts, fontSize: run.FontSize, bold: run.Bold, fontFamilies: run.FontFamilies, fontStyle: run.FontStyle, fontStretch: run.FontStretch, letterSpacing: run.LetterSpacing, wordSpacing: run.WordSpacing}
	width, _, _ := measureStyledText(text, style)
	return width
}

func absFloat32(value float32) float32 {
	if value < 0 {
		return -value
	}
	return value
}

func preservesSpaces(value stylemodel.WhiteSpace) bool {
	return value == stylemodel.WhiteSpacePre || value == stylemodel.WhiteSpacePreWrap
}

func preservesNewlines(value stylemodel.WhiteSpace) bool {
	return value == stylemodel.WhiteSpacePre || value == stylemodel.WhiteSpacePreWrap || value == stylemodel.WhiteSpacePreLine
}

func wrapsWhitespace(value stylemodel.WhiteSpace) bool {
	return value != stylemodel.WhiteSpaceNowrap && value != stylemodel.WhiteSpacePre
}

func cloneRect(source *Rect) *Rect {
	if source == nil {
		return nil
	}
	copy := *source
	return &copy
}

func cloneClipRegions(source []ClipRegion) []ClipRegion {
	return append([]ClipRegion(nil), source...)
}

func cloneBackgroundImage(source stylemodel.BackgroundImage) stylemodel.BackgroundImage {
	result := source
	result.GradientStops = append([]stylemodel.GradientStop(nil), source.GradientStops...)
	return result
}

func cloneBackgroundLayers(source []stylemodel.BackgroundLayer) []stylemodel.BackgroundLayer {
	result := append([]stylemodel.BackgroundLayer(nil), source...)
	for index := range result {
		result[index].Image = cloneBackgroundImage(result[index].Image)
	}
	return result
}

func intersectClip(parent *Rect, child Rect) *Rect {
	if parent == nil {
		return &child
	}
	left, top := max(parent.X, child.X), max(parent.Y, child.Y)
	right := min(parent.X+parent.Width, child.X+child.Width)
	bottom := min(parent.Y+parent.Height, child.Y+child.Height)
	return &Rect{X: left, Y: top, Width: max(right-left, float32(0)), Height: max(bottom-top, float32(0))}
}

func hasVisibleBorder(border stylemodel.Borders) bool {
	return border.Top.Width > 0 || border.Right.Width > 0 || border.Bottom.Width > 0 || border.Left.Width > 0
}

func resolveBorderRadii(source stylemodel.BorderRadii, width, height float32) BorderRadii {
	resolve := func(value stylemodel.RadiusValue) CornerRadius {
		return CornerRadius{X: value.X.Resolve(width), Y: value.Y.Resolve(height)}
	}
	result := BorderRadii{
		TopLeft: resolve(source.TopLeft), TopRight: resolve(source.TopRight),
		BottomRight: resolve(source.BottomRight), BottomLeft: resolve(source.BottomLeft),
	}
	scale := float32(1)
	ratio := func(limit, sum float32) float32 {
		if sum > limit && sum > 0 {
			return limit / sum
		}
		return 1
	}
	for _, ratio := range []float32{
		ratio(width, result.TopLeft.X+result.TopRight.X),
		ratio(width, result.BottomLeft.X+result.BottomRight.X),
		ratio(height, result.TopLeft.Y+result.BottomLeft.Y),
		ratio(height, result.TopRight.Y+result.BottomRight.Y),
	} {
		scale = min(scale, ratio)
	}
	for _, radius := range []*CornerRadius{&result.TopLeft, &result.TopRight, &result.BottomRight, &result.BottomLeft} {
		radius.X *= scale
		radius.Y *= scale
	}
	return result
}

func (e *engine) styleFor(node *dom.Node) blockStyle {
	style := uaStyle(node.TagName)
	if computed, ok := e.computed.For(node); ok {
		style = applyComputed(style, computed)
	}
	style.fonts = e.fonts
	if style.cursor == stylemodel.CursorAuto {
		switch node.TagName {
		case "a", "button", "select":
			style.cursor = stylemodel.CursorPointer
		case "input", "textarea":
			style.cursor = stylemodel.CursorText
		default:
			style.cursor = stylemodel.CursorDefault
		}
	}
	return style
}

func uaStyle(tag string) blockStyle {
	style := defaultStyle()
	switch tag {
	case "html", "body", "div", "main", "section", "article", "header", "footer", "nav", "form", "ul", "ol":
		style.display = stylemodel.DisplayBlock
	case "h1":
		style.display, style.fontSize, style.bold = stylemodel.DisplayBlock, 32, true
		style.margin = stylemodel.Edges{Top: 12, Bottom: 12}
	case "h2":
		style.display, style.fontSize, style.bold = stylemodel.DisplayBlock, 26, true
		style.margin = stylemodel.Edges{Top: 10, Bottom: 10}
	case "h3":
		style.display, style.fontSize, style.bold = stylemodel.DisplayBlock, 21, true
		style.margin = stylemodel.Edges{Top: 8, Bottom: 8}
	case "h4", "h5", "h6":
		style.display, style.bold = stylemodel.DisplayBlock, true
		style.margin = stylemodel.Edges{Top: 8, Bottom: 8}
	case "p":
		style.display = stylemodel.DisplayBlock
		style.margin.Bottom = 14
	case "button":
		style.bold = true
	case "iframe":
		style.display = stylemodel.DisplayInlineBlock
		style.width = stylemodel.SizeValue{Kind: stylemodel.SizeLength, Value: stylemodel.LengthPercentage{Pixels: 300}}
		style.height = stylemodel.SizeValue{Kind: stylemodel.SizeLength, Value: stylemodel.LengthPercentage{Pixels: 150}}
	case "li":
		style.display = stylemodel.DisplayBlock
		style.margin.Bottom = 6
	case "pre":
		style.display, style.fontSize = stylemodel.DisplayBlock, 15
		style.margin.Bottom = 14
	case "a":
		style.color = linkColor
	case "head", "script", "style", "noscript", "template":
		style.display = stylemodel.DisplayNone
	}
	return style
}

func defaultStyle() blockStyle {
	return blockStyle{fontSize: 16, fontFamilies: []string{"Growse Sans", "sans-serif"}, fontStyle: "normal", fontStretch: "normal", color: textColor, decorationColor: textColor, opacity: 1, display: stylemodel.DisplayInline, listStyleType: stylemodel.ListStyleDisc, accentColorAuto: true}
}

func applyComputed(block blockStyle, computed stylemodel.ComputedStyle) blockStyle {
	block.fontSize = computed.FontSize
	block.bold = computed.Bold()
	block.fontFamilies = append([]string(nil), computed.FontFamilies...)
	block.fontStyle, block.fontStretch = computed.FontStyle, computed.FontStretch
	block.color = computed.Color
	block.background = computed.BackgroundColor
	block.image = cloneBackgroundImage(computed.BackgroundImage)
	block.repeat = computed.BackgroundRepeat
	block.position = computed.BackgroundPos
	block.backgroundSize = computed.BackgroundSize
	block.backgroundLayers = cloneBackgroundLayers(computed.BackgroundLayers)
	block.layoutPosition, block.inset = computed.Position, computed.Inset
	block.zIndex, block.zIndexAuto = computed.ZIndex, computed.ZIndexAuto
	block.boxShadows = append([]stylemodel.Shadow(nil), computed.BoxShadows...)
	block.textShadows = append([]stylemodel.Shadow(nil), computed.TextShadows...)
	block.outline, block.outlineOffset = computed.Outline, computed.OutlineOffset
	block.transform = append([]stylemodel.TransformFunction(nil), computed.Transform...)
	block.transformOrigin = computed.TransformOrigin
	block.radius = computed.BorderRadius
	block.decoration = computed.TextDecoration
	block.decorationColor = computed.DecorationColor
	block.opacity = computed.Opacity
	block.display = computed.Display
	block.float = computed.Float
	block.clear = computed.Clear
	block.hidden = computed.Visibility == stylemodel.VisibilityHidden
	block.margin = computed.Margin
	block.padding = computed.Padding
	block.border = computed.Border
	block.boxSizing = computed.BoxSizing
	block.width, block.height = computed.Width, computed.Height
	block.minWidth, block.minHeight = computed.MinWidth, computed.MinHeight
	block.maxWidth, block.maxHeight = computed.MaxWidth, computed.MaxHeight
	block.lineHeight, block.whiteSpace = computed.LineHeight, computed.WhiteSpace
	block.textAlign, block.textTransform, block.textIndent = computed.TextAlign, computed.TextTransform, computed.TextIndent
	block.letterSpacing, block.wordSpacing = computed.LetterSpacing, computed.WordSpacing
	block.wordBreak, block.overflowWrap = computed.WordBreak, computed.OverflowWrap
	block.verticalAlign, block.textOverflow = computed.VerticalAlign, computed.TextOverflow
	block.objectFit, block.objectPosition = computed.ObjectFit, computed.ObjectPosition
	block.listStyleType, block.listStylePosition, block.listStyleImage = computed.ListStyleType, computed.ListStylePosition, computed.ListStyleImage
	block.appearance, block.accentColor, block.accentColorAuto, block.cursor = computed.Appearance, computed.AccentColor, computed.AccentColorAuto, computed.Cursor
	block.filters = append([]stylemodel.Filter(nil), computed.Filters...)
	block.backdropFilters = append([]stylemodel.Filter(nil), computed.BackdropFilters...)
	block.mixBlendMode = computed.MixBlendMode
	block.overflowX, block.overflowY = computed.OverflowX, computed.OverflowY
	block.flexDirection, block.flexWrap = computed.FlexDirection, computed.FlexWrap
	block.justifyContent, block.alignItems, block.justifyItems, block.alignContent = computed.JustifyContent, computed.AlignItems, computed.JustifyItems, computed.AlignContent
	block.order, block.flexGrow, block.flexShrink = computed.Order, computed.FlexGrow, computed.FlexShrink
	block.flexBasis, block.alignSelf, block.justifySelf = computed.FlexBasis, computed.AlignSelf, computed.JustifySelf
	block.rowGap, block.columnGap = computed.RowGap, computed.ColumnGap
	block.gridTemplateColumns = append([]stylemodel.GridTrackSize(nil), computed.GridTemplateColumns...)
	block.gridTemplateRows = append([]stylemodel.GridTrackSize(nil), computed.GridTemplateRows...)
	block.gridAutoColumns = append([]stylemodel.GridTrackSize(nil), computed.GridAutoColumns...)
	block.gridAutoRows = append([]stylemodel.GridTrackSize(nil), computed.GridAutoRows...)
	block.gridColumnLines = computed.GridColumnLines
	block.gridRowLines = computed.GridRowLines
	block.gridTemplateAreas = computed.GridTemplateAreas
	block.gridColumn = computed.GridColumn
	block.gridRow = computed.GridRow
	block.gridAreaName = computed.GridAreaName
	block.gridAutoFlow = computed.GridAutoFlow
	block.marginAuto, block.aspectRatio = computed.MarginAuto, computed.AspectRatio
	return block
}

func findElement(node *dom.Node, tag string) *dom.Node {
	if node == nil {
		return nil
	}
	if node.Type == dom.NodeElement && node.TagName == tag {
		return node
	}
	for _, child := range node.Children {
		if found := findElement(child, tag); found != nil {
			return found
		}
	}
	return nil
}

func normalizeWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
