// Package layout builds a simple visual tree from the Growse DOM.
package layout

import "github.com/saku0512/growse/internal/dom"

// Tree is the result of laying out one document at a specific viewport width.
type Tree struct {
	Width        float32
	Height       float32
	Background   uint32
	Boxes        []Box
	ScrollWidth  float32
	ScrollHeight float32
}

// Rect is a document-coordinate clipping rectangle.
type Rect struct {
	X, Y, Width, Height float32
}

// Box is one line of visible page content.
type Box struct {
	NodeID dom.NodeID
	Tag    string
	Text   string
	Input  bool

	X        float32
	Y        float32
	Width    float32
	Height   float32
	Baseline float32
	Clip     *Rect

	FontSize   float32
	Bold       bool
	Color      uint32
	Background uint32
	Runs       []TextRun
}

// TextRun is a continuously styled fragment inside one line box.
type TextRun struct {
	NodeID dom.NodeID
	Tag    string
	Text   string
	Width  float32

	FontSize   float32
	Bold       bool
	Color      uint32
	Background uint32
	Baseline   float32
}
