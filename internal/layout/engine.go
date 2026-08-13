package layout

import (
	"strings"
	"unicode"

	"github.com/saku0512/growse/internal/dom"
	stylemodel "github.com/saku0512/growse/internal/style"
)

const (
	pagePadding = float32(32)
	textColor   = uint32(0x202124ff)
	linkColor   = uint32(0x0969daff)
	inputWidth  = float32(280)
	inputHeight = float32(40)
)

type blockStyle struct {
	fontSize        float32
	bold            bool
	color           uint32
	background      uint32
	image           stylemodel.BackgroundImage
	repeat          stylemodel.BackgroundRepeat
	position        stylemodel.BackgroundPosition
	backgroundSize  stylemodel.BackgroundSize
	radius          stylemodel.BorderRadii
	decoration      stylemodel.TextDecorationLine
	decorationColor uint32
	opacity         float32
	display         stylemodel.Display
	margin          stylemodel.Edges
	padding         stylemodel.Edges
	border          stylemodel.Borders
	boxSizing       stylemodel.BoxSizing
	width           stylemodel.SizeValue
	height          stylemodel.SizeValue
	minWidth        stylemodel.SizeValue
	minHeight       stylemodel.SizeValue
	maxWidth        stylemodel.SizeValue
	maxHeight       stylemodel.SizeValue
	lineHeight      float32
	whiteSpace      stylemodel.WhiteSpace
	overflowX       stylemodel.Overflow
	overflowY       stylemodel.Overflow
}

type inlineRun struct {
	nodeID  dom.NodeID
	tag     string
	text    string
	style   blockStyle
	atomic  bool
	width   float32
	height  float32
	opacity float32
}

// Build creates a vertical block layout with a minimal inline text flow.
func Build(document *dom.Document, computed stylemodel.Map, viewportWidth float32) *Tree {
	return build(document, computed, viewportWidth, 0)
}

// BuildWithViewport lays out a document with definite viewport dimensions.
func BuildWithViewport(document *dom.Document, computed stylemodel.Map, viewportWidth, viewportHeight float32) *Tree {
	return build(document, computed, viewportWidth, viewportHeight)
}

func build(document *dom.Document, computed stylemodel.Map, viewportWidth, viewportHeight float32) *Tree {
	if viewportWidth < pagePadding*2+1 {
		viewportWidth = pagePadding*2 + 1
	}

	tree := &Tree{Width: viewportWidth, Background: 0xffffffff}
	state := engine{
		tree:     tree,
		computed: computed,
		y:        pagePadding,
		opacity:  1,
	}
	if document != nil {
		if body := findElement(document.Root, "body"); body != nil {
			if bodyStyle, ok := computed.For(body); ok && bodyStyle.BackgroundColor != 0 {
				tree.Background = bodyStyle.BackgroundColor
			}
		}
		state.walk(document.Root, pagePadding, viewportWidth-pagePadding*2, viewportHeight, viewportHeight > 0)
	}
	tree.Height = state.y + pagePadding
	tree.ScrollWidth, tree.ScrollHeight = tree.Width, tree.Height
	for _, box := range tree.Boxes {
		contentWidth := box.Width
		if len(box.Runs) != 0 {
			contentWidth = 0
			for _, run := range box.Runs {
				contentWidth += run.Width
			}
		}
		tree.ScrollWidth = max(tree.ScrollWidth, box.X+contentWidth+pagePadding)
		tree.ScrollHeight = max(tree.ScrollHeight, box.Y+box.Height+pagePadding)
	}
	for _, decoration := range tree.Decorations {
		tree.ScrollWidth = max(tree.ScrollWidth, decoration.X+decoration.Width+pagePadding)
		tree.ScrollHeight = max(tree.ScrollHeight, decoration.Y+decoration.Height+pagePadding)
	}
	return tree
}

type engine struct {
	tree     *Tree
	computed stylemodel.Map
	y        float32
	clip     *Rect
	order    int
	opacity  float32
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
			e.addText(node.ID, "text", text, textStyle, x, width)
		}
		return
	case dom.NodeElement:
		style := e.styleFor(node)
		if style.display == stylemodel.DisplayNone {
			return
		}
		if isTextInput(node) {
			e.addInput(node, style, x, width, containingHeight, heightDefinite)
			return
		}
		if style.display == stylemodel.DisplayBlock {
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
	if resolved, ok := resolveSize(style.height, containingHeight, heightDefinite); ok {
		usedHeight = resolved
	}
	usedHeight = constrainSize(usedHeight, style.minHeight, style.maxHeight, containingHeight, heightDefinite)
	value, _ := node.Attribute("value")
	e.tree.Boxes = append(e.tree.Boxes, Box{
		Order:   e.nextOrder(),
		NodeID:  node.ID,
		Tag:     node.TagName,
		Text:    value,
		Input:   true,
		X:       x,
		Y:       e.y,
		Width:   usedWidth,
		Height:  usedHeight,
		Color:   style.color,
		Clip:    cloneRect(e.clip),
		Opacity: e.opacity * style.opacity,
	})
	e.y += usedHeight + style.margin.Bottom
}

