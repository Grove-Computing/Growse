package browser

import (
	"context"
	"fmt"
	"mime"
	"net/url"
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

const (
	maxScriptBytes      = 2 << 20
	maxScriptsPerEngine = 64
	maxScriptTotalBytes = 8 << 20
)

type scriptSource struct {
	engine runtimemodel.Engine
	inline bool
	source string
	src    string
}

func loadScripts(ctx context.Context, client ResourceLoader, pageURL *url.URL, document *dom.Document) ([]Script, []string) {
	return loadScriptsForEngine(ctx, client, pageURL, document, runtimemodel.EngineGo)
}

func loadScriptsForEngine(ctx context.Context, client ResourceLoader, pageURL *url.URL, document *dom.Document, engine runtimemodel.Engine) ([]Script, []string) {
	if client == nil || pageURL == nil || document == nil || document.Root == nil {
		return nil, nil
	}
	engine = runtimemodel.NormalizeEngine(engine)
	if !engine.Valid() {
		return nil, []string{fmt.Sprintf("unsupported script engine %q", engine)}
	}
	var scripts []Script
	var loadErrors []string
	totalBytes := 0
	candidates := collectScriptsForEngine(document.Root, engine)
	if len(candidates) > maxScriptsPerEngine {
		loadErrors = append(loadErrors, fmt.Sprintf("%s script count exceeds %d", engine, maxScriptsPerEngine))
		candidates = candidates[:maxScriptsPerEngine]
	}
	for _, candidate := range candidates {
		if candidate.inline {
			if len(candidate.source) > maxScriptBytes {
				loadErrors = append(loadErrors, fmt.Sprintf("inline %s script exceeds %d bytes", engine, maxScriptBytes))
				continue
			}
			if totalBytes+len(candidate.source) > maxScriptTotalBytes {
				loadErrors = append(loadErrors, fmt.Sprintf("%s script total exceeds %d bytes", engine, maxScriptTotalBytes))
				continue
			}
			totalBytes += len(candidate.source)
			scripts = append(scripts, Script{Engine: engine, SourceURL: cloneURL(pageURL), Source: candidate.source, Inline: true})
			continue
		}

		scriptURL, err := pageURL.Parse(candidate.src)
		if err != nil {
			loadErrors = append(loadErrors, fmt.Sprintf("resolve %s script %q: %v", engine, candidate.src, err))
			continue
		}
		if !IsTrustedOrigin(scriptURL) || !network.SameOrigin(pageURL, scriptURL) {
			loadErrors = append(loadErrors, fmt.Sprintf("block %s script from untrusted or cross-origin URL %s", engine, network.RedactedURL(scriptURL)))
			continue
		}
		response, err := client.Get(ctx, scriptURL)
		if err != nil {
			loadErrors = append(loadErrors, fmt.Sprintf("load %s script %s: %v", engine, network.RedactedURL(scriptURL), err))
			continue
		}
		if len(response.Body) > maxScriptBytes {
			loadErrors = append(loadErrors, fmt.Sprintf("%s script %s exceeds %d bytes", engine, network.RedactedURL(scriptURL), maxScriptBytes))
			continue
		}
		if totalBytes+len(response.Body) > maxScriptTotalBytes {
			loadErrors = append(loadErrors, fmt.Sprintf("%s script total exceeds %d bytes", engine, maxScriptTotalBytes))
			continue
		}
		if !isScriptContentType(engine, response.ContentType) {
			loadErrors = append(loadErrors, fmt.Sprintf("%s script %s has unsupported Content-Type %q", engine, network.RedactedURL(scriptURL), response.ContentType))
			continue
		}
		finalURL := response.URL
		if finalURL == nil {
			finalURL = scriptURL
		}
		if !IsTrustedOrigin(finalURL) || !network.SameOrigin(pageURL, finalURL) {
			loadErrors = append(loadErrors, fmt.Sprintf("block redirected %s script from untrusted or cross-origin URL %s", engine, network.RedactedURL(finalURL)))
			continue
		}
		totalBytes += len(response.Body)
		scripts = append(scripts, Script{Engine: engine, SourceURL: cloneURL(finalURL), Source: string(response.Body)})
	}
	return scripts, loadErrors
}

func collectScripts(root *dom.Node) []scriptSource {
	return collectScriptsForEngine(root, runtimemodel.EngineGo)
}

func collectScriptsForEngine(root *dom.Node, engine runtimemodel.Engine) []scriptSource {
	engine = runtimemodel.NormalizeEngine(engine)
	var result []scriptSource
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Type == dom.NodeElement && node.TagName == "script" {
			typeValue, hasType := node.Attribute("type")
			if scriptEngine(typeValue, hasType) == engine {
				src, _ := node.Attribute("src")
				src = strings.TrimSpace(src)
				if src != "" {
					result = append(result, scriptSource{engine: engine, src: src})
				} else {
					result = append(result, scriptSource{engine: engine, inline: true, source: node.TextContent()})
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

func scriptEngine(value string, hasType bool) runtimemodel.Engine {
	value = strings.TrimSpace(value)
	if !hasType || value == "" {
		return runtimemodel.EngineJavaScript
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return ""
	}
	switch strings.ToLower(mediaType) {
	case "text/go":
		return runtimemodel.EngineGo
	case "text/javascript", "application/javascript":
		return runtimemodel.EngineJavaScript
	default:
		return ""
	}
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

func isJavaScriptContentType(value string) bool {
	if strings.TrimSpace(value) == "" {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return false
	}
	switch strings.ToLower(mediaType) {
	case "text/javascript", "application/javascript", "text/plain":
		return true
	default:
		return false
	}
}

func isScriptContentType(engine runtimemodel.Engine, value string) bool {
	switch runtimemodel.NormalizeEngine(engine) {
	case runtimemodel.EngineGo:
		return isGoContentType(value)
	case runtimemodel.EngineJavaScript:
		return isJavaScriptContentType(value)
	default:
		return false
	}
}
