package style

import (
	"strconv"
	"strings"
)

func applyGridProperties(computed, parent ComputedStyle, winners map[string]winner, custom map[string]string, context LengthContext) ComputedStyle {
	computed.GridTemplateColumns = resolveTrackListWinner(computed.GridTemplateColumns, parent.GridTemplateColumns, winners["grid-template-columns"], custom, context, true)
	computed.GridTemplateRows = resolveTrackListWinner(computed.GridTemplateRows, parent.GridTemplateRows, winners["grid-template-rows"], custom, context, true)
	computed.GridAutoColumns = resolveTrackListWinner(computed.GridAutoColumns, parent.GridAutoColumns, winners["grid-auto-columns"], custom, context, false)
	computed.GridAutoRows = resolveTrackListWinner(computed.GridAutoRows, parent.GridAutoRows, winners["grid-auto-rows"], custom, context, false)
	return computed
}

func resolveTrackListWinner(current, parent []GridTrackSize, candidate winner, custom map[string]string, context LengthContext, allowNone bool) []GridTrackSize {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current
	}
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return append([]GridTrackSize(nil), parent...)
	case globalInitial, globalUnset:
		return nil
	}
	tracks, valid := parseGridTrackList(value, context, allowNone)
	if !valid {
		return current
	}
	return tracks
}

func parseGridTrackList(value string, context LengthContext, allowNone bool) ([]GridTrackSize, bool) {
	value = strings.TrimSpace(value)
	if allowNone && strings.EqualFold(value, "none") {
		return nil, true
	}
	parts, ok := splitCSSSpaceSeparated(value)
	if !ok || len(parts) == 0 {
		return nil, false
	}
	tracks := make([]GridTrackSize, 0, len(parts))
	for _, part := range parts {
		if repeated, matched, valid := parseGridRepeat(part, context); matched {
			if !valid {
				return nil, false
			}
			tracks = append(tracks, repeated...)
			continue
		}
		track, valid := parseGridTrackSize(part, context)
		if !valid {
			return nil, false
		}
		tracks = append(tracks, track)
	}
	return tracks, true
}

func parseGridTrackSize(value string, context LengthContext) (GridTrackSize, bool) {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "minmax(") && strings.HasSuffix(value, ")") {
		arguments, valid := splitGridArguments(value[len("minmax(") : len(value)-1])
		if !valid || len(arguments) != 2 {
			return GridTrackSize{}, false
		}
		minimum, minValid := parseGridTrackSize(arguments[0], context)
		maximum, maxValid := parseGridTrackSize(arguments[1], context)
		if !minValid || !maxValid || minimum.Kind == GridTrackFraction || minimum.MinSet || maximum.MinSet {
			return GridTrackSize{}, false
		}
		maximum.MinKind, maximum.MinValue, maximum.MinSet = minimum.Kind, minimum.Value, true
		return maximum, true
	}
	if strings.HasPrefix(lower, "fit-content(") && strings.HasSuffix(value, ")") {
		limit, valid := ResolveLength(value[len("fit-content("):len(value)-1], context)
		if !valid || limit.Pixels < 0 || limit.Percentage < 0 {
			return GridTrackSize{}, false
		}
		return GridTrackSize{Kind: GridTrackMaxContent, MinKind: GridTrackMinContent, MinSet: true, FitLimit: &limit}, true
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto":
		return GridTrackSize{Kind: GridTrackAuto}, true
	case "min-content":
		return GridTrackSize{Kind: GridTrackMinContent}, true
	case "max-content":
		return GridTrackSize{Kind: GridTrackMaxContent}, true
	}
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(value)), "fr") {
		number := strings.TrimSpace(value[:len(strings.TrimSpace(value))-2])
		fraction, err := strconv.ParseFloat(number, 32)
		if err == nil && fraction >= 0 {
			return GridTrackSize{Kind: GridTrackFraction, Flex: float32(fraction)}, true
		}
		return GridTrackSize{}, false
	}
	length, valid := ResolveLength(value, context)
	if valid && length.Pixels >= 0 && length.Percentage >= 0 {
		return GridTrackSize{Kind: GridTrackLength, Value: length}, true
	}
	return GridTrackSize{}, false
}

func parseGridRepeat(value string, context LengthContext) ([]GridTrackSize, bool, bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(strings.ToLower(value), "repeat(") || !strings.HasSuffix(value, ")") {
		return nil, false, false
	}
	arguments, valid := splitGridArguments(value[len("repeat(") : len(value)-1])
	if !valid || len(arguments) != 2 {
		return nil, true, false
	}
	count, err := strconv.Atoi(strings.TrimSpace(arguments[0]))
	if err != nil || count < 1 || count > 1000 {
		return nil, true, false
	}
	pattern, valid := parseGridTrackList(arguments[1], context, false)
	if !valid || len(pattern) == 0 || len(pattern) > 1000/count {
		return nil, true, false
	}
	result := make([]GridTrackSize, 0, count*len(pattern))
	for index := 0; index < count; index++ {
		result = append(result, pattern...)
	}
	return result, true, true
}

func splitGridArguments(value string) ([]string, bool) {
	depth, start := 0, 0
	var result []string
	for index, character := range value {
		switch character {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return nil, false
			}
		case ',':
			if depth == 0 {
				result = append(result, strings.TrimSpace(value[start:index]))
				start = index + 1
			}
		}
	}
	if depth != 0 {
		return nil, false
	}
	result = append(result, strings.TrimSpace(value[start:]))
	for _, item := range result {
		if item == "" {
			return nil, false
		}
	}
	return result, true
}
