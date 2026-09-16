package style

import (
	"strconv"
	"strings"
)

const maxComputedColumnCount = 32

func applyColumnProperties(computed, parent ComputedStyle, winners map[string]winner, custom map[string]string, context LengthContext) ComputedStyle {
	computed.ColumnCount = resolveColumnCount(computed.ColumnCount, parent.ColumnCount, winners["column-count"], custom)
	computed.ColumnWidth = resolveColumnWidth(computed.ColumnWidth, parent.ColumnWidth, winners["column-width"], custom, context)
	computed.ColumnGapNormal = resolveColumnGapNormal(computed.ColumnGapNormal, parent.ColumnGapNormal, winners["column-gap"], custom)
	computed.ColumnRule = resolveColumnRule(computed.ColumnRule, parent.ColumnRule, winners, custom, context, computed.Color)
	computed.ColumnFill = resolveColumnFill(computed.ColumnFill, parent.ColumnFill, winners["column-fill"], custom)
	computed.ColumnSpan = resolveColumnSpan(computed.ColumnSpan, parent.ColumnSpan, winners["column-span"], custom)
	computed.BreakBefore = resolveFragmentBreak(computed.BreakBefore, parent.BreakBefore, winners["break-before"], custom, false)
	computed.BreakAfter = resolveFragmentBreak(computed.BreakAfter, parent.BreakAfter, winners["break-after"], custom, false)
	computed.BreakInside = resolveFragmentBreak(computed.BreakInside, parent.BreakInside, winners["break-inside"], custom, true)
	computed.Widows = resolveFragmentLineCount(computed.Widows, parent.Widows, winners["widows"], custom)
	computed.Orphans = resolveFragmentLineCount(computed.Orphans, parent.Orphans, winners["orphans"], custom)
	return computed
}

func resolveColumnCount(current, parent int, candidate winner, custom map[string]string) int {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current
	}
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent
	case globalInitial, globalUnset:
		return 0
	}
	if candidate.source == "columns" {
		value, ok = columnShorthandComponent(value, true)
		if !ok {
			return current
		}
	}
	if strings.EqualFold(strings.TrimSpace(value), "auto") {
		return 0
	}
	count, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || count < 1 {
		return current
	}
	return min(count, maxComputedColumnCount)
}

func resolveColumnWidth(current, parent SizeValue, candidate winner, custom map[string]string, context LengthContext) SizeValue {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current
	}
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent
	case globalInitial, globalUnset:
		return SizeValue{Kind: SizeAuto}
	}
	if candidate.source == "columns" {
		value, ok = columnShorthandComponent(value, false)
		if !ok {
			return current
		}
	}
	if strings.EqualFold(strings.TrimSpace(value), "auto") {
		return SizeValue{Kind: SizeAuto}
	}
	length, valid := ResolveLength(value, context)
	if !valid || length.Pixels < 0 || length.Percentage < 0 || length == (LengthPercentage{}) {
		return current
	}
	return SizeValue{Kind: SizeLength, Value: length}
}

func columnShorthandComponent(value string, wantCount bool) (string, bool) {
	parts, ok := splitCSSSpaceSeparated(value)
	if !ok || len(parts) < 1 || len(parts) > 2 {
		return "", false
	}
	count, width := "auto", "auto"
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.EqualFold(part, "auto") {
			continue
		}
		if parsed, err := strconv.Atoi(part); err == nil && parsed > 0 {
			if count != "auto" {
				return "", false
			}
			count = part
			continue
		}
		if length, valid := ResolveLength(part, LengthContext{FontSize: 16, RootFontSize: 16, ViewportWidth: 1280, ViewportHeight: 720}); valid &&
			(length.Pixels > 0 || length.Percentage > 0) {
			if width != "auto" {
				return "", false
			}
			width = part
			continue
		}
		return "", false
	}
	if wantCount {
		return count, true
	}
	return width, true
}

func resolveColumnGapNormal(current, parent bool, candidate winner, custom map[string]string) bool {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current
	}
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent
	case globalInitial, globalUnset:
		return true
	}
	if candidate.source == "gap" {
		parts, valid := splitCSSSpaceSeparated(value)
		if !valid || len(parts) < 1 || len(parts) > 2 {
			return current
		}
		value = parts[0]
		if len(parts) == 2 {
			value = parts[1]
		}
	}
	return strings.EqualFold(strings.TrimSpace(value), "normal")
}

func resolveColumnRule(current, parent BorderSide, winners map[string]winner, custom map[string]string, context LengthContext, currentColor uint32) BorderSide {
	result := current
	for _, component := range []string{"width", "style", "color"} {
		candidate, ok := winners["column-rule-"+component]
		if !ok {
			continue
		}
		value, ok := winnerValue(candidate, custom)
		if !ok {
			continue
		}
		switch parseGlobalKeyword(value) {
		case globalInherit:
			setBorderComponent(&result, component, parent)
			continue
		case globalInitial, globalUnset:
			setBorderComponent(&result, component, BorderSide{Color: currentColor})
			continue
		}
		if candidate.source == "column-rule" {
			var valid bool
			value, valid = borderComponentValue("border", component, 0, value)
			if !valid {
				continue
			}
		}
		switch component {
		case "width":
			if parsed, valid := parseBorderWidth(value, context); valid {
				result.Width = parsed
			}
		case "style":
			if parsed, valid := parseBorderStyle(value); valid {
				result.Style = parsed
			}
		case "color":
			if parsed, valid := parseColor(value, currentColor); valid {
				result.Color = parsed
			}
		}
	}
	if result.Color == 0 {
		result.Color = currentColor
	}
	return result
}

func resolveColumnFill(current, parent ColumnFill, candidate winner, custom map[string]string) ColumnFill {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current
	}
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent
	case globalInitial, globalUnset:
		return ColumnFillBalance
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "balance":
		return ColumnFillBalance
	case "auto":
		return ColumnFillAuto
	default:
		return current
	}
}

func resolveColumnSpan(current, parent ColumnSpan, candidate winner, custom map[string]string) ColumnSpan {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current
	}
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent
	case globalInitial, globalUnset:
		return ColumnSpanNone
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "all":
		return ColumnSpanAll
	case "none":
		return ColumnSpanNone
	default:
		return current
	}
}

func resolveFragmentBreak(current, parent FragmentBreak, candidate winner, custom map[string]string, inside bool) FragmentBreak {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current
	}
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent
	case globalInitial, globalUnset:
		return FragmentBreakAuto
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto":
		return FragmentBreakAuto
	case "avoid":
		return FragmentBreakAvoid
	case "avoid-column":
		return FragmentBreakAvoidColumn
	case "column":
		if !inside {
			return FragmentBreakColumn
		}
	}
	return current
}

func resolveFragmentLineCount(current, parent int, candidate winner, custom map[string]string) int {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current
	}
	switch parseGlobalKeyword(value) {
	case globalInherit, globalUnset:
		return parent
	case globalInitial:
		return 2
	}
	count, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || count < 1 {
		return current
	}
	return min(count, 32)
}
