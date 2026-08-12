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
	fontSize     float32
	bold         bool
	color        uint32
	background   uint32
	marginTop    float32
	marginBottom float32
}

// Build creates a minimal vertical layout using Growse's UA defaults.
func Build(document *dom.Document, computed stylemodel.Map, viewportWidth float32) *Tree {
	if viewportWidth < pagePadding*2+1 {
		viewportWidth = pagePadding*2 + 1
	}

	tree := &Tree{Width: viewportWidth, Background: 0xffffffff}
	state := engine{
		tree:         tree,
		computed:     computed,
		contentWidth: viewportWidth - pagePadding*2,
		y:            pagePadding,
	}
	if document != nil {
		if body := findElement(document.Root, "body"); body != nil {
			if bodyStyle, ok := computed.For(body); ok && bodyStyle.BackgroundColor != 0 {
				tree.Background = bodyStyle.BackgroundColor
			}
		}
		state.walk(document.Root)
	}
	tree.Height = state.y + pagePadding
	return tree
}

type engine struct {
	tree         *Tree
	computed     stylemodel.Map
	contentWidth float32
	y            float32
}

func (e *engine) walk(node *dom.Node) {
	if node == nil {
		return
	}
	if node.Type == dom.NodeElement {
		if isHidden(node.TagName) {
			return
		}
		if style, ok := visibleBlockStyle(node.TagName); ok {
			e.addBlock(node, style)
			return
		}
	}

	if node.Type == dom.NodeText {
		text := normalizeWhitespace(node.Text)
		if text != "" {
			textStyle := defaultStyle()
			if computed, ok := e.computed.For(node); ok {
				textStyle = applyComputed(textStyle, computed)
			}
			e.addText(node.ID, "text", text, textStyle)
		}
		return
	}

	for _, child := range node.Children {
		e.walk(child)
	}
}

func (e *engine) addBlock(node *dom.Node, style blockStyle) {
	text := normalizeWhitespace(node.TextContent())
	if text == "" {
		return
	}
	if computed, ok := e.computed.For(node); ok {
		style = applyComputed(style, computed)
	}
	e.y += style.marginTop
	e.addText(node.ID, node.TagName, text, style)
	e.y += style.marginBottom
}

func (e *engine) addText(nodeID dom.NodeID, tag, text string, style blockStyle) {
	lineHeight := style.fontSize * 1.4
	for _, line := range wrapText(text, e.contentWidth, style.fontSize) {
		e.tree.Boxes = append(e.tree.Boxes, Box{
			NodeID:     nodeID,
			Tag:        tag,
			Text:       line,
			X:          pagePadding,
			Y:          e.y,
			Width:      e.contentWidth,
			Height:     lineHeight,
			FontSize:   style.fontSize,
			Bold:       style.bold,
			Color:      style.color,
			Background: style.background,
		})
		e.y += lineHeight
	}
}

func visibleBlockStyle(tag string) (blockStyle, bool) {
	switch tag {
	case "h1":
		return blockStyle{fontSize: 32, bold: true, color: textColor, marginTop: 12, marginBottom: 12}, true
	case "h2":
		return blockStyle{fontSize: 26, bold: true, color: textColor, marginTop: 10, marginBottom: 10}, true
	case "h3":
		return blockStyle{fontSize: 21, bold: true, color: textColor, marginTop: 8, marginBottom: 8}, true
	case "p":
		return blockStyle{fontSize: 16, color: textColor, marginBottom: 14}, true
	case "button":
		return blockStyle{fontSize: 16, bold: true, color: textColor, marginTop: 4, marginBottom: 12}, true
	case "li":
		return blockStyle{fontSize: 16, color: textColor, marginBottom: 6}, true
	case "pre":
		return blockStyle{fontSize: 15, color: textColor, marginBottom: 14}, true
	case "a":
		return blockStyle{fontSize: 16, color: linkColor, marginBottom: 8}, true
	default:
		return blockStyle{}, false
	}
}

func defaultStyle() blockStyle {
	return blockStyle{fontSize: 16, color: textColor, marginBottom: 8}
}

func applyComputed(block blockStyle, computed stylemodel.ComputedStyle) blockStyle {
	block.fontSize = computed.FontSize
	block.bold = computed.Bold()
	block.color = computed.Color
	block.background = computed.BackgroundColor
	return block
}

func isHidden(tag string) bool {
	switch tag {
	case "head", "script", "style", "noscript", "template":
		return true
	default:
		return false
	}
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
