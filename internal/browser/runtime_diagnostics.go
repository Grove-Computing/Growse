package browser

import (
	"net/url"
	"sort"
	"strings"

	"github.com/Grove-Computing/Growse/internal/devtools"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

// RuntimeDiagnostics returns bounded, secret-free Page, Frame, and Service Worker metadata.
func (page *Page) RuntimeDiagnostics() []devtools.RuntimeContext {
	if page == nil {
		return nil
	}
	contexts := []devtools.RuntimeContext{runtimeContextForPage("page", 0, 0, page.window.Self.Generation, page)}
	var appendFrames func(*Page)
	appendFrames = func(parent *Page) {
		for _, frame := range parent.Frames {
			if frame == nil || frame.Page == nil || frame.Closed {
				continue
			}
			context := runtimeContextForPage("frame", frame.ID, frame.ParentID, frame.Generation, frame.Page)
			if frame.LoadError != "" {
				context.ErrorCategories = appendCategory(context.ErrorCategories, "frame")
			}
			contexts = append(contexts, context)
			appendFrames(frame.Page)
		}
	}
	appendFrames(page)
	if page.serviceWorkers != nil && page.URL != nil {
		if registrations, err := page.serviceWorkers.GetRegistrations(page.URL); err == nil {
			for _, registration := range registrations {
				contexts = append(contexts, serviceWorkerRuntimeContext(registration))
			}
		}
	}
	return contexts
}

func runtimeContextForPage(kind string, id, parentID, browsingGeneration uint64, page *Page) devtools.RuntimeContext {
	context := devtools.RuntimeContext{
		Kind: kind, ID: id, ParentID: parentID, BrowsingGeneration: browsingGeneration,
		URL: redactedDiagnosticURL(page.URL), Engine: string(runtimemodel.NormalizeEngine(page.Engine)), State: runtimeDiagnosticState(page),
		Sandbox: devtools.RuntimeSandbox{
			Ready: page.Sandbox.Ready, ProcessBoundary: page.Sandbox.ProcessBoundary, BrokeredHostIO: page.Sandbox.BrokeredHostIO,
			Generation: page.Sandbox.Generation, ConstraintCount: len(page.Sandbox.Constraints), Failure: page.Sandbox.Failure != "",
		},
	}
	context.Diagnostics = compatibilityDiagnostics(page)
	for _, script := range page.Scripts {
		location := "inline"
		if !script.Inline {
			location = redactedDiagnosticURL(script.SourceURL)
		}
		context.Scripts = append(context.Scripts, devtools.RuntimeScript{
			Kind: string(script.Kind), Schedule: string(script.Schedule), Location: location,
		})
	}
	for _, message := range append(append([]string(nil), page.ScriptErrors...), page.RuntimeError) {
		context.ErrorCategories = appendCategory(context.ErrorCategories, runtimeErrorCategory(message))
	}
	return context
}

func serviceWorkerRuntimeContext(registration runtimemodel.ServiceWorkerRegistration) devtools.RuntimeContext {
	state := string(registration.Active)
	if state == "" {
		state = string(registration.Waiting)
	}
	if state == "" {
		state = string(registration.Installing)
	}
	return devtools.RuntimeContext{
		Kind: "service-worker", ID: registration.ID, BrowsingGeneration: registration.Generation,
		URL: redactedDiagnosticURLString(registration.ScriptURL), Engine: string(runtimemodel.EngineJavaScript), State: state,
		Scripts: []devtools.RuntimeScript{{Kind: "service-worker", Schedule: "worker", Location: redactedDiagnosticURLString(registration.ScriptURL)}},
		Sandbox: devtools.RuntimeSandbox{ProcessBoundary: true, BrokeredHostIO: true, Generation: registration.Generation},
	}
}

func runtimeDiagnosticState(page *Page) string {
	if page.RuntimeStarted {
		return "running"
	}
	if page.RuntimeError != "" {
		return "error"
	}
	if len(page.Scripts) != 0 {
		return "stopped"
	}
	return "idle"
}

func runtimeErrorCategory(message string) string {
	value := strings.ToLower(message)
	switch {
	case strings.Contains(value, "hydration") || strings.Contains(value, "hydrate"):
		return "hydration"
	case strings.Contains(value, "unsupported global") || strings.Contains(value, "unsupported web api"):
		return "unsupported-global"
	case strings.Contains(value, "event dispatch") || strings.Contains(value, "event listener"):
		return "event"
	case strings.Contains(value, "observer") || strings.Contains(value, "loop limit"):
		return "observer"
	case strings.Contains(value, "chunk"):
		return "chunk"
	case strings.Contains(value, "stale") || strings.Contains(value, "generation"):
		return "stale-generation"
	case strings.Contains(value, "host api") || strings.Contains(value, "host function"):
		return "host-api"
	case strings.Contains(value, "webassembly") || strings.Contains(value, "wasm"):
		return "wasm"
	case strings.Contains(value, "module") || strings.Contains(value, "import"):
		return "module"
	case strings.Contains(value, "sandbox"):
		return "sandbox"
	case value != "":
		return "runtime"
	default:
		return ""
	}
}

func appendCategory(categories []string, category string) []string {
	if category == "" {
		return categories
	}
	for _, existing := range categories {
		if existing == category {
			return categories
		}
	}
	categories = append(categories, category)
	sort.Strings(categories)
	return categories
}

func redactedDiagnosticURL(target *url.URL) string {
	if target == nil {
		return "-"
	}
	copy := *target
	copy.RawQuery = ""
	copy.ForceQuery = false
	copy.Fragment = ""
	return network.RedactedURL(&copy)
}

func redactedDiagnosticURLString(value string) string {
	target, err := url.Parse(value)
	if err != nil {
		return "-"
	}
	return redactedDiagnosticURL(target)
}
