package browser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/html"
)

func TestLoadImportMapUsesFirstValidInlineMap(t *testing.T) {
	document, err := html.Parse(strings.NewReader(`
		<script type="importmap">invalid</script>
		<script type="importmap" src="/external.json"></script>
		<script type="IMPORTMAP">{
			"imports": {
				"app": "./modules/app.js",
				"lib/": "https://cdn.example/lib/",
				"./not-bare.js": "./ignored.js",
				"invalid": 42
			}
		}</script>
		<script type="importmap">{"imports":{"later":"./later.js"}}</script>`))
	if err != nil {
		t.Fatal(err)
	}
	mappings, loadErrors := loadImportMap(document, mustParseURL(t, "https://app.example/pages/index.html"))
	if got, want := mappings["app"], "https://app.example/pages/modules/app.js"; got != want {
		t.Fatalf("app mapping = %q, want %q", got, want)
	}
	if got, want := mappings["lib/"], "https://cdn.example/lib/"; got != want {
		t.Fatalf("lib mapping = %q, want %q", got, want)
	}
	if len(mappings) != 2 || len(loadErrors) != 2 {
		t.Fatalf("mappings=%v errors=%v", mappings, loadErrors)
	}
	if _, exists := mappings["later"]; exists {
		t.Fatal("a later import map was applied")
	}
}

func TestLoadImportMapEnforcesSourceAndMappingLimits(t *testing.T) {
	tooLarge, _ := html.Parse(strings.NewReader(`<script type="importmap">` + strings.Repeat(" ", maxImportMapBytes+1) + `</script>`))
	if mappings, loadErrors := loadImportMap(tooLarge, mustParseURL(t, "https://app.example/")); mappings != nil || len(loadErrors) != 1 || !strings.Contains(loadErrors[0], "bytes") {
		t.Fatalf("oversized import map = mappings:%v errors:%v", mappings, loadErrors)
	}

	entries := make([]string, maxImportMapMappings+1)
	for index := range entries {
		entries[index] = fmt.Sprintf("%q:%q", fmt.Sprintf("pkg-%d", index), "./module.js")
	}
	document, _ := html.Parse(strings.NewReader(`<script type="importmap">{"imports":{` + strings.Join(entries, ",") + `}}</script>`))
	if mappings, loadErrors := loadImportMap(document, mustParseURL(t, "https://app.example/")); mappings != nil || len(loadErrors) != 1 || !strings.Contains(loadErrors[0], "mappings") {
		t.Fatalf("mapping-limited import map = mappings:%v errors:%v", mappings, loadErrors)
	}
}
