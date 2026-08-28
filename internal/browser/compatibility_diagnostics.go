package browser

import (
	"fmt"
	"strings"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/devtools"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
)

const maxCompatibilityDiagnostics = 2000

func compatibilityDiagnostics(page *Page) []devtools.CompatibilityDiagnostic {
	if page == nil {
		return nil
	}
	diagnostics := make([]devtools.CompatibilityDiagnostic, 0)
	appendDiagnostic := func(diagnostic devtools.CompatibilityDiagnostic) {
		if len(diagnostics) >= maxCompatibilityDiagnostics {
			return
		}
		if diagnostic.Count <= 0 {
			diagnostic.Count = 1
		}
		diagnostics = append(diagnostics, diagnostic)
	}

	if page.DevTools != nil {
		for _, record := range page.DevTools.Network() {
			if !compatibilityResourceKind(record.Kind) {
				continue
			}
			state, reason := "loaded", "ok"
			if record.ErrorCategory != "" || record.StatusCode >= 400 {
				state, reason = "error", record.ErrorCategory
				if reason == "" {
					reason = "http"
				}
			}
			subject := record.URL
			if record.FinalURL != "" && record.FinalURL != "unknown" && record.FinalURL != record.URL {
				subject += " -> " + record.FinalURL
			}
			appendDiagnostic(devtools.CompatibilityDiagnostic{
				Category: "resource/" + record.Kind, Subject: subject, State: state, Reason: reason,
				Initiator: record.Initiator, Schedule: record.Schedule,
			})
		}
	}

	appendStyleDiagnostics(page, appendDiagnostic)
	for index, failure := range page.FontErrors {
		appendDiagnostic(devtools.CompatibilityDiagnostic{Category: "font", Subject: fmt.Sprintf("font#%d", index+1), State: "fallback", Reason: fallbackCategory(failure)})
	}
	for index, failure := range append(append([]string(nil), page.ImageErrors...), page.BackgroundErrors...) {
		appendDiagnostic(devtools.CompatibilityDiagnostic{Category: "image", Subject: fmt.Sprintf("image#%d", index+1), State: "fallback", Reason: fallbackCategory(failure)})
	}
	for index, failure := range page.ScriptErrors {
		appendDiagnostic(devtools.CompatibilityDiagnostic{Category: "runtime", Subject: fmt.Sprintf("script#%d", index+1), State: "error", Reason: runtimeErrorCategory(failure)})
	}
	if page.RuntimeError != "" {
		appendDiagnostic(devtools.CompatibilityDiagnostic{Category: "runtime", Subject: "page", State: "error", Reason: runtimeErrorCategory(page.RuntimeError)})
	}
	if page.DevTools != nil {
		for _, record := range page.DevTools.Console() {
			if record.Level != devtools.ConsoleError {
				continue
			}
			reason := runtimeErrorCategory(record.Message)
			if reason == "" || reason == "runtime" {
				continue
			}
			appendDiagnostic(devtools.CompatibilityDiagnostic{
				Category: "runtime", Subject: "console/" + boundedDiagnosticLabel(record.Source), State: "error", Reason: reason,
			})
		}
	}
	return diagnostics
}

func boundedDiagnosticLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	if len(value) > 64 {
		return value[:64]
	}
	return value
}

func compatibilityResourceKind(kind string) bool {
	switch kind {
	case "script", "module", "stylesheet", "font", "image":
		return true
	default:
		return false
	}
}

func appendStyleDiagnostics(page *Page, appendDiagnostic func(devtools.CompatibilityDiagnostic)) {
	if page.Stylesheet == nil || page.Document == nil {
		return
	}
	environment := style.Environment{
		ViewportWidth: page.ViewportWidth, ViewportHeight: page.ViewportHeight, RootFontSize: 16, ResolutionDPI: 96,
		ColorScheme: "light", Hover: true, Pointer: "fine", ReducedMotion: page.ReducedMotion,
	}
	state := interactionState(page)
	for index, rule := range page.Stylesheet.Rules {
		matched := ruleMatchesDocument(page.Document, rule, state)
		activeMedia := true
		for _, group := range rule.Media {
			if !style.MatchesMediaQueryList(group, environment) {
				activeMedia = false
				break
			}
		}
		stateName, reason := "applied", "matched"
		switch {
		case !matched:
			stateName, reason = "ignored", "selector-unmatched"
		case !activeMedia:
			stateName, reason = "ignored", "media-condition"
		case hasUnknownSupports(rule.Supports):
			stateName, reason = "ignored", "supports-condition"
		case len(rule.Containers) != 0:
			reason = "container-condition"
		}
		layer := rule.Layer
		if layer == "" {
			layer = "unlayered"
		}
		appendDiagnostic(devtools.CompatibilityDiagnostic{
			Category: "style", Subject: fmt.Sprintf("rule#%d layer=%s declarations=%d", index+1, layer, len(rule.Declarations)),
			State: stateName, Reason: reason,
		})
	}
}

func ruleMatchesDocument(document *dom.Document, rule css.Rule, state style.InteractionState) bool {
	if document == nil || document.Root == nil {
		return false
	}
	matched := false
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil || matched {
			return
		}
		if node.Type == dom.NodeElement {
			for _, selector := range rule.Selectors {
				if style.MatchesSelector(node, selector, state) {
					matched = true
					return
				}
			}
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(document.Root)
	return matched
}

func hasUnknownSupports(conditions []css.SupportsCondition) bool {
	for _, condition := range conditions {
		if condition.Kind == css.SupportsUnknown || hasUnknownSupports(condition.Children) {
			return true
		}
	}
	return false
}

func fallbackCategory(message string) string {
	value := strings.ToLower(message)
	switch {
	case strings.Contains(value, "cors") || strings.Contains(value, "origin"):
		return "cors"
	case strings.Contains(value, "mime") || strings.Contains(value, "content-type"):
		return "mime"
	case strings.Contains(value, "timeout"):
		return "timeout"
	case strings.Contains(value, "decode") || strings.Contains(value, "malformed") || strings.Contains(value, "invalid format") || strings.Contains(value, "dimensions were rejected"):
		return "decode"
	case strings.Contains(value, "limit") || strings.Contains(value, "large") || strings.Contains(value, "exceed"):
		return "limit"
	case strings.Contains(value, "unsupported"):
		return "unsupported"
	case strings.Contains(value, "redirect"):
		return "redirect"
	default:
		return "load"
	}
}
