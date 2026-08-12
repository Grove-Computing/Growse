// Package layout builds a simple visual tree from the Growse DOM.
package layout

import "github.com/saku0512/growse/internal/dom"

// Tree is the result of laying out one document at a specific viewport width.
type Tree struct {
	Width  float32
	Height float32
	Boxes  []Box
}

// Box is one line of visible page content.
type Box struct {
	NodeID dom.NodeID
	Tag    string
	Text   string

	X      float32
	Y      float32
	Width  float32
	Height float32

	FontSize float32
	Bold     bool
	Color    uint32
}
