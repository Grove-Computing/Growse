// Package layout builds a simple visual tree from the Growse DOM.
package layout

import (
	"sort"

	"github.com/Grove-Computing/Growse/internal/dom"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

// Tree is the result of laying out one document at a specific viewport width.
type Tree struct {
	Width            float32
	Height           float32
	Background       uint32
	Decorations      []Decoration
	Boxes            []Box
	ScrollWidth      float32
	ScrollHeight     float32
	StackingContexts []StackingContext
}

// StackingContext records atomic paint-order ownership.
type StackingContext struct {
	Parent    int
	NodeID    dom.NodeID
	ZIndex    int
	Order     int
	Opacity   float32
	Offscreen bool
}

// ClipRegion is one nested rectangular or rounded clipping boundary.
type ClipRegion struct {
	Rect
	Radius BorderRadii
}

// PaintEntry references one visual in final forward paint order.
type PaintEntry struct {
	Order, StackingID         int
	BoxIndex, DecorationIndex int
}

// OrderedPaintEntries returns the shared paint and hit-test ordering.
func (t *Tree) OrderedPaintEntries() []PaintEntry {
	if t == nil {
		return nil
	}
	entries := make([]PaintEntry, 0, len(t.Boxes)+len(t.Decorations))
	for index, box := range t.Boxes {
		entries = append(entries, PaintEntry{Order: box.Order, StackingID: box.StackingID, BoxIndex: index, DecorationIndex: -1})
	}
	for index, decoration := range t.Decorations {
		entries = append(entries, PaintEntry{Order: decoration.Order, StackingID: decoration.StackingID, BoxIndex: -1, DecorationIndex: index})
	}
	sort.SliceStable(entries, func(left, right int) bool { return t.paintEntryLess(entries[left], entries[right]) })
	return entries
}

func (t *Tree) paintEntryLess(left, right PaintEntry) bool {
	if left.StackingID == right.StackingID {
		return left.Order < right.Order
	}
	leftContext, rightContext := t.rootChildContext(left.StackingID), t.rootChildContext(right.StackingID)
	if leftContext.ZIndex != rightContext.ZIndex {
		return leftContext.ZIndex < rightContext.ZIndex
	}
	if leftContext.Order != rightContext.Order {
		return leftContext.Order < rightContext.Order
	}
	return left.Order < right.Order
}

func (t *Tree) rootChildContext(id int) StackingContext {
	if id < 0 || id >= len(t.StackingContexts) {
		return StackingContext{}
	}
	context := t.StackingContexts[id]
	for context.Parent > 0 && context.Parent < len(t.StackingContexts) {
		context = t.StackingContexts[context.Parent]
	}
	return context
}

// Decoration is the border-box visual of one element. It is kept separately
// from line boxes so backgrounds can be painted before descendant content.
type Decoration struct {
	Order      int
	StackingID int
	NodeID     dom.NodeID
	Rect
	Background    uint32
	Image         stylemodel.BackgroundImage
	Layers        []stylemodel.BackgroundLayer
	Repeat        stylemodel.BackgroundRepeat
	Position      stylemodel.BackgroundPosition
	Size          stylemodel.BackgroundSize
	Border        stylemodel.Borders
	Radius        BorderRadii
	Opacity       float32
	Clip          *Rect
	Clips         []ClipRegion
	BoxShadows    []stylemodel.Shadow
	Outline       stylemodel.BorderSide
	OutlineOffset float32
	Transform     stylemodel.Matrix
	Hidden        bool
}

// CornerRadius is one resolved elliptical radius in CSS pixels.
type CornerRadius struct{ X, Y float32 }

// BorderRadii contains resolved radii in clockwise order.
type BorderRadii struct {
	TopLeft, TopRight, BottomRight, BottomLeft CornerRadius
}

// Rect is a document-coordinate clipping rectangle.
type Rect struct {
	X, Y, Width, Height float32
}

// Box is one line of visible page content.
type Box struct {
	Order      int
	StackingID int
	NodeID     dom.NodeID
	Tag        string
	Text       string
	Input      bool

	X        float32
	Y        float32
	Width    float32
	Height   float32
	Baseline float32
	Clip     *Rect
	Clips    []ClipRegion
	Opacity  float32

	FontSize        float32
	Bold            bool
	Color           uint32
	Background      uint32
	Decoration      stylemodel.TextDecorationLine
	DecorationColor uint32
	TextShadows     []stylemodel.Shadow
	Transform       stylemodel.Matrix
	Hidden          bool
	Runs            []TextRun
}

// TextRun is a continuously styled fragment inside one line box.
type TextRun struct {
	NodeID dom.NodeID
	Tag    string
	Text   string
	Width  float32

	FontSize        float32
	Bold            bool
	Color           uint32
	Background      uint32
	Baseline        float32
	Decoration      stylemodel.TextDecorationLine
	DecorationColor uint32
	Opacity         float32
	TextShadows     []stylemodel.Shadow
}
