package style

import (
	"strconv"
	"strings"

	"github.com/saku0512/growse/internal/css"
)

func applyGridProperties(computed, parent ComputedStyle, winners map[string]winner, custom map[string]string, context LengthContext) ComputedStyle {
	computed.GridTemplateColumns = resolveTrackListWinner(computed.GridTemplateColumns, parent.GridTemplateColumns, winners["grid-template-columns"], custom, context, true)
	computed.GridTemplateRows = resolveTrackListWinner(computed.GridTemplateRows, parent.GridTemplateRows, winners["grid-template-rows"], custom, context, true)
	computed.GridAutoColumns = resolveTrackListWinner(computed.GridAutoColumns, parent.GridAutoColumns, winners["grid-auto-columns"], custom, context, false)
	computed.GridAutoRows = resolveTrackListWinner(computed.GridAutoRows, parent.GridAutoRows, winners["grid-auto-rows"], custom, context, false)
	computed.GridColumnLines = resolveNamedLines(computed.GridColumnLines, parent.GridColumnLines, winners["grid-template-columns"], custom)
	computed.GridRowLines = resolveNamedLines(computed.GridRowLines, parent.GridRowLines, winners["grid-template-rows"], custom)
	computed.GridTemplateAreas = resolveTemplateAreas(computed.GridTemplateAreas, parent.GridTemplateAreas, winners["grid-template-areas"], custom)
	computed.GridColumn = resolveGridPlacement(computed.GridColumn, parent.GridColumn, winners, "grid-column", custom)
	computed.GridRow = resolveGridPlacement(computed.GridRow, parent.GridRow, winners, "grid-row", custom)
	computed.GridAutoFlow = resolveGridAutoFlow(computed.GridAutoFlow, parent.GridAutoFlow, winners["grid-auto-flow"], custom)
	if candidate, ok := winners["grid-area"]; ok {
		if value, valid := winnerValue(candidate, custom); valid && !strings.Contains(value, "/") && parseGlobalKeyword(value) == globalNone {
			computed.GridAreaName = strings.TrimSpace(value)
		}
	}
	return computed
}

func resolveGridAutoFlow(current, parent GridAutoFlow, candidate winner, custom map[string]string) GridAutoFlow {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current
	}
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent
	case globalInitial, globalUnset:
		return GridAutoFlow{}
	}
	result := GridAutoFlow{}
	for _, field := range strings.Fields(strings.ToLower(value)) {
		switch field {
		case "row":
			result.Column = false
		case "column":
			result.Column = true
		case "dense":
			result.Dense = true
		default:
			return current
		}
	}
	return result
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
		if strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]") {
			continue
		}
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

func resolveNamedLines(current, parent map[string][]int, candidate winner, custom map[string]string) map[string][]int {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current
	}
	if parseGlobalKeyword(value) == globalInherit {
		return cloneLineMap(parent)
	}
	if parseGlobalKeyword(value) != globalNone {
		return nil
	}
	parts, valid := splitCSSSpaceSeparated(value)
	if !valid {
		return current
	}
	result, line := make(map[string][]int), 0
	for _, part := range parts {
		if strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]") {
			for _, name := range strings.Fields(part[1 : len(part)-1]) {
				result[name] = append(result[name], line)
			}
			continue
		}
		if repeated, matched, valid := parseGridRepeat(part, LengthContext{}); matched && valid {
			line += len(repeated)
		} else {
			line++
		}
	}
	return result
}

func cloneLineMap(source map[string][]int) map[string][]int {
	if source == nil {
		return nil
	}
	result := make(map[string][]int, len(source))
	for name, lines := range source {
		result[name] = append([]int(nil), lines...)
	}
	return result
}

func resolveTemplateAreas(current, parent map[string]GridArea, candidate winner, custom map[string]string) map[string]GridArea {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current
	}
	if parseGlobalKeyword(value) == globalInherit {
		result := make(map[string]GridArea, len(parent))
		for name, area := range parent {
			result[name] = area
		}
		return result
	}
	if strings.EqualFold(strings.TrimSpace(value), "none") || parseGlobalKeyword(value) != globalNone {
		return nil
	}
	parts, valid := splitCSSSpaceSeparated(value)
	if !valid {
		return current
	}
	result := make(map[string]GridArea)
	rowIndex, columns := 0, -1
	for _, part := range parts {
		row, decoded := css.DecodeString(part)
		if !decoded {
			return current
		}
		cells := strings.Fields(row)
		if len(cells) == 0 || columns >= 0 && len(cells) != columns {
			return current
		}
		columns = len(cells)
		for column, name := range cells {
			if name == "." {
				continue
			}
			area, exists := result[name]
			if !exists {
				area = GridArea{RowStart: rowIndex, RowEnd: rowIndex + 1, ColumnStart: column, ColumnEnd: column + 1}
			} else {
				area.RowEnd, area.ColumnEnd = rowIndex+1, max(area.ColumnEnd, column+1)
			}
			result[name] = area
		}
		rowIndex++
	}
	return result
}

func resolveGridPlacement(current, parent GridPlacement, winners map[string]winner, shorthand string, custom map[string]string) GridPlacement {
	if candidate, ok := winners[shorthand]; ok {
		if value, valid := winnerValue(candidate, custom); valid {
			parts := strings.Split(value, "/")
			if len(parts) <= 2 {
				current.Start = parseGridLine(parts[0])
				if len(parts) == 2 {
					current.End = parseGridLine(parts[1])
				}
			}
		}
	}
	for _, edge := range []struct {
		name string
		line *GridLine
		base GridLine
	}{{shorthand + "-start", &current.Start, parent.Start}, {shorthand + "-end", &current.End, parent.End}} {
		if candidate, ok := winners[edge.name]; ok {
			if value, valid := winnerValue(candidate, custom); valid {
				switch parseGlobalKeyword(value) {
				case globalInherit:
					*edge.line = edge.base
				case globalInitial, globalUnset:
					*edge.line = GridLine{}
				default:
					*edge.line = parseGridLine(value)
				}
			}
		}
	}
	return current
}

func parseGridLine(value string) GridLine {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 || strings.EqualFold(fields[0], "auto") {
		return GridLine{}
	}
	line := GridLine{}
	if strings.EqualFold(fields[0], "span") {
		line.Span = 1
		fields = fields[1:]
	}
	for _, field := range fields {
		if number, err := strconv.Atoi(field); err == nil {
			if line.Span > 0 {
				line.Span = max(number, 1)
			} else {
				line.Index = number
			}
		} else {
			line.Name = field
		}
	}
	return line
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
