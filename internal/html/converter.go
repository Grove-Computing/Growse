package html

import (
	"fmt"

	"github.com/Grove-Computing/Growse/internal/dom"
	xhtml "golang.org/x/net/html"
)

// Convert translates an x/net/html tree into a Growse DOM tree.
func Convert(root *xhtml.Node) (*dom.Document, error) {
	if root == nil {
		return nil, fmt.Errorf("convert HTML: root is nil")
	}

	document := dom.NewDocument()
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if err := convertNode(document, document.Root, child); err != nil {
			return nil, err
		}
	}
	return document, nil
}

func convertNode(document *dom.Document, parent *dom.Node, source *xhtml.Node) error {
	var target *dom.Node

	switch source.Type {
	case xhtml.ElementNode:
		attributes := make(map[string]string, len(source.Attr))
		for _, attribute := range source.Attr {
			attributes[attribute.Key] = attribute.Val
		}
		target = document.CreateElement(source.Data, attributes)
	case xhtml.TextNode:
		target = document.CreateText(source.Data)
	default:
		return convertChildren(document, parent, source)
	}

	if err := document.AppendChild(parent, target); err != nil {
		return fmt.Errorf("append %q node: %w", source.Data, err)
	}
	if target.Type == dom.NodeElement && target.TagName == "template" {
		content := document.CreateDocumentFragment()
		if err := document.AppendChild(target, content); err != nil {
			return fmt.Errorf("append template content: %w", err)
		}
		return convertChildren(document, content, source)
	}
	return convertChildren(document, target, source)
}

func convertChildren(document *dom.Document, parent *dom.Node, source *xhtml.Node) error {
	for child := source.FirstChild; child != nil; child = child.NextSibling {
		if err := convertNode(document, parent, child); err != nil {
			return err
		}
	}
	return nil
}
