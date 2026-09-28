package findpage

import (
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

func TestSearchReturnsVisibleTextMatchesInDOMOrder(t *testing.T) {
	document := dom.NewDocument()
	body := document.CreateElement("body", nil)
	first := document.CreateText("Alpha alpha")
	hidden := document.CreateElement("section", nil)
	hiddenText := document.CreateText("alpha")
	control := document.CreateElement("textarea", nil)
	controlText := document.CreateText("alpha")
	last := document.CreateText(" ALPHA")
	mustAppend(t, document, document.Root, body)
	mustAppend(t, document, body, first)
	mustAppend(t, document, body, hidden)
	mustAppend(t, document, hidden, hiddenText)
	mustAppend(t, document, body, control)
	mustAppend(t, document, control, controlText)
	mustAppend(t, document, body, last)

	styles := stylemodel.Map{body.ID: {}, hidden.ID: {Display: stylemodel.DisplayNone}, control.ID: {}}
	result := Search(document, styles, "alpha", Options{}, Limits{})
	if result.Limit != LimitNone || len(result.Matches) != 3 {
		t.Fatalf("result = %#v, want three visible matches", result)
	}
	want := []Match{{NodeID: first.ID, Start: 0, End: 5}, {NodeID: first.ID, Start: 6, End: 11}, {NodeID: last.ID, Start: 1, End: 6}}
	for index := range want {
		if result.Matches[index] != want[index] {
			t.Fatalf("match[%d] = %#v, want %#v", index, result.Matches[index], want[index])
		}
	}
}

func TestSearchNormalizesCaseCJKAndCombiningSequences(t *testing.T) {
	document := documentWithText(t, "Cafe\u0301 café 日本語日本語")
	text := document.Root.Children[0].Children[0]

	result := Search(document, nil, "CAFÉ", Options{}, Limits{})
	if got := result.Matches; len(got) != 2 || got[0] != (Match{NodeID: text.ID, Start: 0, End: 6}) || got[1] != (Match{NodeID: text.ID, Start: 7, End: 12}) {
		t.Fatalf("normalized matches = %#v", got)
	}
	if split := Search(document, nil, "\u0301", Options{}, Limits{}); len(split.Matches) != 0 {
		t.Fatalf("combining mark split a normalization segment: %#v", split.Matches)
	}
	if cjk := Search(document, nil, "日本語", Options{}, Limits{}); len(cjk.Matches) != 2 {
		t.Fatalf("CJK matches = %#v, want 2", cjk.Matches)
	}
	if sensitive := Search(document, nil, "CAFÉ", Options{CaseSensitive: true}, Limits{}); len(sensitive.Matches) != 0 {
		t.Fatalf("case-sensitive matches = %#v, want none", sensitive.Matches)
	}
}

func TestSearchWholeWordUsesUnicodeBoundaries(t *testing.T) {
	document := documentWithText(t, "cat scatter cat 猫 山猫")
	if got := Search(document, nil, "cat", Options{WholeWord: true}, Limits{}).Matches; len(got) != 2 {
		t.Fatalf("whole word Latin matches = %#v, want 2", got)
	}
	if got := Search(document, nil, "猫", Options{WholeWord: true}, Limits{}).Matches; len(got) != 1 {
		t.Fatalf("whole word CJK matches = %#v, want 1", got)
	}
}

func TestSearchReportsTextAndMatchLimits(t *testing.T) {
	document := documentWithText(t, "aaaa")
	matchLimited := Search(document, nil, "a", Options{}, Limits{MaxMatches: 2, MaxTextBytes: 10})
	if matchLimited.Limit != LimitMatches || len(matchLimited.Matches) != 2 {
		t.Fatalf("match-limited result = %#v", matchLimited)
	}
	textLimited := Search(document, nil, "a", Options{}, Limits{MaxMatches: 10, MaxTextBytes: 3})
	if textLimited.Limit != LimitTextBytes || len(textLimited.Matches) != 0 {
		t.Fatalf("text-limited result = %#v", textLimited)
	}
}

func documentWithText(t *testing.T, value string) *dom.Document {
	t.Helper()
	document := dom.NewDocument()
	body := document.CreateElement("body", nil)
	text := document.CreateText(value)
	mustAppend(t, document, document.Root, body)
	mustAppend(t, document, body, text)
	return document
}

func mustAppend(t *testing.T, document *dom.Document, parent, child *dom.Node) {
	t.Helper()
	if err := document.AppendChild(parent, child); err != nil {
		t.Fatal(err)
	}
}
