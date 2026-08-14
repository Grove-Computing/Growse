package forms

import (
	"errors"
	"strings"
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

func TestCollectEntriesPreservesDOMOrderDuplicatesAndSuccessfulControls(t *testing.T) {
	document := dom.NewDocument()
	form := document.CreateElement("form", map[string]string{"id": "survey"})
	first := document.CreateElement("input", map[string]string{"name": "tag", "value": "one"})
	empty := document.CreateElement("input", map[string]string{"name": "empty", "value": ""})
	checkbox := document.CreateElement("input", map[string]string{"type": "checkbox", "name": "agree", "checked": ""})
	unchecked := document.CreateElement("input", map[string]string{"type": "checkbox", "name": "skip"})
	textarea := document.CreateElement("textarea", map[string]string{"name": "note"})
	selectNode := document.CreateElement("select", map[string]string{"name": "size"})
	option := document.CreateElement("option", map[string]string{"value": "large", "selected": ""})
	submit := document.CreateElement("button", map[string]string{"name": "intent", "value": "save"})
	otherSubmit := document.CreateElement("button", map[string]string{"name": "intent", "value": "delete"})
	external := document.CreateElement("input", map[string]string{"form": "survey", "name": "tag", "value": "two"})
	appendNode(t, document, document.Root, form)
	for _, node := range []*dom.Node{first, empty, checkbox, unchecked, textarea, selectNode, submit, otherSubmit} {
		appendNode(t, document, form, node)
	}
	appendNode(t, document, textarea, document.CreateText("a\nb\r\nc"))
	appendNode(t, document, selectNode, option)
	appendNode(t, document, option, document.CreateText("Large"))
	appendNode(t, document, document.Root, external)

	want := []Entry{
		{Name: "tag", Value: "one"}, {Name: "empty", Value: ""}, {Name: "agree", Value: "on"},
		{Name: "note", Value: "a\r\nb\r\nc"}, {Name: "size", Value: "large"},
		{Name: "intent", Value: "save"}, {Name: "tag", Value: "two"},
	}
	got := CollectEntries(document, form, submit)
	if len(got) != len(want) {
		t.Fatalf("entries = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("entry %d = %#v, want %#v", index, got[index], want[index])
		}
	}
}

func TestEncodeURLEncodedUsesUTF8AndPreservesEntryOrder(t *testing.T) {
	entries := []Entry{
		{Name: "tag", Value: "one"},
		{Name: "日本 語", Value: "東京/大阪~"},
		{Name: "tag", Value: ""},
	}
	want := "tag=one&%E6%97%A5%E6%9C%AC+%E8%AA%9E=%E6%9D%B1%E4%BA%AC%2F%E5%A4%A7%E9%98%AA%7E&tag="
	if got := EncodeURLEncoded(entries); got != want {
		t.Fatalf("encoded = %q, want %q", got, want)
	}
}

func TestEncodeURLEncodedRejectsPathologicalEntryCountAndSize(t *testing.T) {
	tooMany := make([]Entry, MaxFormEntries+1)
	if _, err := EncodeURLEncodedLimited(tooMany); !errors.Is(err, ErrFormDataTooLarge) {
		t.Fatalf("entry count error = %v", err)
	}
	if _, err := EncodeURLEncodedLimited([]Entry{{Name: "value", Value: strings.Repeat("x", MaxEncodedFormBytes)}}); !errors.Is(err, ErrFormDataTooLarge) {
		t.Fatalf("encoded size error = %v", err)
	}
}
