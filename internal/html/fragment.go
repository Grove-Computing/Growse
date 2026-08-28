package html

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// ParseFragment parses an HTML fragment in the supplied element context.
func ParseFragment(source, contextTag string) (*dom.Document, error) {
	contextTag = strings.ToLower(strings.TrimSpace(contextTag))
	context := &xhtml.Node{Type: xhtml.ElementNode, Data: contextTag, DataAtom: atom.Lookup([]byte(contextTag))}
	nodes, err := xhtml.ParseFragment(strings.NewReader(source), context)
	if err != nil {
		return nil, fmt.Errorf("parse HTML fragment: %w", err)
	}
	document := dom.NewDocument()
	for _, node := range nodes {
		if err := convertNode(document, document.Root, node, 1); err != nil {
			return nil, err
		}
	}
	return document, nil
}

// SerializeChildren renders an element's child nodes as an HTML fragment.
func SerializeChildren(parent *dom.Node) (string, error) {
	if parent == nil {
		return "", nil
	}
	var output bytes.Buffer
	for _, child := range parent.Children {
		if err := xhtml.Render(&output, renderNode(child)); err != nil {
			return "", fmt.Errorf("serialize HTML fragment: %w", err)
		}
	}
	return output.String(), nil
}

// SerializeNode renders one DOM node and its subtree as HTML.
func SerializeNode(node *dom.Node) (string, error) {
	if node == nil {
		return "", nil
	}
	var output bytes.Buffer
	if err := xhtml.Render(&output, renderNode(node)); err != nil {
		return "", fmt.Errorf("serialize HTML node: %w", err)
	}
	return output.String(), nil
}

func renderNode(source *dom.Node) *xhtml.Node {
	if source == nil {
		return nil
	}
	target := &xhtml.Node{}
	switch source.Type {
	case dom.NodeText:
		target.Type, target.Data = xhtml.TextNode, source.Text
	case dom.NodeElement:
		target.Type, target.Data, target.DataAtom = xhtml.ElementNode, source.TagName, atom.Lookup([]byte(source.TagName))
		names := make([]string, 0, len(source.Attributes))
		for name := range source.Attributes {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			target.Attr = append(target.Attr, xhtml.Attribute{Key: name, Val: source.Attributes[name]})
		}
	case dom.NodeDocument, dom.NodeDocumentFragment:
		target.Type = xhtml.DocumentNode
	}
	for _, child := range source.Children {
		if source.Type == dom.NodeElement && source.TagName == "template" && child.Type == dom.NodeDocumentFragment {
			for _, contentChild := range child.Children {
				if converted := renderNode(contentChild); converted != nil {
					target.AppendChild(converted)
				}
			}
			continue
		}
		if converted := renderNode(child); converted != nil {
			target.AppendChild(converted)
		}
	}
	return target
}
