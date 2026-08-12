package browser

import (
	"context"
	"fmt"
	"mime"
	"net/url"
	"strings"

	"github.com/saku0512/growse/internal/dom"
)

const maxScriptBytes = 2 << 20

type scriptSource struct {
	inline bool
	source string
	src    string
}

func loadScripts(ctx context.Context, client ResourceLoader, pageURL *url.URL, document *dom.Document) ([]Script, []string) {
	if client == nil || pageURL == nil || document == nil || document.Root == nil {
		return nil, nil
	}
	var scripts []Script
	var loadErrors []string
	for _, candidate := range collectScripts(document.Root) {
		if candidate.inline {
			if len(candidate.source) > maxScriptBytes {
				loadErrors = append(loadErrors, fmt.Sprintf("inline Go script exceeds %d bytes", maxScriptBytes))
				continue
			}
			scripts = append(scripts, Script{SourceURL: cloneURL(pageURL), Source: candidate.source, Inline: true})
			continue
		}

		scriptURL, err := pageURL.Parse(candidate.src)
		if err != nil {
			loadErrors = append(loadErrors, fmt.Sprintf("resolve Go script %q: %v", candidate.src, err))
			continue
		}
		response, err := client.Get(ctx, scriptURL)
		if err != nil {
			loadErrors = append(loadErrors, fmt.Sprintf("load Go script %s: %v", scriptURL.Redacted(), err))
			continue
		}
		if len(response.Body) > maxScriptBytes {
			loadErrors = append(loadErrors, fmt.Sprintf("Go script %s exceeds %d bytes", scriptURL.Redacted(), maxScriptBytes))
			continue
		}
		if !isGoContentType(response.ContentType) {
			loadErrors = append(loadErrors, fmt.Sprintf("Go script %s has unsupported Content-Type %q", scriptURL.Redacted(), response.ContentType))
			continue
		}
		finalURL := response.URL
		if finalURL == nil {
			finalURL = scriptURL
		}
		scripts = append(scripts, Script{SourceURL: cloneURL(finalURL), Source: string(response.Body)})
	}
	return scripts, loadErrors
}

func collectScripts(root *dom.Node) []scriptSource {
	var result []scriptSource
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Type == dom.NodeElement && node.TagName == "script" {
			typeValue, _ := node.Attribute("type")
			if isGoScriptType(typeValue) {
				src, _ := node.Attribute("src")
				src = strings.TrimSpace(src)
				if src != "" {
					result = append(result, scriptSource{src: src})
				} else {
					result = append(result, scriptSource{inline: true, source: node.TextContent()})
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

func isGoScriptType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	return err == nil && strings.EqualFold(mediaType, "text/go")
}

func isGoContentType(value string) bool {
	if strings.TrimSpace(value) == "" {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return false
	}
	switch strings.ToLower(mediaType) {
	case "text/go", "text/x-go", "text/plain", "application/x-go":
		return true
	default:
		return false
	}
}
