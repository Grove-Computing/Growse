package layout

import (
	"strings"
	"unicode"
	"unicode/utf8"

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
	fontSize   float32
	bold       bool
	color      uint32
	background uint32
	display    stylemodel.Display
	margin     stylemodel.Edges
	padding    stylemodel.Edges
	boxSizing  stylemodel.BoxSizing
	width      stylemodel.SizeValue
	height     stylemodel.SizeValue
	minWidth   stylemodel.SizeValue
	minHeight  stylemodel.SizeValue
	maxWidth   stylemodel.SizeValue
	maxHeight  stylemodel.SizeValue
}

type inlineRun struct {
	nodeID dom.NodeID
	tag    string
	text   string
	style  blockStyle
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
	return tree
}

type engine struct {
	tree     *Tree
	computed stylemodel.Map
	y        float32
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
			e.addBlock(node, style, x, width, containingHeight, heightDefinite)
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
		NodeID: node.ID,
		Tag:    node.TagName,
		Text:   value,
		Input:  true,
		X:      x,
		Y:      e.y,
		Width:  usedWidth,
		Height: usedHeight,
		Color:  style.color,
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

func (e *engine) addBlock(node *dom.Node, style blockStyle, x, width, containingHeight float32, heightDefinite bool) {
	e.y += style.margin.Top
	x += style.margin.Left
	availableWidth := width - style.margin.Left - style.margin.Right
	if availableWidth < 1 {
		availableWidth = 1
	}
	sizingWidth := availableWidth
	if style.boxSizing == stylemodel.BoxSizingContentBox {
		sizingWidth -= style.padding.Left + style.padding.Right
	}
	if resolved, ok := resolveSize(style.width, width, true); ok {
		sizingWidth = resolved
	}
	sizingWidth = constrainSize(sizingWidth, style.minWidth, style.maxWidth, width, true)
	outerWidth := sizingWidth
	if style.boxSizing == stylemodel.BoxSizingContentBox {
		outerWidth += style.padding.Left + style.padding.Right
	}
	if outerWidth > availableWidth && style.width.Kind == stylemodel.SizeAuto {
		outerWidth = availableWidth
	}
	if outerWidth < 1 {
		outerWidth = 1
	}
	contentX := x + style.padding.Left
	contentWidth := outerWidth - style.padding.Left - style.padding.Right
	if contentWidth < 1 {
		contentWidth = 1
	}
	boxTop := e.y
	e.y += style.padding.Top
	contentTop := e.y
	declaredHeight, declaredHeightDefinite := resolveSize(style.height, containingHeight, heightDefinite)
	childContainingHeight := declaredHeight
	if declaredHeightDefinite && style.boxSizing == stylemodel.BoxSizingBorderBox {
		childContainingHeight -= style.padding.Top + style.padding.Bottom
		if childContainingHeight < 0 {
			childContainingHeight = 0
		}
	}

	inlineRuns := e.generatedRuns(node, true, style)
	flushInline := func() {
		if len(inlineRuns) != 0 {
			e.addInlineRuns(node.ID, node.TagName, inlineRuns, style, contentX, contentWidth)
		}
		inlineRuns = inlineRuns[:0]
	}

	for _, child := range node.Children {
		if child.Type == dom.NodeElement {
			childStyle := e.styleFor(child)
			if childStyle.display == stylemodel.DisplayNone {
				continue
			}
			if childStyle.display == stylemodel.DisplayBlock {
				flushInline()
				e.walk(child, contentX, contentWidth, childContainingHeight, declaredHeightDefinite)
				continue
			}
		}
		inlineRuns = append(inlineRuns, e.collectInlineRuns(child, node)...)
	}
	inlineRuns = append(inlineRuns, e.generatedRuns(node, false, style)...)
	flushInline()

	contentHeight := e.y - contentTop
	sizingHeight := contentHeight
	if declaredHeightDefinite {
		sizingHeight = declaredHeight
	}
	sizingHeight = constrainSize(sizingHeight, style.minHeight, style.maxHeight, containingHeight, heightDefinite)
	outerHeight := sizingHeight
	if style.boxSizing == stylemodel.BoxSizingContentBox {
		outerHeight += style.padding.Top + style.padding.Bottom
	}
	if outerHeight < 0 {
		outerHeight = 0
	}
	e.y = boxTop + outerHeight + style.margin.Bottom
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
	if node == nil {
		return nil
	}
	if node.Type == dom.NodeText {
		style := e.styleFor(node)
		return []inlineRun{{nodeID: owner.ID, tag: owner.TagName, text: node.Text, style: style}}
	}
	if node.Type != dom.NodeElement {
		return nil
	}
	style := e.styleFor(node)
	if style.display == stylemodel.DisplayNone {
		return nil
	}
	if node.TagName == "br" {
		return []inlineRun{{nodeID: node.ID, tag: node.TagName, text: "\n", style: style}}
	}

	result := e.generatedRuns(node, true, style)
	for _, child := range node.Children {
		result = append(result, e.collectInlineRuns(child, node)...)
	}
	result = append(result, e.generatedRuns(node, false, style)...)
	return result
}

func (e *engine) generatedRuns(node *dom.Node, before bool, style blockStyle) []inlineRun {
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
	return []inlineRun{{nodeID: node.ID, tag: tag, text: text, style: style}}
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
	e.addInlineRuns(nodeID, tag, []inlineRun{{nodeID: nodeID, tag: tag, text: text, style: style}}, style, x, width)
}

func (e *engine) addInlineRuns(nodeID dom.NodeID, tag string, runs []inlineRun, container blockStyle, x, width float32) {
	var lineRuns []TextRun
	var lineText strings.Builder
	var usedWidth, lineHeight float32
	var pendingSpace *inlineRun

	flushLine := func() {
		if len(lineRuns) == 0 {
			return
		}
		if lineHeight == 0 {
			lineHeight = container.fontSize * 1.4
		}
		e.tree.Boxes = append(e.tree.Boxes, Box{
			NodeID: nodeID, Tag: tag, Text: lineText.String(),
			X: x, Y: e.y, Width: width, Height: lineHeight,
			FontSize: container.fontSize, Bold: container.bold, Color: container.color,
			Background: container.background, Runs: append([]TextRun(nil), lineRuns...),
		})
		e.y += lineHeight
		lineRuns = lineRuns[:0]
		lineText.Reset()
		usedWidth, lineHeight, pendingSpace = 0, 0, nil
	}

	appendPiece := func(run inlineRun, text string, pieceWidth float32) {
		textRun := TextRun{
			NodeID: run.nodeID, Tag: run.tag, Text: text, Width: pieceWidth,
			FontSize: run.style.fontSize, Bold: run.style.bold,
			Color: run.style.color, Background: run.style.background,
		}
		if len(lineRuns) > 0 && sameTextStyle(lineRuns[len(lineRuns)-1], textRun) {
			lineRuns[len(lineRuns)-1].Text += text
			lineRuns[len(lineRuns)-1].Width += pieceWidth
		} else {
			lineRuns = append(lineRuns, textRun)
		}
		lineText.WriteString(text)
		usedWidth += pieceWidth
		if height := run.style.fontSize * 1.4; height > lineHeight {
			lineHeight = height
		}
	}

	for _, token := range tokenizeInlineRuns(runs) {
		if token.text == "\n" {
			flushLine()
			continue
		}
		if token.text == " " {
			if len(lineRuns) > 0 {
				copy := token
				pendingSpace = &copy
			}
			continue
		}

		spaceWidth := float32(0)
		if pendingSpace != nil {
			spaceWidth = estimatedTextWidth(" ", pendingSpace.style.fontSize)
		}
		wordWidth := estimatedTextWidth(token.text, token.style.fontSize)
		if usedWidth > 0 && usedWidth+spaceWidth+wordWidth > width {
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
			characters := int(available / estimatedTextWidth("m", token.style.fontSize))
			if characters < 1 {
				flushLine()
				continue
			}
			if characters > len(remaining) {
				characters = len(remaining)
			}
			piece := string(remaining[:characters])
			appendPiece(token, piece, estimatedTextWidth(piece, token.style.fontSize))
			remaining = remaining[characters:]
			if len(remaining) > 0 {
				flushLine()
			}
		}
	}
	flushLine()
}

func tokenizeInlineRuns(runs []inlineRun) []inlineRun {
	var tokens []inlineRun
	for _, run := range runs {
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
				token := run
				token.text = "\n"
				tokens = append(tokens, token)
			} else if unicode.IsSpace(character) {
				flushWord()
				if len(tokens) == 0 || tokens[len(tokens)-1].text != " " {
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

func sameTextStyle(left, right TextRun) bool {
	return left.NodeID == right.NodeID && left.Tag == right.Tag && left.FontSize == right.FontSize &&
		left.Bold == right.Bold && left.Color == right.Color && left.Background == right.Background
}

func estimatedTextWidth(text string, fontSize float32) float32 {
	return float32(utf8.RuneCountInString(text)) * fontSize * 0.58
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
	return blockStyle{fontSize: 16, color: textColor, display: stylemodel.DisplayInline}
}

func applyComputed(block blockStyle, computed stylemodel.ComputedStyle) blockStyle {
	block.fontSize = computed.FontSize
	block.bold = computed.Bold()
	block.color = computed.Color
	block.background = computed.BackgroundColor
	block.display = computed.Display
	block.margin = computed.Margin
	block.padding = computed.Padding
	block.boxSizing = computed.BoxSizing
	block.width, block.height = computed.Width, computed.Height
	block.minWidth, block.minHeight = computed.MinWidth, computed.MinHeight
	block.maxWidth, block.maxHeight = computed.MaxWidth, computed.MaxHeight
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
