package browser

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

type stylesheetSource struct {
	inline string
	href   string
}

const (
	maxCSSImportDepth     = 8
	maxCSSStylesheetCount = 32
	maxCSSTotalBytes      = 16 << 20
)

type stylesheetLoadState struct {
	client     ResourceLoader
	origin     *url.URL
	activeURLs map[string]bool
	fetches    int
	totalBytes int
}

func (b *Browser) loadStylesWithBase(ctx context.Context, client ResourceLoader, pageURL, baseURL *url.URL, document *dom.Document) (*css.Stylesheet, error) {
	return loadStylesWithBase(ctx, client, pageURL, baseURL, document)
}

func loadStylesWithBase(ctx context.Context, client ResourceLoader, pageURL, baseURL *url.URL, document *dom.Document) (*css.Stylesheet, error) {
	combined := &css.Stylesheet{}
	state := &stylesheetLoadState{client: client, origin: pageURL, activeURLs: make(map[string]bool)}
	for _, source := range collectStylesheets(document.Root) {
		if source.inline != "" {
			content := []byte(source.inline)
			if !state.consumeBytes(len(content)) {
				continue
			}
			parsed, err := state.loadContent(ctx, content, baseURL, 0)
			if err != nil {
				return nil, fmt.Errorf("parse stylesheet: %w", err)
			}
			combined.Append(parsed)
			continue
		}
		stylesheetURL, err := baseURL.Parse(source.href)
		if err != nil {
			continue
		}
		parsed, err := state.loadExternal(ctx, stylesheetURL, 0)
		if err != nil {
			continue
		}
		combined.Append(parsed)
	}
	return combined, nil
}

func (state *stylesheetLoadState) loadContent(ctx context.Context, content []byte, baseURL *url.URL, depth int) (*css.Stylesheet, error) {
	parsed, err := css.Parse(bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	css.ResolveResourceURLs(parsed, baseURL)
	combined := &css.Stylesheet{}
	for _, importRule := range parsed.Imports {
		importURL, err := baseURL.Parse(importRule.URL)
		if err != nil {
			continue
		}
		imported, err := state.loadExternal(ctx, importURL, depth+1)
		if err != nil {
			continue
		}
		if len(importRule.Media) != 0 {
			for index := range imported.Rules {
				imported.Rules[index].Media = append(
					[][]css.MediaQuery{append([]css.MediaQuery(nil), importRule.Media...)},
					imported.Rules[index].Media...,
				)
			}
		}
		if importRule.Layered {
			imported.NestUnderLayer(importRule.Layer)
		}
		combined.Append(imported)
	}
	combined.Append(parsed)
	return combined, nil
}

func (state *stylesheetLoadState) loadExternal(ctx context.Context, requestedURL *url.URL, depth int) (*css.Stylesheet, error) {
	empty := &css.Stylesheet{}
	if requestedURL == nil || depth > maxCSSImportDepth || state.fetches >= maxCSSStylesheetCount ||
		!isHTTPURL(requestedURL) || requestedURL.User != nil || isMixedContent(state.origin, requestedURL) {
		return empty, nil
	}
	requested := *requestedURL
	requested.Fragment = ""
	key := requested.String()
	if state.activeURLs[key] {
		return empty, nil
	}
	state.activeURLs[key] = true
	defer delete(state.activeURLs, key)
	state.fetches++
	response, err := state.client.Get(ctx, &requested)
	if err != nil || response == nil {
		return empty, err
	}
	finalURL := response.URL
	if finalURL == nil {
		finalURL = &requested
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices ||
		!isHTTPURL(finalURL) || finalURL.User != nil || isMixedContent(state.origin, finalURL) ||
		!isCSSContentType(response.ContentType) || !state.consumeBytes(len(response.Body)) {
		return empty, nil
	}
	finalKey := finalURL.String()
	if finalKey != key {
		if state.activeURLs[finalKey] {
			return empty, nil
		}
		state.activeURLs[finalKey] = true
		defer delete(state.activeURLs, finalKey)
	}
	return state.loadContent(ctx, response.Body, finalURL, depth)
}

func (state *stylesheetLoadState) consumeBytes(size int) bool {
	if size < 0 || size > maxCSSTotalBytes-state.totalBytes {
		return false
	}
	state.totalBytes += size
	return true
}

func isCSSContentType(contentType string) bool {
	if contentType == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && mediaType == "text/css"
}

func collectStylesheets(root *dom.Node) []stylesheetSource {
	var result []stylesheetSource
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil || node.Type == dom.NodeDocumentFragment {
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
