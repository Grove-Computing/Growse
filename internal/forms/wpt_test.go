package forms

import "testing"

// WPT source: html/semantics/forms/form-submission-0/urlencoded2.window.js.
func TestWPTURLEncodedPreservesDuplicateNamesAndEmptyValues(t *testing.T) {
	entries := []Entry{{Name: "q", Value: "one"}, {Name: "empty", Value: ""}, {Name: "q", Value: "two"}}
	if got, want := EncodeURLEncoded(entries), "q=one&empty=&q=two"; got != want {
		t.Fatalf("EncodeURLEncoded() = %q, want %q", got, want)
	}
}
