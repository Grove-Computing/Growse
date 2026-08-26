package javascript

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/evanw/esbuild/pkg/api"
)

const moduleNamespace = "growse-module"

type moduleGraph struct {
	ctx         context.Context
	environment runtimemodel.Environment
	credentials network.CredentialsMode
	rootURL     string
	rootSource  string

	mu        sync.Mutex
	sources   map[string]string
	referrers map[string]string
}

func bundleModule(ctx context.Context, script runtimemodel.Script, environment runtimemodel.Environment) (string, error) {
	if script.SourceURL == nil {
		return "", errors.New("module requires a source URL")
	}
	rootURL, err := normalizeModuleURL(script.SourceURL)
	if err != nil {
		return "", err
	}
	credentials := network.CredentialsSameOrigin
	if script.CrossOrigin == "use-credentials" {
		credentials = network.CredentialsInclude
	}
	graph := &moduleGraph{
		ctx: ctx, environment: environment, credentials: credentials,
		rootURL: rootURL, rootSource: script.Source,
		sources: make(map[string]string), referrers: make(map[string]string),
	}
	result := api.Build(api.BuildOptions{
		EntryPoints: []string{rootURL}, Bundle: true, Write: false,
		Platform: api.PlatformBrowser, Format: api.FormatIIFE, Target: api.ES2020,
		Charset: api.CharsetUTF8, LegalComments: api.LegalCommentsNone,
		TreeShaking: api.TreeShakingFalse, LogLevel: api.LogLevelSilent,
		Banner:  map[string]string{"js": `"use strict";`},
		Plugins: []api.Plugin{{Name: moduleNamespace, Setup: graph.setup}},
	})
	if len(result.Errors) != 0 {
		messages := api.FormatMessages(result.Errors, api.FormatMessagesOptions{Kind: api.ErrorMessage, Color: false})
		return "", errors.New(strings.TrimSpace(strings.Join(messages, "\n")))
	}
	if len(result.OutputFiles) != 1 {
		return "", fmt.Errorf("module graph produced %d output files", len(result.OutputFiles))
	}
	return string(result.OutputFiles[0].Contents), nil
}

func (graph *moduleGraph) setup(build api.PluginBuild) {
	build.OnResolve(api.OnResolveOptions{Filter: ".*"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
		if err := graph.ctx.Err(); err != nil {
			return api.OnResolveResult{}, err
		}
		if args.Kind == api.ResolveJSDynamicImport {
			return api.OnResolveResult{}, errors.New("dynamic import() is not enabled for this module graph")
		}
		if args.Kind == api.ResolveEntryPoint {
			return api.OnResolveResult{Path: graph.rootURL, Namespace: moduleNamespace}, nil
		}
		graph.mu.Lock()
		referrer := graph.referrers[args.Importer]
		graph.mu.Unlock()
		if referrer == "" {
			referrer = args.Importer
		}
		resolved, err := resolveModuleSpecifier(referrer, args.Path)
		if err != nil {
			return api.OnResolveResult{}, err
		}
		return api.OnResolveResult{Path: resolved, Namespace: moduleNamespace}, nil
	})
	build.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: moduleNamespace}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
		source, referrer, err := graph.load(args.Path)
		if err != nil {
			return api.OnLoadResult{}, err
		}
		graph.mu.Lock()
		graph.referrers[args.Path] = referrer
		graph.mu.Unlock()
		return api.OnLoadResult{Contents: &source, Loader: api.LoaderJS}, nil
	})
}

