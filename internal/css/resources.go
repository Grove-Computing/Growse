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
			if declaration.Property != "background-image" && declaration.Property != "background" {
				continue
			}
			declaration.Value = parseValue(resolveURLFunctions(declaration.Value.Raw, baseURL))
		}
	}
	for index := range stylesheet.FontFaces {
		stylesheet.FontFaces[index].Source = resolveURLFunctions(stylesheet.FontFaces[index].Source, baseURL)
	}
}

func resolveURLFunctions(value string, baseURL *url.URL) string {
	if baseURL == nil {
		return value
	}
	var output strings.Builder
	for position := 0; position < len(value); {
		relative := strings.Index(strings.ToLower(value[position:]), "url(")
		if relative < 0 {
			output.WriteString(value[position:])
			break
		}
		start := position + relative
		output.WriteString(value[position:start])
		end := start + 4
		quote := byte(0)
		for end < len(value) {
			character := value[end]
			if quote != 0 {
				if character == '\\' {
					end++
				} else if character == quote {
					quote = 0
				}
			} else if character == '\'' || character == '"' {
				quote = character
			} else if character == ')' {
				break
			}
			end++
		}
		if end >= len(value) {
			output.WriteString(value[start:])
			break
		}
		raw := strings.TrimSpace(value[start+4 : end])
		if decoded, ok := DecodeString(raw); ok {
			raw = decoded
		}
		reference, err := url.Parse(raw)
		if err != nil {
			output.WriteString(value[start : end+1])
		} else {
			output.WriteString("url(")
			output.WriteString(strconv.Quote(baseURL.ResolveReference(reference).String()))
			output.WriteByte(')')
		}
		position = end + 1
	}
	return output.String()
}
