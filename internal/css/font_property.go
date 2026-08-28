package css

import (
	"bytes"
	"strings"
)

const maxDescriptorValueBytes = 64 << 10

func rewritePropertyAtRules(input []byte) []byte {
	var output strings.Builder
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
		const keyword = "@property"
		if position+len(keyword) <= len(input) && strings.EqualFold(string(input[position:position+len(keyword)]), keyword) &&
			(position+len(keyword) == len(input) || !containerNameByte(input[position+len(keyword)])) {
			preludeEnd := containerPreludeEnd(input, position+len(keyword))
			if preludeEnd > 0 {
				name := strings.TrimSpace(string(input[position+len(keyword) : preludeEnd]))
				if strings.HasPrefix(name, "--") && validName(strings.TrimPrefix(name, "--")) {
					output.WriteString("@font-face { growse-property-name: ")
					output.WriteString(name)
					output.WriteString(";")
					position = preludeEnd + 1
					continue
				}
			}
		}
		output.WriteByte(input[position])
		position++
	}
	return []byte(output.String())
}

func applyFontFaceDescriptor(rule *FontFaceRule, property, value string) {
	if rule == nil || value == "" || len(value) > maxDescriptorValueBytes {
		return
	}
	switch property {
	case "font-family":
		if decoded, ok := DecodeString(value); ok {
			value = decoded
		}
		if value != "" && !strings.ContainsAny(value, "{};") {
			rule.Family = strings.TrimSpace(value)
		}
	case "src":
		rule.Source = value
	case "font-style":
		rule.Style = strings.ToLower(value)
	case "font-weight":
		rule.Weight = strings.ToLower(value)
	case "font-stretch":
		rule.Stretch = strings.ToLower(value)
	case "unicode-range":
		rule.UnicodeRange = value
	case "font-display":
		rule.Display = strings.ToLower(value)
	}
}

func applyPropertyDescriptor(rule *PropertyRule, property, value string) {
	if rule == nil || value == "" || len(value) > maxDescriptorValueBytes {
		return
	}
	switch property {
	case "syntax":
		if decoded, ok := DecodeString(value); ok {
			value = decoded
		}
		rule.Syntax = strings.TrimSpace(value)
	case "initial-value":
		rule.InitialValue = strings.TrimSpace(value)
	case "inherits":
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true":
			rule.Inherits, rule.InheritsSet = true, true
		case "false":
			rule.Inherits, rule.InheritsSet = false, true
		}
	}
}

func finalizeStylesheetMetadata(stylesheet *Stylesheet) {
	if stylesheet == nil {
		return
	}
	faces := stylesheet.FontFaces[:0]
	for _, face := range stylesheet.FontFaces {
		if face.Family == "" || face.Source == "" {
			continue
		}
		if face.Style == "" {
			face.Style = "normal"
		}
		if face.Weight == "" {
			face.Weight = "normal"
		}
		if face.Stretch == "" {
			face.Stretch = "normal"
		}
		if face.Display == "" {
			face.Display = "auto"
		}
		faces = append(faces, face)
	}
	stylesheet.FontFaces = faces
	for index := range stylesheet.Properties {
		rule := &stylesheet.Properties[index]
		switch rule.Syntax {
		case "*":
			rule.Valid = rule.InheritsSet
		case "<length>", "<number>", "<color>":
			rule.Valid = rule.InheritsSet && rule.InitialValue != ""
		default:
			rule.Valid = false
		}
	}
}
