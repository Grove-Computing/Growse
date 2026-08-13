package css

import (
	"net/url"
	"strconv"
	"strings"
)

// ResolveResourceURLs makes supported CSS resource URLs absolute while their
// stylesheet base URL is still known.
func ResolveResourceURLs(stylesheet *Stylesheet, baseURL *url.URL) {
	if stylesheet == nil || baseURL == nil {
		return
	}
	for ruleIndex := range stylesheet.Rules {
		for declarationIndex := range stylesheet.Rules[ruleIndex].Declarations {
			declaration := &stylesheet.Rules[ruleIndex].Declarations[declarationIndex]
			if declaration.Property != "background-image" {
				continue
			}
			resource, ok := singleURL(declaration.Value.Raw)
			if !ok {
				continue
			}
			reference, err := url.Parse(resource)
			if err != nil {
				continue
			}
			resolved := baseURL.ResolveReference(reference)
			if resolved.Scheme != "http" && resolved.Scheme != "https" {
				continue
			}
			declaration.Value = parseValue("url(" + strconv.Quote(resolved.String()) + ")")
		}
	}
}

func singleURL(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(strings.ToLower(value), "url(") || !strings.HasSuffix(value, ")") {
		return "", false
	}
	raw := strings.TrimSpace(value[4 : len(value)-1])
	if decoded, ok := DecodeString(raw); ok {
		raw = decoded
	}
	return raw, raw != "" && !strings.ContainsAny(raw, "\x00\r\n")
}
