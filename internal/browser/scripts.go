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
	kind        runtimemodel.ScriptKind
	inline      bool
	source      string
	src         string
	integrity   string
	crossOrigin string
	hasCORS     bool
	schedule    runtimemodel.ScriptSchedule
}

func loadScriptsForEngine(ctx context.Context, client ResourceLoader, pageURL *url.URL, document *dom.Document, engine runtimemodel.Engine) ([]Script, []string) {
	if client == nil || pageURL == nil || document == nil || document.Root == nil {
		return nil, nil
	}
	engine = runtimemodel.NormalizeEngine(engine)
	if !engine.Valid() {
		return nil, []string{fmt.Sprintf("unsupported script engine %q", engine)}
	}
	candidates := collectScriptsForEngine(document.Root, engine)
	var loadErrors []string
	if len(candidates) > maxScriptsPerEngine {
		loadErrors = append(loadErrors, fmt.Sprintf("%s script count exceeds %d", engine, maxScriptsPerEngine))
		candidates = candidates[:maxScriptsPerEngine]
	}
	type loadResult struct {
		script Script
		size   int
		err    error
	}
	results := make([]loadResult, len(candidates))
	type asyncResult struct {
		index int
		loadResult
	}
	asyncResults := make(chan asyncResult, len(candidates))
	asyncCount := 0
	for index, candidate := range candidates {
		if engine == runtimemodel.EngineJavaScript && !candidate.inline && candidate.schedule == runtimemodel.ScriptAsync {
			asyncCount++
			go func(index int, candidate scriptSource) {
				script, size, err := loadScriptCandidate(ctx, client, pageURL, engine, candidate, index)
				asyncResults <- asyncResult{index: index, loadResult: loadResult{script: script, size: size, err: err}}
			}(index, candidate)
			continue
		}
		script, size, err := loadScriptCandidate(ctx, client, pageURL, engine, candidate, index)
		results[index] = loadResult{script: script, size: size, err: err}
	}
	for fetchOrder := 1; fetchOrder <= asyncCount; fetchOrder++ {
		loaded := <-asyncResults
		loaded.script.FetchOrder = fetchOrder
		results[loaded.index] = loaded.loadResult
		results[loaded.index].script.FetchOrder = fetchOrder
	}
	var scripts []Script
	totalBytes := 0
	for _, result := range results {
		if result.err != nil {
			loadErrors = append(loadErrors, result.err.Error())
			continue
		}
		if totalBytes+result.size > maxScriptTotalBytes {
			loadErrors = append(loadErrors, fmt.Sprintf("%s script total exceeds %d bytes", engine, maxScriptTotalBytes))
			continue
		}
		totalBytes += result.size
		scripts = append(scripts, result.script)
	}
	return scripts, loadErrors
}