func (graph *moduleGraph) load(moduleURL string) (string, string, error) {
	if err := graph.ctx.Err(); err != nil {
		return "", "", err
	}
	graph.mu.Lock()
	if source, ok := graph.sources[moduleURL]; ok {
		referrer := graph.referrers[moduleURL]
		graph.mu.Unlock()
		return source, referrer, nil
	}
	if moduleURL == graph.rootURL {
		graph.sources[moduleURL] = graph.rootSource
		graph.referrers[moduleURL] = moduleURL
		graph.mu.Unlock()
		return graph.rootSource, moduleURL, nil
	}
	graph.mu.Unlock()

	target, err := url.Parse(moduleURL)
	if err != nil {
		return "", "", fmt.Errorf("parse module URL: %w", err)
	}
	if graph.environment.Fetch == nil {
		return "", "", errors.New("module fetch broker is unavailable")
	}
	response, err := graph.environment.Fetch(graph.ctx, &network.Request{
		Method: http.MethodGet, URL: target, SiteURL: graph.environment.BaseURL,
		Kind: network.RequestModule, Engine: string(runtimemodel.EngineJavaScript),
		Credentials: graph.credentials, CORS: true,
	})
	if err != nil {
		return "", "", fmt.Errorf("fetch module %s: %w", network.RedactedURL(target), err)
	}
	if response == nil {
		return "", "", fmt.Errorf("fetch module %s: empty response", network.RedactedURL(target))
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", "", fmt.Errorf("module %s returned HTTP status %d", network.RedactedURL(target), response.StatusCode)
	}
	if len(response.Body) > maxModuleBytes {
		return "", "", fmt.Errorf("module %s exceeds %d bytes", network.RedactedURL(target), maxModuleBytes)
	}
	if !isModuleContentType(response.ContentType) {
		return "", "", fmt.Errorf("module %s has unsupported Content-Type %q", network.RedactedURL(target), response.ContentType)
	}
	finalURL := response.URL
	if finalURL == nil {
		finalURL = target
	}
	final, err := normalizeModuleURL(finalURL)
	if err != nil {
		return "", "", fmt.Errorf("redirected module URL: %w", err)
	}
	if mixedModuleContent(graph.environment.BaseURL, finalURL) {
		return "", "", fmt.Errorf("block mixed-content module %s", network.RedactedURL(finalURL))
	}
	source := string(response.Body)
	graph.mu.Lock()
	graph.sources[moduleURL] = source
	graph.referrers[moduleURL] = final
	if _, exists := graph.sources[final]; !exists {
		graph.sources[final] = source
		graph.referrers[final] = final
	}
	graph.mu.Unlock()
	return source, final, nil
}

func resolveModuleSpecifier(referrer, specifier string) (string, error) {
	base, err := url.Parse(referrer)
	if err != nil {
		return "", fmt.Errorf("parse module referrer: %w", err)
	}
	specifier = strings.TrimSpace(specifier)
	if specifier == "" {
		return "", errors.New("module specifier is empty")
	}
	parsed, err := url.Parse(specifier)
	if err != nil {
		return "", fmt.Errorf("parse module specifier %q: %w", specifier, err)
	}
	if !parsed.IsAbs() && !strings.HasPrefix(specifier, "/") && !strings.HasPrefix(specifier, "./") &&
		!strings.HasPrefix(specifier, "../") && !strings.HasPrefix(specifier, "?") && !strings.HasPrefix(specifier, "#") {
		return "", fmt.Errorf("bare module specifier %q requires an import map", specifier)
	}
	return normalizeModuleURL(base.ResolveReference(parsed))
}

func normalizeModuleURL(target *url.URL) (string, error) {
	if target == nil || target.User != nil || target.Hostname() == "" ||
		(!strings.EqualFold(target.Scheme, "http") && !strings.EqualFold(target.Scheme, "https")) {
		return "", fmt.Errorf("module URL must be HTTP(S) without userinfo: %s", network.RedactedURL(target))
	}
	copy := *target
	copy.User = nil
	copy.Fragment = ""
	copy.Scheme = strings.ToLower(copy.Scheme)
	copy.Host = strings.ToLower(copy.Host)
	return copy.String(), nil
}

func isModuleContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
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

func mixedModuleContent(pageURL, moduleURL *url.URL) bool {
	return pageURL != nil && moduleURL != nil && strings.EqualFold(pageURL.Scheme, "https") && strings.EqualFold(moduleURL.Scheme, "http")
}
