// Package html parses HTML and converts it into Growse's own DOM.
package html

import (
	"fmt"
	"io"

	"github.com/Grove-Computing/Growse/internal/dom"
	xhtml "golang.org/x/net/html"
)

// Parse parses HTML syntax and returns a Growse-owned document.
func Parse(reader io.Reader) (*dom.Document, error) {
	root, err := xhtml.Parse(reader)
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}
	return Convert(root)
}
