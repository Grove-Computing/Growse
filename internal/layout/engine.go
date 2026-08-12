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
)

type blockStyle struct {
	fontSize   float32
	bold       bool
	color      uint32
	background uint32
	display    stylemodel.Display
	margin     stylemodel.Edges
	padding    stylemodel.Edges
}

// Build creates a vertical block layout with a minimal inline text flow.
func Build(document *dom.Document, computed stylemodel.Map, viewportWidth float32) *Tree {
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
		state.walk(document.Root, pagePadding, viewportWidth-pagePadding*2)
	}
	tree.Height = state.y + pagePadding
	return tree
}

type engine struct {
	tree     *Tree
	computed stylemodel.Map
	y        float32
}

func (e *engine) walk(node *dom.Node, x, width float32) {
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
		if style.display == stylemodel.DisplayBlock {
			e.addBlock(node, style, x, width)
			return
		}
		text := e.inlineText(node)
		if text != "" {
			e.addText(node.ID, node.TagName, text, style, x, width)
		}
		return
	}

	for _, child := range node.Children {
		e.walk(child, x, width)
	}
}

func (e *engine) addBlock(node *dom.Node, style blockStyle, x, width float32) {
	e.y += style.margin.Top
	x += style.margin.Left
	width -= style.margin.Left + style.margin.Right
	if width < 1 {
		width = 1
	}

	contentX := x + style.padding.Left
	contentWidth := width - style.padding.Left - style.padding.Right
	if contentWidth < 1 {
		contentWidth = 1
	}
	e.y += style.padding.Top

	var inlineParts []string
	flushInline := func() {
		text := normalizeWhitespace(strings.Join(inlineParts, ""))
		if text != "" {
			e.addText(node.ID, node.TagName, text, style, contentX, contentWidth)
		}
		inlineParts = inlineParts[:0]
	}

	for _, child := range node.Children {
		if child.Type == dom.NodeElement {
			childStyle := e.styleFor(child)
			if childStyle.display == stylemodel.DisplayNone {
				continue
			}
			if childStyle.display == stylemodel.DisplayBlock {
				flushInline()
				e.walk(child, contentX, contentWidth)
				continue
			}
		}
		if text := e.inlineText(child); text != "" {
			inlineParts = append(inlineParts, text)
		}
	}
	flushInline()

	e.y += style.padding.Bottom + style.margin.Bottom
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
	lineHeight := style.fontSize * 1.4
	for _, line := range wrapText(text, width, style.fontSize) {
		e.tree.Boxes = append(e.tree.Boxes, Box{
			NodeID:     nodeID,
			Tag:        tag,
			Text:       line,
			X:          x,
			Y:          e.y,
			Width:      width,
			Height:     lineHeight,
			FontSize:   style.fontSize,
			Bold:       style.bold,
			Color:      style.color,
			Background: style.background,
		})
		e.y += lineHeight
	}
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

func wrapText(text string, width, fontSize float32) []string {
	maxRunes := int(width / (fontSize * 0.58))
	if maxRunes < 1 {
		maxRunes = 1
	}
	if utf8.RuneCountInString(text) <= maxRunes {
		return []string{text}
	}

	var lines []string
	remaining := []rune(text)
	for len(remaining) > maxRunes {
		cut := maxRunes
		for i := maxRunes; i > maxRunes/2; i-- {
			if unicode.IsSpace(remaining[i-1]) {
				cut = i - 1
				break
			}
		}
		line := strings.TrimSpace(string(remaining[:cut]))
		if line != "" {
			lines = append(lines, line)
		}
		remaining = remaining[cut:]
		for len(remaining) > 0 && unicode.IsSpace(remaining[0]) {
			remaining = remaining[1:]
		}
	}
	if line := strings.TrimSpace(string(remaining)); line != "" {
		lines = append(lines, line)
	}
	return lines
}
