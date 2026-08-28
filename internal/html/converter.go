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
		if err := convertNode(document, document.Root, child, 1); err != nil {
			return nil, err
		}
	}
	return document, nil
}

func convertNode(document *dom.Document, parent *dom.Node, source *xhtml.Node, depth int) error {
	if depth > dom.MaxTreeDepth {
		return fmt.Errorf("convert HTML: DOM tree depth exceeds %d", dom.MaxTreeDepth)
	}
	var target *dom.Node

	switch source.Type {
	case xhtml.ElementNode:
		if len(source.Attr) > dom.MaxAttributesPerNode || len(source.Data) > dom.MaxDOMStringBytes {
			return fmt.Errorf("convert HTML: element payload exceeds DOM limit")
		}
		attributes := make(map[string]string, len(source.Attr))
		for _, attribute := range source.Attr {
			if len(attribute.Key) > dom.MaxDOMStringBytes || len(attribute.Val) > dom.MaxDOMStringBytes {
				return fmt.Errorf("convert HTML: attribute exceeds DOM string limit")
			}
			attributes[attribute.Key] = attribute.Val
		}
		if document.OwnedNodeCount() >= dom.MaxNodesPerDocument {
			return fmt.Errorf("convert HTML: DOM node count exceeds %d", dom.MaxNodesPerDocument)
		}
		target = document.CreateElement(source.Data, attributes)
	case xhtml.TextNode:
		parentTag := ""
		if parent != nil {
			parentTag = parent.TagName
		}
		if len(source.Data) > dom.TextLimitForParent(parentTag) || document.OwnedNodeCount() >= dom.MaxNodesPerDocument {
			return fmt.Errorf("convert HTML: text payload exceeds DOM limit")
		}
		target = document.CreateText(source.Data)
	default:
		return convertChildren(document, parent, source, depth)
	}

	if err := document.AppendChild(parent, target); err != nil {
		return fmt.Errorf("append %q node: %w", source.Data, err)
	}
	if target.Type == dom.NodeElement && target.TagName == "template" {
		if document.OwnedNodeCount() >= dom.MaxNodesPerDocument {
			return fmt.Errorf("convert HTML: DOM node count exceeds %d", dom.MaxNodesPerDocument)
		}
		content := document.CreateDocumentFragment()
		if err := document.AppendChild(target, content); err != nil {
			return fmt.Errorf("append template content: %w", err)
		}
		return convertChildren(document, content, source, depth+1)
	}
	return convertChildren(document, target, source, depth+1)
}

func convertChildren(document *dom.Document, parent *dom.Node, source *xhtml.Node, depth int) error {
	for child := source.FirstChild; child != nil; child = child.NextSibling {
		if err := convertNode(document, parent, child, depth); err != nil {
			return err
		}
	}
	return nil
}
