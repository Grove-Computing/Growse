package forms

import (
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestFormOwnerPrefersExplicitFormAttribute(t *testing.T) {
	document := dom.NewDocument()
	outer := document.CreateElement("form", map[string]string{"id": "outer"})
	target := document.CreateElement("form", map[string]string{"id": "target"})
	control := document.CreateElement("input", map[string]string{"form": "target"})
	appendNode(t, document, document.Root, outer)
	appendNode(t, document, document.Root, target)
	appendNode(t, document, outer, control)
	if got := FormOwner(document, control); got != target {
		t.Fatalf("form owner = %#v, want target", got)
	}
}

func TestResolveSubmissionAppliesSubmitterOverridesAndDefaults(t *testing.T) {
	document := dom.NewDocument()
	form := document.CreateElement("form", map[string]string{
		"id": "search", "action": "/default", "method": "post", "enctype": URLEncoded, "target": "result",
	})
	submitter := document.CreateElement("button", map[string]string{
		"form": "search", "formaction": "/override", "formmethod": "get", "formenctype": "text/plain",
		"formtarget": "_self", "formnovalidate": "",
	})
	appendNode(t, document, document.Root, form)
	appendNode(t, document, document.Root, submitter)

	config, ok := ResolveSubmission(document, submitter)
	if !ok || config.Form != form || config.Submitter != submitter || config.Action != "/override" || config.Method != "get" ||
		config.Enctype != URLEncoded || config.Target != "_self" || !config.NoValidate {
		t.Fatalf("submission config = %#v, ok=%v", config, ok)
	}

	plain := document.CreateElement("button", map[string]string{"form": "search"})
	appendNode(t, document, document.Root, plain)
	config, ok = ResolveSubmission(document, plain)
	if !ok || config.Action != "/default" || config.Method != "post" || config.Enctype != URLEncoded || config.Target != "result" || config.NoValidate {
		t.Fatalf("default config = %#v, ok=%v", config, ok)
	}
}
