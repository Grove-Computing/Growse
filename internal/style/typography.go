package style

import (
	"strings"
)

type fontShorthand struct {
	style, weight, stretch, size, lineHeight, family string
}

func parseFontShorthand(value string) (fontShorthand, bool) {
	parts, ok := splitCSSSpaceSeparated(strings.TrimSpace(value))
	if !ok || len(parts) < 2 {
		return fontShorthand{}, false
	}
	result := fontShorthand{style: "normal", weight: "400", stretch: "normal", lineHeight: "normal"}
	sizeIndex := -1
	for index, part := range parts {
		candidate := part
		if slash := strings.IndexByte(candidate, '/'); slash >= 0 {
			candidate = candidate[:slash]
		}
		if _, valid := ResolveLength(candidate, LengthContext{FontSize: 16, RootFontSize: 16}); valid {
			result.size, sizeIndex = candidate, index
			if slash := strings.IndexByte(part, '/'); slash >= 0 {
				result.lineHeight = part[slash+1:]
			}
			break
		}
		switch {
		case part == "normal":
		case func() bool { _, valid := parseFontStyle(part); return valid }():
			result.style = part
		case func() bool { _, valid := parseFontWeight(part); return valid }():
			result.weight = part
		case func() bool { _, valid := parseFontStretch(part); return valid }():
			result.stretch = part
		default:
			return fontShorthand{}, false
		}
	}
	if sizeIndex < 0 {
		return fontShorthand{}, false
	}
	familyIndex := sizeIndex + 1
	if familyIndex < len(parts) && parts[familyIndex] == "/" {
		familyIndex++
		if familyIndex >= len(parts) {
			return fontShorthand{}, false
		}
		result.lineHeight = parts[familyIndex]
		familyIndex++
	} else if familyIndex < len(parts) && strings.HasPrefix(parts[familyIndex], "/") {
		result.lineHeight = strings.TrimPrefix(parts[familyIndex], "/")
		familyIndex++
	}
	if familyIndex >= len(parts) || result.lineHeight == "" {
		return fontShorthand{}, false
	}
	result.family = strings.Join(parts[familyIndex:], " ")
	if _, valid := parseFontFamilies(result.family); !valid {
		return fontShorthand{}, false
	}
	return result, true
}

func fontCandidateComponent(candidate winner, resolved, property string) string {
	if candidate.source != "font" || parseGlobalKeyword(resolved) != globalNone {
		return resolved
	}
	parsed, ok := parseFontShorthand(resolved)
	if !ok {
		return ""
	}
	switch property {
	case "font-style":
		return parsed.style
	case "font-weight":
		return parsed.weight
	case "font-stretch":
		return parsed.stretch
	case "font-size":
		return parsed.size
	case "line-height":
		return parsed.lineHeight
	case "font-family":
		return parsed.family
	default:
		return ""
	}
}

