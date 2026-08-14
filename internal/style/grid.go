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
		track, valid := parseGridTrackSize(part, context)
		if !valid {
			return nil, false
		}
		tracks = append(tracks, track)
	}
	return tracks, true
}

func parseGridTrackSize(value string, context LengthContext) (GridTrackSize, bool) {
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
