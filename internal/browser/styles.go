package browser

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"net/url"
	"strings"

	"github.com/saku0512/growse/internal/css"
	"github.com/saku0512/growse/internal/dom"
)

type stylesheetSource struct {
	inline string
	href   string
}

func (b *Browser) loadStyles(ctx context.Context, client ResourceLoader, pageURL *url.URL, document *dom.Document) (*css.Stylesheet, error) {
	combined := &css.Stylesheet{}
	for _, source := range collectStylesheets(document.Root) {
		var content []byte
		if source.inline != "" {
			content = []byte(source.inline)
		} else {
			stylesheetURL, err := pageURL.Parse(source.href)
			if err != nil || !sameOrigin(pageURL, stylesheetURL) {
				continue
			}
			response, err := client.Get(ctx, stylesheetURL)
			if err != nil {
				continue
			}
			if response.ContentType != "" {
				mediaType, _, err := mime.ParseMediaType(response.ContentType)
				if err != nil || mediaType != "text/css" {
					continue
				}
			}
			content = response.Body
		}

		parsed, err := css.Parse(bytes.NewReader(content))
		if err != nil {
			return nil, fmt.Errorf("parse stylesheet: %w", err)
		}
		combined.Append(parsed)
	}
	return combined, nil
}

func collectStylesheets(root *dom.Node) []stylesheetSource {
	var result []stylesheetSource
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Type == dom.NodeElement {
			switch node.TagName {
			case "style":
				if content := node.TextContent(); strings.TrimSpace(content) != "" {
					result = append(result, stylesheetSource{inline: content})
				}
			case "link":
				rel, _ := node.Attribute("rel")
				href, _ := node.Attribute("href")
				if containsASCIIToken(rel, "stylesheet") && strings.TrimSpace(href) != "" {
					result = append(result, stylesheetSource{href: strings.TrimSpace(href)})
				}
			}
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(root)
	return result
}

func containsASCIIToken(value, target string) bool {
	for _, token := range strings.Fields(value) {
		if strings.EqualFold(token, target) {
			return true
		}
	}
	return false
}

func sameOrigin(left, right *url.URL) bool {
	return left != nil && right != nil &&
		strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Host, right.Host)
}
