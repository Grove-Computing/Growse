// Package desktopassets exposes the canonical Growse desktop icon to the application.
package desktopassets

import _ "embed"

//go:embed io.github.grovecomputing.Growse.png
var iconPNG []byte

// IconPNG returns the canonical PNG used by desktop packages and the internal home.
func IconPNG() []byte {
	return iconPNG
}
