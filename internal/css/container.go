package css

import (
	"bytes"
	"strings"
)

const containerRewritePrefix = "growse-container("

func rewriteContainerAtRules(input []byte) []byte {
	var output bytes.Buffer
	for position := 0; position < len(input); {
		if input[position] == '/' && position+1 < len(input) && input[position+1] == '*' {
			end := bytes.Index(input[position+2:], []byte("*/"))
			if end < 0 {
				output.Write(input[position:])
				break
			}
			end += position + 4
			output.Write(input[position:end])
			position = end
			continue
		}
		if input[position] == '\'' || input[position] == '"' {
			end := quotedBytesEnd(input, position)
			output.Write(input[position:end])
			position = end
			continue
		}
		const keyword = "@container"
		if position+len(keyword) <= len(input) && strings.EqualFold(string(input[position:position+len(keyword)]), keyword) &&
			(position+len(keyword) == len(input) || !containerNameByte(input[position+len(keyword)])) {
			preludeEnd := containerPreludeEnd(input, position+len(keyword))
			if preludeEnd > 0 {
				output.WriteString("@supports ")
				output.WriteString(containerRewritePrefix)
				output.Write(input[position+len(keyword) : preludeEnd])
				output.WriteByte(')')
				position = preludeEnd
				continue
			}
		}
		output.WriteByte(input[position])
		position++
	}
	return output.Bytes()
}

func containerNameByte(value byte) bool {
	return value == '-' || value == '_' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value >= 0x80
}

func quotedBytesEnd(input []byte, start int) int {
	quote, escaped := input[start], false
	for position := start + 1; position < len(input); position++ {
		if escaped {
			escaped = false
		} else if input[position] == '\\' {
			escaped = true
		} else if input[position] == quote {
			return position + 1
		}
	}
	return len(input)
}

func containerPreludeEnd(input []byte, start int) int {
	depth := 0
	for position := start; position < len(input); position++ {
		if input[position] == '/' && position+1 < len(input) && input[position+1] == '*' {
			end := bytes.Index(input[position+2:], []byte("*/"))
			if end < 0 {
				return 0
			}
			position += end + 3
			continue
		}
		if input[position] == '\'' || input[position] == '"' {
			position = quotedBytesEnd(input, position) - 1
			continue
		}
		switch input[position] {
		case '(':
			depth++
		case ')':
			depth--
		case '{':
			if depth == 0 {
				return position
			}
		}
		if depth < 0 {
			return 0
		}
	}
	return 0
}

func unwrapContainerPrelude(value string) (string, bool) {
	if !strings.HasPrefix(strings.ToLower(value), containerRewritePrefix) {
		return "", false
	}
	open := len("growse-container")
	end, ok := parenthesisEnd(value, open)
	if !ok || strings.TrimSpace(value[end+1:]) != "" {
		return "", false
	}
	return strings.TrimSpace(value[open+1 : end]), true
}

// parseContainerQuery accepts an optional container name followed by one or
// more parenthesized inline-size features joined by and.
func parseContainerQuery(value string) ContainerQuery {
	value = strings.TrimSpace(value)
	result := ContainerQuery{}
	firstParen := strings.IndexByte(value, '(')
	if firstParen < 0 {
		return result
	}
	name := strings.TrimSpace(value[:firstParen])
	if name != "" {
		if !validName(name) || strings.EqualFold(name, "none") {
			return result
		}
		result.Name = name
	}
	rest := strings.TrimSpace(value[firstParen:])
	for rest != "" {
		end, ok := parenthesisEnd(rest, 0)
		if !ok {
			return ContainerQuery{}
		}
		property, featureValue, ok := splitSupportsDeclaration(strings.TrimSpace(rest[1:end]))
		if !ok || property != "width" && property != "min-width" && property != "max-width" {
			return ContainerQuery{}
		}
		result.Features = append(result.Features, MediaFeature{Name: property, Value: featureValue})
		rest = strings.TrimSpace(rest[end+1:])
		if rest == "" {
			break
		}
		if !hasSupportsKeywordPrefix(rest, "and") {
			return ContainerQuery{}
		}
		rest = strings.TrimSpace(rest[len("and"):])
	}
	result.Valid = len(result.Features) != 0
	return result
}
