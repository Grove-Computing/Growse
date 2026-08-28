package browser

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/devtools"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
)

const maxCompatibilityDiagnostics = 2000

const maxCompatibilityDiagnosticBytes = 4 * 1024

const maxCompatibilityDiagnosticCount = 1_000_000

func compatibilityDiagnostics(page *Page) []devtools.CompatibilityDiagnostic {
	if page == nil {
		return nil
	}
	diagnostics := make([]devtools.CompatibilityDiagnostic, 0)
	indexes := make(map[devtools.CompatibilityDiagnostic]int)
	appendDiagnostic := func(diagnostic devtools.CompatibilityDiagnostic) {
		diagnostic = normalizeCompatibilityDiagnostic(diagnostic)
		if diagnostic.Count <= 0 {
			diagnostic.Count = 1
		}
		key := diagnostic
		key.Count = 0
		if index, exists := indexes[key]; exists {
			if diagnostics[index].Count > maxCompatibilityDiagnosticCount-diagnostic.Count {
				diagnostics[index].Count = maxCompatibilityDiagnosticCount
			} else {
				diagnostics[index].Count += diagnostic.Count
			}
			return
		}
		if len(diagnostics) >= maxCompatibilityDiagnostics {
			return
		}
		indexes[key] = len(diagnostics)
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
	for _, failure := range page.FontErrors {
		appendDiagnostic(devtools.CompatibilityDiagnostic{Category: "font", Subject: "font", State: "fallback", Reason: fallbackCategory(failure)})
	}
	for _, failure := range append(append([]string(nil), page.ImageErrors...), page.BackgroundErrors...) {
		appendDiagnostic(devtools.CompatibilityDiagnostic{Category: "image", Subject: "image", State: "fallback", Reason: fallbackCategory(failure)})
	}
	for _, failure := range page.ScriptErrors {
		appendDiagnostic(devtools.CompatibilityDiagnostic{Category: "runtime", Subject: "script", State: "error", Reason: runtimeErrorCategory(failure)})
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
				Category: "runtime", Subject: "console/" + diagnosticSourceCategory(record.Source), State: "error", Reason: reason,
			})
		}
	}
	return diagnostics
}

func diagnosticSourceCategory(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "console", "runtime", "script", "event", "scheduler", "module", "observer":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "other"
	}
}

func normalizeCompatibilityDiagnostic(diagnostic devtools.CompatibilityDiagnostic) devtools.CompatibilityDiagnostic {
	diagnostic.Category = truncateDiagnosticUTF8(diagnostic.Category)
	diagnostic.Subject = truncateDiagnosticUTF8(diagnostic.Subject)
	diagnostic.State = truncateDiagnosticUTF8(diagnostic.State)
	diagnostic.Reason = truncateDiagnosticUTF8(diagnostic.Reason)
	diagnostic.Initiator = truncateDiagnosticUTF8(diagnostic.Initiator)
	diagnostic.Schedule = truncateDiagnosticUTF8(diagnostic.Schedule)
	if diagnostic.Count > maxCompatibilityDiagnosticCount {
		diagnostic.Count = maxCompatibilityDiagnosticCount
	}
	return diagnostic
}

func truncateDiagnosticUTF8(value string) string {
	value = strings.ToValidUTF8(value, "�")
	if len(value) <= maxCompatibilityDiagnosticBytes {
		return value
	}
	const suffix = "…"
	limit := maxCompatibilityDiagnosticBytes - len(suffix)
	for limit > 0 && !utf8.ValidString(value[:limit]) {
		limit--
	}
	return value[:limit] + suffix
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
