package html

import (
	"strings"
	"testing"
)

func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		"<!doctype html><html><body><p>Hello</p></body></html>",
		"<html><body><p>first<p>second",
		"<script>const value = '<>&'</script>",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 64*1024 {
			t.Skip()
		}
		_, _ = Parse(strings.NewReader(source))
	})
}
