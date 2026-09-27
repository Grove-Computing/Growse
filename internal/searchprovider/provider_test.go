package searchprovider

import (
	"net/url"
	"testing"
)

func TestProviderManagementAndKeyword(t *testing.T) {
	s := Defaults()
	p := Provider{ID: "custom", Name: "Custom", Keyword: "c", SearchTemplate: "https://example.com/search?q={searchTerms}"}
	if err := s.Put(p); err != nil {
		t.Fatal(err)
	}
	got, q, override := s.Resolve("c 日本語 + %20")
	if got.ID != "custom" || q != "日本語 + %20" || !override || s.DefaultID != "duckduckgo" {
		t.Fatal(got, q, override)
	}
	target, err := got.SearchURL(q)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(target)
	if u.Query().Get("q") != q {
		t.Fatal(target)
	}
	s.DefaultID = "custom"
	if err := s.Delete("custom"); err != nil || s.DefaultID != "duckduckgo" {
		t.Fatal(err, s)
	}
	if s.Delete("duckduckgo") == nil {
		t.Fatal("deleted builtin")
	}
	p.ID = "other"
	p.Keyword = "ddg"
	if s.Put(p) == nil {
		t.Fatal("duplicate keyword")
	}
}
func TestInvalidTemplates(t *testing.T) {
	for _, v := range []string{"http://example.com/?q={searchTerms}", "https://u:p@example.com/?q={searchTerms}", "https://example.com/?q={searchTerms}#x", "https://example.com/?q=x", "https://example.com/?q={searchTerms}{unknown}", "https://{searchTerms}.com/", "https://example.com/?{searchTerms}=x"} {
		p := Builtin()
		p.SearchTemplate = v
		if p.Validate() == nil {
			t.Errorf("accepted %s", v)
		}
	}
}
