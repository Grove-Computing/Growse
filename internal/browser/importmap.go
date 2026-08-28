package browser

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/network"
)

const (
	maxImportMapBytes    = 256 << 10
	maxImportMapMappings = 1_024
)

type importMapDocument struct {
	Imports map[string]json.RawMessage `json:"imports"`
}

func loadImportMap(document *dom.Document, documentURL *url.URL) (map[string]string, []string) {
	if document == nil || document.Root == nil || documentURL == nil {
		return nil, nil
	}
	var candidates []*dom.Node
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil || node.Type == dom.NodeDocumentFragment {
			return
		}
		if node.Type == dom.NodeElement && node.TagName == "script" {
			value, present := node.Attribute("type")
			if present && strings.EqualFold(strings.TrimSpace(value), "importmap") {
				candidates = append(candidates, node)
			}
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(document.Root)

	var loadErrors []string
	for _, candidate := range candidates {
		if source, present := candidate.Attribute("src"); present && strings.TrimSpace(source) != "" {
			loadErrors = append(loadErrors, "ignore external import map: src is not supported")
			continue
		}
		source := candidate.TextContent()
		if len(source) > maxImportMapBytes {
			loadErrors = append(loadErrors, fmt.Sprintf("import map exceeds %d bytes", maxImportMapBytes))
			continue
		}
		var parsed importMapDocument
		if err := json.Unmarshal([]byte(source), &parsed); err != nil {
			loadErrors = append(loadErrors, fmt.Sprintf("parse import map: %v", err))
			continue
		}
		if len(parsed.Imports) > maxImportMapMappings {
			loadErrors = append(loadErrors, fmt.Sprintf("import map exceeds %d mappings", maxImportMapMappings))
			continue
		}
		mappings := make(map[string]string, len(parsed.Imports))
		for key, rawTarget := range parsed.Imports {
			var target string
			if json.Unmarshal(rawTarget, &target) != nil || !isBareImportSpecifier(key) {
				continue
			}
			resolved, err := resolveImportMapTarget(documentURL, target)
			if err != nil || strings.HasSuffix(key, "/") != strings.HasSuffix(resolved, "/") {
				continue
			}
			mappings[key] = resolved
		}
		return mappings, loadErrors
	}
	return nil, loadErrors
}

func resolveImportMapTarget(base *url.URL, value string) (string, error) {
	target, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	resolved := base.ResolveReference(target)
	if !isHTTPURL(resolved) {
		return "", fmt.Errorf("import map target must be HTTP(S): %s", network.RedactedURL(resolved))
	}
	resolved.User = nil
	resolved.Fragment = ""
	return resolved.String(), nil
}

func isBareImportSpecifier(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") ||
		strings.HasPrefix(value, "?") || strings.HasPrefix(value, "#") {
		return false
	}
	parsed, err := url.Parse(value)
	return err == nil && !parsed.IsAbs()
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