func loadScriptCandidate(ctx context.Context, client ResourceLoader, pageURL *url.URL, engine runtimemodel.Engine, candidate scriptSource, documentOrder int) (Script, int, error) {
	script := Script{Engine: engine, Kind: candidate.kind, Schedule: candidate.schedule, DocumentOrder: documentOrder}
	if candidate.inline {
		if len(candidate.source) > maxScriptBytes {
			return Script{}, 0, fmt.Errorf("inline %s script exceeds %d bytes", engine, maxScriptBytes)
		}
		script.SourceURL, script.Source, script.Inline = cloneURL(pageURL), candidate.source, true
		return script, len(candidate.source), nil
	}
	baseURL := cloneURL(pageURL)
	baseURL.User = nil
	scriptURL, err := baseURL.Parse(candidate.src)
	if err != nil {
		return Script{}, 0, fmt.Errorf("resolve %s script %q: %v", engine, candidate.src, err)
	}
	if !isHTTPURL(scriptURL) {
		return Script{}, 0, fmt.Errorf("block %s script from invalid URL %s", engine, network.RedactedURL(scriptURL))
	}
	if isMixedContent(pageURL, scriptURL) {
		return Script{}, 0, fmt.Errorf("block mixed-content %s script %s", engine, network.RedactedURL(scriptURL))
	}
	if engine == runtimemodel.EngineGo && (!IsTrustedOrigin(scriptURL) || !network.SameOrigin(pageURL, scriptURL)) {
		return Script{}, 0, fmt.Errorf("block %s script from untrusted or cross-origin URL %s", engine, network.RedactedURL(scriptURL))
	}
	credentials := scriptCredentials(candidate)
	if candidate.kind == runtimemodel.ScriptModule && candidate.crossOrigin != "use-credentials" {
		credentials = network.CredentialsSameOrigin
	}
	if engine == runtimemodel.EngineJavaScript && candidate.kind != runtimemodel.ScriptModule && candidate.integrity != "" && !network.SameOrigin(pageURL, scriptURL) && !candidate.hasCORS {
		return Script{}, 0, fmt.Errorf("cross-origin JavaScript integrity requires crossorigin for %s", network.RedactedURL(scriptURL))
	}
	cors := candidate.hasCORS || candidate.kind == runtimemodel.ScriptModule
	requestKind := network.RequestScript
	if candidate.kind == runtimemodel.ScriptModule {
		requestKind = network.RequestModule
	}
	response, err := loadScriptResource(ctx, client, scriptURL, requestKind, cors, credentials)
	if err != nil {
		return Script{}, 0, fmt.Errorf("load %s script %s: %v", engine, network.RedactedURL(scriptURL), err)
	}
	if response == nil {
		return Script{}, 0, fmt.Errorf("load %s script %s: empty response", engine, network.RedactedURL(scriptURL))
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return Script{}, 0, fmt.Errorf("%s script %s returned HTTP status %d", engine, network.RedactedURL(scriptURL), response.StatusCode)
	}
	if len(response.Body) > maxScriptBytes {
		return Script{}, 0, fmt.Errorf("%s script %s exceeds %d bytes", engine, network.RedactedURL(scriptURL), maxScriptBytes)
	}
	if !isScriptContentType(engine, response.ContentType) {
		return Script{}, 0, fmt.Errorf("%s script %s has unsupported Content-Type %q", engine, network.RedactedURL(scriptURL), response.ContentType)
	}
	finalURL := response.URL
	if finalURL == nil {
		finalURL = scriptURL
	}
	if !isHTTPURL(finalURL) || isMixedContent(pageURL, finalURL) {
		return Script{}, 0, fmt.Errorf("block redirected %s script from invalid or mixed-content URL %s", engine, network.RedactedURL(finalURL))
	}
	if engine == runtimemodel.EngineGo && (!IsTrustedOrigin(finalURL) || !network.SameOrigin(pageURL, finalURL)) {
		return Script{}, 0, fmt.Errorf("block redirected %s script from untrusted or cross-origin URL %s", engine, network.RedactedURL(finalURL))
	}
	if engine == runtimemodel.EngineJavaScript {
		if candidate.kind != runtimemodel.ScriptModule && candidate.integrity != "" && !network.SameOrigin(pageURL, finalURL) && !candidate.hasCORS {
			return Script{}, 0, fmt.Errorf("redirected cross-origin JavaScript integrity requires crossorigin for %s", network.RedactedURL(finalURL))
		}
		if err := verifyScriptIntegrity(response.Body, candidate.integrity); err != nil {
			return Script{}, 0, fmt.Errorf("JavaScript integrity check failed for %s: %v", network.RedactedURL(finalURL), err)
		}
	}
	script = Script{
		Engine: engine, Kind: candidate.kind, SourceURL: cloneURL(finalURL), Source: string(response.Body),
		Integrity: candidate.integrity, CrossOrigin: candidate.crossOrigin, Schedule: candidate.schedule, DocumentOrder: documentOrder,
	}
	return script, len(response.Body), nil
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
			scriptEngine, kind := classifyScript(typeValue, hasType)
			if scriptEngine == engine {
				src, _ := node.Attribute("src")
				src = strings.TrimSpace(src)
				if src != "" {
					schedule := runtimemodel.ScriptParserBlocking
					if engine == runtimemodel.EngineJavaScript {
						if _, async := node.Attribute("async"); async {
							schedule = runtimemodel.ScriptAsync
						} else if kind == runtimemodel.ScriptModule {
							schedule = runtimemodel.ScriptDefer
						} else if _, deferred := node.Attribute("defer"); deferred {
							schedule = runtimemodel.ScriptDefer
						}
					}
					integrity, _ := node.Attribute("integrity")
					crossOrigin, hasCORS := node.Attribute("crossorigin")
					result = append(result, scriptSource{
						engine: engine, kind: kind, src: src, integrity: strings.TrimSpace(integrity),
						crossOrigin: normalizeCrossOrigin(crossOrigin, hasCORS), hasCORS: hasCORS, schedule: schedule,
					})
				} else {
					schedule := runtimemodel.ScriptParserBlocking
					if kind == runtimemodel.ScriptModule {
						schedule = runtimemodel.ScriptDefer
						if _, async := node.Attribute("async"); async {
							schedule = runtimemodel.ScriptAsync
						}
					}
					result = append(result, scriptSource{engine: engine, kind: kind, inline: true, source: node.TextContent(), schedule: schedule})
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

func classifyScript(value string, hasType bool) (runtimemodel.Engine, runtimemodel.ScriptKind) {
	value = strings.TrimSpace(value)
	if !hasType || value == "" {
		return runtimemodel.EngineJavaScript, runtimemodel.ScriptClassic
	}
	if strings.EqualFold(value, "module") {
		return runtimemodel.EngineJavaScript, runtimemodel.ScriptModule
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return "", ""
	}
	switch strings.ToLower(mediaType) {
	case "text/go":
		return runtimemodel.EngineGo, runtimemodel.ScriptClassic
	case "text/javascript", "application/javascript":
		return runtimemodel.EngineJavaScript, runtimemodel.ScriptClassic
	default:
		return "", ""
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

func loadScriptResource(ctx context.Context, client ResourceLoader, target *url.URL, kind network.RequestKind, cors bool, credentials network.CredentialsMode) (*network.Response, error) {
	if loader, ok := client.(requestLoader); ok {
		return loader.Do(ctx, &network.Request{
			Method: http.MethodGet, URL: target, Kind: kind, CORS: cors, Credentials: credentials,
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
