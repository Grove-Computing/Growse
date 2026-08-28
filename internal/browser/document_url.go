package browser

import (
	"net/url"
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
)

// documentBaseURL returns the fallback document URL or the first valid base
// href in tree order. Credentials are never retained in a resource base.
func documentBaseURL(document *dom.Document, documentURL *url.URL) *url.URL {
	fallback := cloneURL(documentURL)
	if fallback != nil {
		fallback.User = nil
	}
	if document == nil || document.Root == nil || documentURL == nil {
		return fallback
	}
	var result *url.URL
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil || node.Type == dom.NodeDocumentFragment || result != nil {
			return
		}
		if node.Type == dom.NodeElement && node.TagName == "base" {
			href, present := node.Attribute("href")
			if present {
				reference, err := url.Parse(strings.TrimSpace(href))
				if err == nil {
					result = documentURL.ResolveReference(reference)
					result.User = nil
					return
				}
			}
		}
		for _, child := range node.Children {
			walk(child)
			if result != nil {
				return
			}
		}
	}
	walk(document.Root)
	if result != nil {
		return result
	}
	return fallback
}

func pageBaseURL(page *Page) *url.URL {
	if page == nil {
		return nil
	}
	if page.BaseURL != nil {
		return page.BaseURL
	}
	return page.URL
}
