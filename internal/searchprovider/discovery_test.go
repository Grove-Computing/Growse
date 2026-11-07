package searchprovider

import (
	"context"
	"github.com/Grove-Computing/Growse/internal/dom"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestConfirmedDiscovery(t *testing.T) {
	doc := dom.NewDocument()
	doc.Root.Children = []*dom.Node{{TagName: "link", Attributes: map[string]string{"rel": "search", "type": "application/opensearchdescription+xml", "href": "/opensearch.xml", "title": "Example"}}}
	base, _ := url.Parse("https://example.com/page")
	ds := Discover(doc, base)
	if len(ds) != 1 || ds[0].URL != "https://example.com/opensearch.xml" {
		t.Fatal(ds)
	}
	body := `<OpenSearchDescription xmlns="http://a9.com/-/spec/opensearch/1.1/"><ShortName>Example</ShortName><Url type="text/html" template="https://example.com/?q={searchTerms}"/></OpenSearchDescription>`
	calls := 0
	tr := NewTransport(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})})
	if _, err := tr.ImportDescription(context.Background(), ds[0], "example", "ex", false); err == nil || calls != 0 {
		t.Fatal("fetched without confirmation")
	}
	p, err := tr.ImportDescription(context.Background(), ds[0], "example", "ex", true)
	if err != nil || p.Name != "Example" || calls != 1 {
		t.Fatal(p, err, calls)
	}
	for _, bad := range []string{strings.Replace(body, "https://", "http://", 1), strings.Replace(body, "{searchTerms}", "{unknown}", 1), strings.Replace(body, "text/html", `text/html" method="POST`, 1), "<bad>", strings.Repeat("x", MaxDescriptionBytes+1)} {
		if _, err := ParseDescription([]byte(bad), "example", "ex"); err == nil {
			t.Fatal("accepted invalid description")
		}
	}
}
