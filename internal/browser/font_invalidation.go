package browser

import (
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
)

// GlyphRunInvalidation identifies one contiguous rune range whose selected
// face changed. It intentionally contains no glyph or font payload.
type GlyphRunInvalidation struct {
	TextNode  dom.NodeID
	RuneStart int
	RuneEnd   int
}

// FontInvalidation describes the bounded layout work caused by a web-font
// completion. Browser chrome and text using other families are never included.
type FontInvalidation struct {
	Revision uint64
	Runs     []GlyphRunInvalidation
}

const maxFontInvalidationRuns = 1024

// CommitWebFontCompletion installs a decoded face and marks only text spans
// covered by its family and unicode-range for re-shaping.
func (p *Page) CommitWebFontCompletion(resource FontResource) FontInvalidation {
	if p == nil || !resource.Loaded || !resource.Decoded || resource.Face == nil {
		return FontInvalidation{}
	}
	p.fontMu.Lock()
	defer p.fontMu.Unlock()

	replaced := false
	for index := range p.Fonts {
		if sameFontResource(p.Fonts[index], resource) {
			p.Fonts[index] = resource
			replaced = true
			break
		}
	}
	if !replaced {
		p.Fonts = append(p.Fonts, resource)
	}
	p.WebFonts = layoutPageFonts(p.Fonts, p.Compatibility == CompatibilityProfileModernWeb)
	p.fontDirty.Revision++
	p.fontDirty.Runs = affectedGlyphRuns(p, resource, maxFontInvalidationRuns)
	return cloneFontInvalidation(p.fontDirty)
}

func (p *Page) FontInvalidationSnapshot() FontInvalidation {
	if p == nil {
		return FontInvalidation{}
	}
	p.fontMu.Lock()
	defer p.fontMu.Unlock()
	return cloneFontInvalidation(p.fontDirty)
}

func cloneFontInvalidation(source FontInvalidation) FontInvalidation {
	source.Runs = append([]GlyphRunInvalidation(nil), source.Runs...)
	return source
}

func sameFontResource(left, right FontResource) bool {
	if left.URL != "" || right.URL != "" {
		return left.URL == right.URL
	}
	return strings.EqualFold(left.Family, right.Family) && strings.EqualFold(left.Style, right.Style) && left.Weight == right.Weight && left.Stretch == right.Stretch
}

func affectedGlyphRuns(page *Page, resource FontResource, limit int) []GlyphRunInvalidation {
	if page.Document == nil || limit <= 0 {
		return nil
	}
	var result []GlyphRunInvalidation
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if node == nil || len(result) >= limit {
			return
		}
		if node.Type == dom.NodeText {
			computed, exists := inheritedTextStyle(page, node)
			if !exists || !fontFamilySelected(computed.FontFamilies, resource.Family) {
				return
			}
			start := -1
			for index, value := range []rune(node.Text) {
				covered := fontResourceCovers(resource, value)
				if covered && start < 0 {
					start = index
				}
				if !covered && start >= 0 {
					result = append(result, GlyphRunInvalidation{TextNode: node.ID, RuneStart: start, RuneEnd: index})
					start = -1
					if len(result) >= limit {
						return
					}
				}
			}
			if start >= 0 && len(result) < limit {
				result = append(result, GlyphRunInvalidation{TextNode: node.ID, RuneStart: start, RuneEnd: len([]rune(node.Text))})
			}
			return
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(page.Document.Root)
	return result
}

func inheritedTextStyle(page *Page, textNode *dom.Node) (computedStyle, bool) {
	for current := textNode.Parent; current != nil; current = current.Parent {
		if computed, exists := page.ComputedStyles[current.ID]; exists {
			return computedStyle{FontFamilies: computed.FontFamilies}, true
		}
	}
	return computedStyle{}, false
}

// computedStyle keeps this traversal independent of the much larger style
// value while retaining the only property needed for face selection.
type computedStyle struct{ FontFamilies []string }

func fontFamilySelected(families []string, target string) bool {
	target = normalizeFontFamily(target)
	for _, family := range families {
		if normalizeFontFamily(family) == target {
			return true
		}
	}
	return false
}

func normalizeFontFamily(value string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(value), "\"'"))
}

func fontResourceCovers(resource FontResource, value rune) bool {
	if len(resource.UnicodeRanges) == 0 {
		return true
	}
	for _, interval := range resource.UnicodeRanges {
		if value >= interval.Start && value <= interval.End {
			return true
		}
	}
	return false
}
