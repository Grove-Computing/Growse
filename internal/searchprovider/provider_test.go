package searchprovider

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestResolveDuckDuckGoResultURL(t *testing.T) {
	redirect, err := url.Parse("https://duckduckgo.com/l/?uddg=https%3A%2F%2Fja.wikipedia.org%2Fwiki%2F%25E3%2583%25A1%25E3%2582%25BF%25E6%25A7%258B%25E6%2596%2587%25E5%25A4%2589%25E6%2595%25B0&rut=ignored")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveResultURL(redirect)
	if err != nil || resolved.String() != "https://ja.wikipedia.org/wiki/%E3%83%A1%E3%82%BF%E6%A7%8B%E6%96%87%E5%A4%89%E6%95%B0" {
		t.Fatalf("resolved result = %v, %v", resolved, err)
	}

	direct, _ := url.Parse("https://example.com/page")
	resolved, err = ResolveResultURL(direct)
	if err != nil || resolved.String() != direct.String() || resolved == direct {
		t.Fatalf("direct result = %v, %v", resolved, err)
	}

	for _, raw := range []string{
		"https://duckduckgo.com/l/",
		"https://duckduckgo.com/l/?uddg=javascript%3Aalert%281%29",
		"https://duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com&uddg=https%3A%2F%2Fevil.example",
		"https://duckduckgo.com/l/?uddg=https%3A%2F%2Falice%3Asecret%40example.com",
	} {
		link, _ := url.Parse(raw)
		if resolved, err := ResolveResultURL(link); err == nil || resolved != nil {
			t.Fatalf("invalid redirect %q resolved to %v, %v", raw, resolved, err)
		}
	}
}

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

func TestProviderLimitsAndPathEncoding(t *testing.T) {
	s := Defaults()
	for i := 1; i < MaxProviders; i++ {
		p := Provider{ID: fmt.Sprintf("p%d", i), Name: "Provider", Keyword: fmt.Sprintf("p%d", i), SearchTemplate: "https://example.com/?q={searchTerms}"}
		if err := s.Put(p); err != nil {
			t.Fatal(err)
		}
	}
	extra := Provider{ID: "extra", Name: "Extra", Keyword: "extra", SearchTemplate: "https://example.com/?q={searchTerms}"}
	if s.Put(extra) == nil {
		t.Fatal("exceeded provider limit")
	}
	p := Builtin()
	p.SearchTemplate = "https://example.com/search/{searchTerms}"
	target, err := p.SearchURL("日本語/a + %20")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(target)
	if u.Path != "/search/日本語/a + %20" || !strings.Contains(u.EscapedPath(), "%2F") {
		t.Fatal("path query not escaped once", target)
	}
	for _, template := range []string{"https://example.com/?q={searchTerms}#", "https://example.com:0/?q={searchTerms}", "https://example.com:65536/?q={searchTerms}", "https://example.com/?q={searchTerms}{searchTerms}"} {
		p.SearchTemplate = template
		if p.Validate() == nil {
			t.Fatal(template)
		}
	}
}
