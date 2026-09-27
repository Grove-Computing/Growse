package searchprovider

import (
	"context"
	"encoding/xml"
	"net/url"
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
)

const openSearchNamespace = "http://a9.com/-/spec/opensearch/1.1/"

// Discovery records a declared description URL. Detection never fetches it.
type Discovery struct {
	Title string
	URL   string
}

func Discover(document *dom.Document, base *url.URL) []Discovery {
	if document == nil || base == nil {
		return nil
	}
	var out []Discovery
	seen := map[string]bool{}
	root := document.Snapshot().Root
	stack := []*dom.NodeSnapshot{&root}
	for len(stack) > 0 && len(out) < MaxProviders {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if strings.EqualFold(n.TagName, "link") && strings.EqualFold(strings.TrimSpace(n.Attributes["type"]), "application/opensearchdescription+xml") {
			alternate := false
			for _, r := range strings.Fields(strings.ToLower(n.Attributes["rel"])) {
				if r == "search" {
					alternate = true
				}
			}
			if alternate {
				if ref, err := url.Parse(n.Attributes["href"]); err == nil && n.Attributes["href"] != "" {
					u := base.ResolveReference(ref)
					if validateEndpoint(u) == nil && !seen[u.String()] {
						out = append(out, Discovery{Title: n.Attributes["title"], URL: u.String()})
						seen[u.String()] = true
					}
				}
			}
		}
		for i := len(n.Children) - 1; i >= 0; i-- {
			stack = append(stack, &n.Children[i])
		}
	}
	return out
}

// ImportDescription requires explicit confirmation before any network access.
func (t *Transport) ImportDescription(ctx context.Context, d Discovery, id, keyword string, confirmed bool) (Provider, error) {
	if !confirmed {
		return Provider{}, ErrInvalid
	}
	body, err := t.get(ctx, d.URL, MaxDescriptionBytes)
	if err != nil {
		return Provider{}, err
	}
	return ParseDescription(body, id, keyword)
}
func ParseDescription(body []byte, id, keyword string) (Provider, error) {
	if len(body) > MaxDescriptionBytes {
		return Provider{}, ErrResponse
	}
	var description struct {
		XMLName       xml.Name
		Name          string   `xml:"ShortName"`
		InputEncoding []string `xml:"InputEncoding"`
		URLs          []struct {
			Type     string     `xml:"type,attr"`
			Method   string     `xml:"method,attr"`
			Template string     `xml:"template,attr"`
			Params   []struct{} `xml:"Param"`
		} `xml:"Url"`
	}
	if xml.Unmarshal(body, &description) != nil || description.XMLName.Local != "OpenSearchDescription" || description.XMLName.Space != openSearchNamespace {
		return Provider{}, ErrInvalid
	}
	for _, encoding := range description.InputEncoding {
		if !strings.EqualFold(strings.TrimSpace(encoding), "UTF-8") {
			return Provider{}, ErrInvalid
		}
	}
	p := Provider{ID: id, Name: strings.TrimSpace(description.Name), Keyword: keyword}
	for _, u := range description.URLs {
		if u.Method != "" && !strings.EqualFold(u.Method, "GET") || len(u.Params) != 0 {
			return Provider{}, ErrInvalid
		}
		switch u.Type {
		case "text/html":
			if p.SearchTemplate != "" {
				return Provider{}, ErrInvalid
			}
			p.SearchTemplate = u.Template
		case "application/x-suggestions+json":
			if p.SuggestionTemplate != "" {
				return Provider{}, ErrInvalid
			}
			p.SuggestionTemplate = u.Template
		}
	}
	if p.Validate() != nil {
		return Provider{}, ErrInvalid
	}
	return p, nil
}
