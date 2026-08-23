package css

import (
	"strings"
	"testing"
)

func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		"body { color: red; }",
		"@media (min-width: 40rem) { .card { display: grid; } }",
		"p::before { content: '<&>'; }",
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