func isTextInput(node *dom.Node) bool {
	if node == nil || node.Type != dom.NodeElement || node.TagName != "input" {
		return false
	}
	typeValue, ok := node.Attribute("type")
	return !ok || strings.EqualFold(strings.TrimSpace(typeValue), "text")
}

func (e *engine) addBlock(node *dom.Node, style blockStyle, x, width, containingHeight float32, heightDefinite bool, topMargin *float32) {
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
	if style.background != 0 || style.image.Kind != stylemodel.BackgroundImageNone || hasVisibleBorder(style.border) {
		decorationIndex = len(e.tree.Decorations)
		e.tree.Decorations = append(e.tree.Decorations, Decoration{
			Order: e.nextOrder(), NodeID: node.ID,
			Rect:       Rect{X: x, Y: boxTop, Width: outerWidth},
			Background: style.background, Image: cloneBackgroundImage(style.image),
			Repeat: style.repeat, Position: style.position, Size: style.backgroundSize, Clip: cloneRect(e.clip),
			Border: style.border, Opacity: e.opacity,
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
	if (style.overflowX != stylemodel.OverflowVisible || style.overflowY != stylemodel.OverflowVisible) && declaredHeightDefinite {
		clipHeight := declaredHeight
		if style.boxSizing == stylemodel.BoxSizingContentBox {
			clipHeight += style.padding.Top + style.padding.Bottom
		}
		e.clip = intersectClip(previousClip, Rect{
			X: x + style.border.Left.Width, Y: boxTop + style.border.Top.Width,
			Width: outerWidth - horizontalBorder, Height: clipHeight,
		})
	}

	inlineRuns := e.generatedRuns(node, true, style)
	previousBlock := false
	previousBottomMargin := float32(0)
	flushInline := func() {
		if len(inlineRuns) != 0 {
			e.addInlineRuns(node.ID, node.TagName, inlineRuns, style, contentX, contentWidth)
			previousBlock = false
		}
		inlineRuns = inlineRuns[:0]
	}

	for _, child := range node.Children {
		if child.Type == dom.NodeElement {
			childStyle := e.styleFor(child)
			if childStyle.display == stylemodel.DisplayNone {
				continue
			}
			if isTextInput(child) {
				flushInline()
				e.addInput(child, childStyle, contentX, contentWidth, childContainingHeight, declaredHeightDefinite)
				previousBlock = true
				previousBottomMargin = childStyle.margin.Bottom
				continue
			}
			if childStyle.display == stylemodel.DisplayBlock {
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
	e.clip = previousClip

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
	if decorationIndex >= 0 {
		e.tree.Decorations[decorationIndex].Height = outerHeight
		e.tree.Decorations[decorationIndex].Radius = resolveBorderRadii(style.radius, outerWidth, outerHeight)
	}
	e.y = boxTop + outerHeight + style.margin.Bottom
	e.opacity = previousOpacity
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
	if style.display == stylemodel.DisplayInlineBlock {
		return []inlineRun{{nodeID: node.ID, tag: node.TagName, text: e.inlineText(node), style: style, atomic: true, opacity: opacity}}
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
	var lineText strings.Builder
	var usedWidth, lineHeight, lineAscent float32
	var pendingSpace *inlineRun

	flushLine := func() {
		if len(lineRuns) == 0 {
			return
		}
		if lineHeight == 0 {
			lineHeight = container.fontSize * 1.4
		}
		e.tree.Boxes = append(e.tree.Boxes, Box{
			Order:  e.nextOrder(),
			NodeID: nodeID, Tag: tag, Text: lineText.String(),
			X: x, Y: e.y, Width: width, Height: lineHeight,
			FontSize: container.fontSize, Bold: container.bold, Color: container.color,
			Opacity: e.opacity, Decoration: container.decoration, DecorationColor: container.decorationColor,
			Runs:     append([]TextRun(nil), lineRuns...),
			Baseline: e.y + lineAscent, Clip: cloneRect(e.clip),
		})
		e.y += lineHeight
		lineRuns = lineRuns[:0]
		lineText.Reset()
		usedWidth, lineHeight, lineAscent, pendingSpace = 0, 0, 0, nil
	}

	appendPiece := func(run inlineRun, text string, pieceWidth float32) {
		textRun := TextRun{
			NodeID: run.nodeID, Tag: run.tag, Text: text, Width: pieceWidth,
			FontSize: run.style.fontSize, Bold: run.style.bold,
			Color: run.style.color, Background: run.style.background,
			Decoration: run.style.decoration, DecorationColor: run.style.decorationColor, Opacity: run.opacity,
		}
		runHeight, runAscent := usedLineMetrics(run)
		textRun.Baseline = e.y + runAscent
		if len(lineRuns) > 0 && sameTextStyle(lineRuns[len(lineRuns)-1], textRun) {
			lineRuns[len(lineRuns)-1].Text += text
			lineRuns[len(lineRuns)-1].Width += pieceWidth
		} else {
			lineRuns = append(lineRuns, textRun)
		}
		lineText.WriteString(text)
		usedWidth += pieceWidth
		height := runHeight
		if height > lineHeight {
			lineHeight = height
		}
		if runAscent > lineAscent {
			lineAscent = runAscent
		}
	}

	for _, token := range tokenizeInlineRuns(runs) {
		if token.atomic {
			token.width, token.height = resolveAtomicSize(token, width)
			if usedWidth > 0 && usedWidth+token.width > width && wrapsWhitespace(token.style.whiteSpace) {
				flushLine()
			}
			appendPiece(token, token.text, token.width)
			continue
		}
		if token.text == "\n" {
			flushLine()
			continue
		}
		if token.text == " " {
			if preservesSpaces(token.style.whiteSpace) {
				spaceWidth, _, _ := measureText(" ", token.style.fontSize, token.style.bold)
				if usedWidth > 0 && usedWidth+spaceWidth > width && wrapsWhitespace(token.style.whiteSpace) {
					flushLine()
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
			spaceWidth, _, _ = measureText(" ", pendingSpace.style.fontSize, pendingSpace.style.bold)
		}
		wordWidth, _, _ := measureText(token.text, token.style.fontSize, token.style.bold)
		if usedWidth > 0 && usedWidth+spaceWidth+wordWidth > width && wrapsWhitespace(token.style.whiteSpace) {
			flushLine()
			spaceWidth = 0
		}
		if pendingSpace != nil && usedWidth > 0 {
			appendPiece(*pendingSpace, " ", spaceWidth)
		}
		pendingSpace = nil

		remaining := []rune(token.text)
		for len(remaining) > 0 {
			available := width - usedWidth
			mWidth, _, _ := measureText("m", token.style.fontSize, token.style.bold)
			characters := int(available / max(mWidth, float32(1)))
			if characters < 1 {
				if wrapsWhitespace(token.style.whiteSpace) {
					flushLine()
					continue
				}
				characters = len(remaining)
			}
			if characters > len(remaining) {
				characters = len(remaining)
			}
			piece := string(remaining[:characters])
			pieceWidth, _, _ := measureText(piece, token.style.fontSize, token.style.bold)
			appendPiece(token, piece, pieceWidth)
			remaining = remaining[characters:]
			if len(remaining) > 0 && wrapsWhitespace(token.style.whiteSpace) {
				flushLine()
			}
		}
	}
	flushLine()
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
	width, _, _ := measureText(normalizeWhitespace(run.text), run.style.fontSize, run.style.bold)
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
		left.Decoration == right.Decoration && left.DecorationColor == right.DecorationColor && left.Opacity == right.Opacity
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

func cloneBackgroundImage(source stylemodel.BackgroundImage) stylemodel.BackgroundImage {
	result := source
	result.GradientStops = append([]stylemodel.GradientStop(nil), source.GradientStops...)
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
	return blockStyle{fontSize: 16, color: textColor, decorationColor: textColor, opacity: 1, display: stylemodel.DisplayInline}
}

func applyComputed(block blockStyle, computed stylemodel.ComputedStyle) blockStyle {
	block.fontSize = computed.FontSize
	block.bold = computed.Bold()
	block.color = computed.Color
	block.background = computed.BackgroundColor
	block.image = cloneBackgroundImage(computed.BackgroundImage)
	block.repeat = computed.BackgroundRepeat
	block.position = computed.BackgroundPos
	block.backgroundSize = computed.BackgroundSize
	block.radius = computed.BorderRadius
	block.decoration = computed.TextDecoration
	block.decorationColor = computed.DecorationColor
	block.opacity = computed.Opacity
	block.display = computed.Display
	block.margin = computed.Margin
	block.padding = computed.Padding
	block.border = computed.Border
	block.boxSizing = computed.BoxSizing
	block.width, block.height = computed.Width, computed.Height
	block.minWidth, block.minHeight = computed.MinWidth, computed.MinHeight
	block.maxWidth, block.maxHeight = computed.MaxWidth, computed.MaxHeight
	block.lineHeight, block.whiteSpace = computed.LineHeight, computed.WhiteSpace
	block.overflowX, block.overflowY = computed.OverflowX, computed.OverflowY
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