func applyTextProperties(computed, parent ComputedStyle, winners map[string]winner, context LengthContext) ComputedStyle {
	applyKeyword := func(property string, inherited bool, initial string, parse func(string) bool, assign func(string)) {
		candidate, ok := winners[property]
		if !ok {
			return
		}
		resolved, ok := resolveVariables(candidate.value, computed.CustomProperties)
		if !ok {
			return
		}
		switch parseGlobalKeyword(resolved) {
		case globalInitial:
			assign(initial)
		case globalUnset:
			if inherited {
				assign("inherit")
			} else {
				assign(initial)
			}
		case globalInherit:
			assign("inherit")
		default:
			lower := strings.ToLower(strings.TrimSpace(resolved))
			if parse(lower) {
				assign(lower)
			}
		}
	}
	applyKeyword("text-align", true, "start", func(v string) bool {
		return v == "start" || v == "end" || v == "left" || v == "right" || v == "center" || v == "justify"
	}, func(v string) {
		if v == "inherit" {
			computed.TextAlign = parent.TextAlign
			return
		}
		computed.TextAlign = map[string]TextAlign{"start": TextAlignStart, "end": TextAlignEnd, "left": TextAlignLeft, "right": TextAlignRight, "center": TextAlignCenter, "justify": TextAlignJustify}[v]
	})
	applyKeyword("text-transform", true, "none", func(v string) bool {
		return v == "none" || v == "uppercase" || v == "lowercase" || v == "capitalize"
	}, func(v string) {
		if v == "inherit" {
			computed.TextTransform = parent.TextTransform
			return
		}
		computed.TextTransform = map[string]TextTransform{"none": TextTransformNone, "uppercase": TextTransformUppercase, "lowercase": TextTransformLowercase, "capitalize": TextTransformCapitalize}[v]
	})
	applyKeyword("word-break", true, "normal", func(v string) bool { return v == "normal" || v == "break-all" || v == "keep-all" }, func(v string) {
		if v == "inherit" {
			computed.WordBreak = parent.WordBreak
			return
		}
		computed.WordBreak = map[string]WordBreak{"normal": WordBreakNormal, "break-all": WordBreakBreakAll, "keep-all": WordBreakKeepAll}[v]
	})
	applyKeyword("overflow-wrap", true, "normal", func(v string) bool { return v == "normal" || v == "break-word" || v == "anywhere" }, func(v string) {
		if v == "inherit" {
			computed.OverflowWrap = parent.OverflowWrap
			return
		}
		computed.OverflowWrap = map[string]OverflowWrap{"normal": OverflowWrapNormal, "break-word": OverflowWrapBreakWord, "anywhere": OverflowWrapAnywhere}[v]
	})
	applyKeyword("text-overflow", false, "clip", func(v string) bool { return v == "clip" || v == "ellipsis" }, func(v string) {
		if v == "inherit" {
			computed.TextOverflow = parent.TextOverflow
			return
		}
		if v == "ellipsis" {
			computed.TextOverflow = TextOverflowEllipsis
		} else {
			computed.TextOverflow = TextOverflowClip
		}
	})

	if candidate, ok := winners["text-indent"]; ok {
		if resolved, valid := resolveVariables(candidate.value, computed.CustomProperties); valid {
			switch parseGlobalKeyword(resolved) {
			case globalInherit, globalUnset:
				computed.TextIndent = parent.TextIndent
			case globalInitial:
				computed.TextIndent = LengthPercentage{}
			default:
				context.PercentageBase = 0
				if parsed, valid := ResolveLength(resolved, context); valid {
					computed.TextIndent = parsed
				}
			}
		}
	}
	applySpacing := func(property string, target *float32, parentValue float32) {
		candidate, ok := winners[property]
		if !ok {
			return
		}
		resolved, valid := resolveVariables(candidate.value, computed.CustomProperties)
		if !valid {
			return
		}
		switch parseGlobalKeyword(resolved) {
		case globalInherit, globalUnset:
			*target = parentValue
		case globalInitial:
			*target = 0
		default:
			if strings.EqualFold(strings.TrimSpace(resolved), "normal") {
				*target = 0
				return
			}
			if parsed, valid := ResolveLength(resolved, context); valid && parsed.Percentage == 0 {
				*target = parsed.Pixels
			}
		}
	}
	applySpacing("letter-spacing", &computed.LetterSpacing, parent.LetterSpacing)
	applySpacing("word-spacing", &computed.WordSpacing, parent.WordSpacing)

	if candidate, ok := winners["vertical-align"]; ok {
		if resolved, valid := resolveVariables(candidate.value, computed.CustomProperties); valid {
			computed.VerticalAlign = parseVerticalAlign(resolved, parent.VerticalAlign, context)
		}
	}
	return computed
}

func parseVerticalAlign(value string, parent VerticalAlign, context LengthContext) VerticalAlign {
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent
	case globalInitial, globalUnset:
		return VerticalAlign{}
	}
	lower := strings.ToLower(strings.TrimSpace(value))
	keywords := map[string]VerticalAlignKind{
		"baseline": VerticalAlignBaseline, "sub": VerticalAlignSub, "super": VerticalAlignSuper,
		"middle": VerticalAlignMiddle, "text-top": VerticalAlignTextTop, "text-bottom": VerticalAlignTextBottom,
		"top": VerticalAlignTop, "bottom": VerticalAlignBottom,
	}
	if kind, ok := keywords[lower]; ok {
		return VerticalAlign{Kind: kind}
	}
	if length, ok := ResolveLength(lower, context); ok && length.Percentage == 0 {
		return VerticalAlign{Kind: VerticalAlignLength, Value: length.Pixels}
	}
	return VerticalAlign{}
}
