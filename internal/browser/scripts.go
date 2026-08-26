package browser

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
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
	engine      runtimemodel.Engine
	inline      bool
	source      string
	src         string
	integrity   string
	crossOrigin string
	hasCORS     bool
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

		baseURL := cloneURL(pageURL)
		baseURL.User = nil
		scriptURL, err := baseURL.Parse(candidate.src)
		if err != nil {
			loadErrors = append(loadErrors, fmt.Sprintf("resolve %s script %q: %v", engine, candidate.src, err))
			continue
		}
		if !isHTTPURL(scriptURL) {
			loadErrors = append(loadErrors, fmt.Sprintf("block %s script from invalid URL %s", engine, network.RedactedURL(scriptURL)))
			continue
		}
		if isMixedContent(pageURL, scriptURL) {
			loadErrors = append(loadErrors, fmt.Sprintf("block mixed-content %s script %s", engine, network.RedactedURL(scriptURL)))
			continue
		}
		if engine == runtimemodel.EngineGo && (!IsTrustedOrigin(scriptURL) || !network.SameOrigin(pageURL, scriptURL)) {
			loadErrors = append(loadErrors, fmt.Sprintf("block %s script from untrusted or cross-origin URL %s", engine, network.RedactedURL(scriptURL)))
			continue
		}
		credentials := scriptCredentials(candidate)
		if engine == runtimemodel.EngineJavaScript && candidate.integrity != "" && !network.SameOrigin(pageURL, scriptURL) && !candidate.hasCORS {
			loadErrors = append(loadErrors, fmt.Sprintf("cross-origin JavaScript integrity requires crossorigin for %s", network.RedactedURL(scriptURL)))
			continue
		}
		response, err := loadScriptResource(ctx, client, scriptURL, candidate.hasCORS, credentials)
		if err != nil {
			loadErrors = append(loadErrors, fmt.Sprintf("load %s script %s: %v", engine, network.RedactedURL(scriptURL), err))
			continue
		}
		if response == nil {
			loadErrors = append(loadErrors, fmt.Sprintf("load %s script %s: empty response", engine, network.RedactedURL(scriptURL)))
			continue
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			loadErrors = append(loadErrors, fmt.Sprintf("%s script %s returned HTTP status %d", engine, network.RedactedURL(scriptURL), response.StatusCode))
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
		if !isHTTPURL(finalURL) || isMixedContent(pageURL, finalURL) {
			loadErrors = append(loadErrors, fmt.Sprintf("block redirected %s script from invalid or mixed-content URL %s", engine, network.RedactedURL(finalURL)))
			continue
		}
		if engine == runtimemodel.EngineGo && (!IsTrustedOrigin(finalURL) || !network.SameOrigin(pageURL, finalURL)) {
			loadErrors = append(loadErrors, fmt.Sprintf("block redirected %s script from untrusted or cross-origin URL %s", engine, network.RedactedURL(finalURL)))
			continue
		}
		if engine == runtimemodel.EngineJavaScript {
			if candidate.integrity != "" && !network.SameOrigin(pageURL, finalURL) && !candidate.hasCORS {
				loadErrors = append(loadErrors, fmt.Sprintf("redirected cross-origin JavaScript integrity requires crossorigin for %s", network.RedactedURL(finalURL)))
				continue
			}
			if err := verifyScriptIntegrity(response.Body, candidate.integrity); err != nil {
				loadErrors = append(loadErrors, fmt.Sprintf("JavaScript integrity check failed for %s: %v", network.RedactedURL(finalURL), err))
				continue
			}
		}
		totalBytes += len(response.Body)
		scripts = append(scripts, Script{
			Engine: engine, SourceURL: cloneURL(finalURL), Source: string(response.Body),
			Integrity: candidate.integrity, CrossOrigin: candidate.crossOrigin,
		})
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
					integrity, _ := node.Attribute("integrity")
					crossOrigin, hasCORS := node.Attribute("crossorigin")
					result = append(result, scriptSource{
						engine: engine, src: src, integrity: strings.TrimSpace(integrity),
						crossOrigin: normalizeCrossOrigin(crossOrigin, hasCORS), hasCORS: hasCORS,
					})
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
		return false
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return false
	}
	switch strings.ToLower(mediaType) {
	case "text/javascript", "application/javascript", "application/ecmascript", "text/ecmascript":
		return true
	default:
		return false
	}
}

func isHTTPURL(value *url.URL) bool {
	return value != nil && value.User == nil && value.Hostname() != "" && (strings.EqualFold(value.Scheme, "http") || strings.EqualFold(value.Scheme, "https"))
}

func isMixedContent(pageURL, resourceURL *url.URL) bool {
	return pageURL != nil && resourceURL != nil && strings.EqualFold(pageURL.Scheme, "https") && strings.EqualFold(resourceURL.Scheme, "http")
}

func normalizeCrossOrigin(value string, present bool) string {
	if !present {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(value), "use-credentials") {
		return "use-credentials"
	}
	return "anonymous"
}

func scriptCredentials(source scriptSource) network.CredentialsMode {
	if source.crossOrigin == "use-credentials" {
		return network.CredentialsInclude
	}
	if source.hasCORS {
		return network.CredentialsSameOrigin
	}
	return network.CredentialsInclude
}

func loadScriptResource(ctx context.Context, client ResourceLoader, target *url.URL, cors bool, credentials network.CredentialsMode) (*network.Response, error) {
	if loader, ok := client.(requestLoader); ok {
		return loader.Do(ctx, &network.Request{
			Method: http.MethodGet, URL: target, Kind: network.RequestScript, CORS: cors, Credentials: credentials,
		})
	}
	return client.Get(ctx, target)
}

func verifyScriptIntegrity(body []byte, metadata string) error {
	metadata = strings.TrimSpace(metadata)
	if metadata == "" {
		return nil
	}
	type candidate struct {
		strength int
		digest   []byte
	}
	var candidates []candidate
	strongest := 0
	for _, token := range strings.Fields(metadata) {
		token, _, _ = strings.Cut(token, "?")
		algorithm, encoded, found := strings.Cut(token, "-")
		if !found {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			continue
		}
		strength := 0
		switch strings.ToLower(algorithm) {
		case "sha256":
			strength = 1
		case "sha384":
			strength = 2
		case "sha512":
			strength = 3
		}
		if strength != 0 {
			candidates = append(candidates, candidate{strength: strength, digest: decoded})
			if strength > strongest {
				strongest = strength
			}
		}
	}
	if strongest == 0 {
		return fmt.Errorf("no supported integrity digest")
	}
	var actual []byte
	switch strongest {
	case 1:
		digest := sha256.Sum256(body)
		actual = digest[:]
	case 2:
		digest := sha512.Sum384(body)
		actual = digest[:]
	case 3:
		digest := sha512.Sum512(body)
		actual = digest[:]
	}
	for _, expected := range candidates {
		if expected.strength == strongest && len(expected.digest) == len(actual) && subtle.ConstantTimeCompare(expected.digest, actual) == 1 {
			return nil
		}
	}
	return fmt.Errorf("digest mismatch")
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
