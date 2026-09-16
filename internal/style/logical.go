package style

import "strings"

type physicalSide uint8

const (
	physicalTop physicalSide = iota
	physicalRight
	physicalBottom
	physicalLeft
)

type logicalAxes struct {
	inlineStart, inlineEnd physicalSide
	blockStart, blockEnd   physicalSide
}

func axesForWritingMode(writingMode WritingMode, direction Direction) logicalAxes {
	inlineStart, inlineEnd := physicalLeft, physicalRight
	if direction == DirectionRTL {
		inlineStart, inlineEnd = inlineEnd, inlineStart
	}
	if writingMode == WritingModeHorizontalTB {
		return logicalAxes{inlineStart: inlineStart, inlineEnd: inlineEnd, blockStart: physicalTop, blockEnd: physicalBottom}
	}
	inlineStart, inlineEnd = physicalTop, physicalBottom
	if direction == DirectionRTL {
		inlineStart, inlineEnd = inlineEnd, inlineStart
	}
	if writingMode == WritingModeVerticalRL {
		return logicalAxes{inlineStart: inlineStart, inlineEnd: inlineEnd, blockStart: physicalRight, blockEnd: physicalLeft}
	}
	return logicalAxes{inlineStart: inlineStart, inlineEnd: inlineEnd, blockStart: physicalLeft, blockEnd: physicalRight}
}

func (side physicalSide) name() string {
	return [...]string{"top", "right", "bottom", "left"}[side]
}

func resolveLogicalWinners(source map[string]winner, writingMode WritingMode, direction Direction, custom map[string]string) map[string]winner {
	result := make(map[string]winner, len(source))
	for property, candidate := range source {
		if !isLogicalProperty(property) {
			result[property] = candidate
		}
	}
	axes := axesForWritingMode(writingMode, direction)
	add := func(property string, candidate winner) {
		candidate.source = property
		if current, exists := result[property]; exists {
			if selected, ok := selectCascadeWinner([]winner{current, candidate}); ok {
				result[property] = selected
			}
			return
		}
		result[property] = candidate
	}
	addPair := func(startProperty, endProperty string, candidate winner) {
		value, valid := winnerValue(candidate, custom)
		if !valid {
			return
		}
		if parseGlobalKeyword(value) != globalNone {
			start, end := candidate, candidate
			start.value, end.value = value, value
			add(startProperty, start)
			add(endProperty, end)
			return
		}
		parts, valid := splitCSSSpaceSeparated(value)
		if !valid || len(parts) == 0 || len(parts) > 2 {
			return
		}
		start, end := candidate, candidate
		start.value, end.value = parts[0], parts[0]
		if len(parts) == 2 {
			end.value = parts[1]
		}
		add(startProperty, start)
		add(endProperty, end)
	}
	addBorderSide := func(side physicalSide, candidate winner) {
		for _, component := range []string{"width", "style", "color"} {
			target := "border-" + side.name() + "-" + component
			mapped := candidate
			mapped.source = "border-" + side.name()
			if current, exists := result[target]; exists {
				if selected, ok := selectCascadeWinner([]winner{current, mapped}); ok {
					result[target] = selected
				}
			} else {
				result[target] = mapped
			}
		}
	}
	addBorderPair := func(startSide, endSide physicalSide, component string, candidate winner) {
		value, valid := winnerValue(candidate, custom)
		if !valid {
			return
		}
		if parseGlobalKeyword(value) != globalNone {
			start, end := candidate, candidate
			start.value, end.value = value, value
			add("border-"+startSide.name()+"-"+component, start)
			add("border-"+endSide.name()+"-"+component, end)
			return
		}
		parts, valid := splitCSSSpaceSeparated(value)
		if !valid || len(parts) == 0 || len(parts) > 2 {
			return
		}
		start, end := candidate, candidate
		start.value, end.value = parts[0], parts[0]
		if len(parts) == 2 {
			end.value = parts[1]
		}
		add("border-"+startSide.name()+"-"+component, start)
		add("border-"+endSide.name()+"-"+component, end)
	}

	for property, candidate := range source {
		switch property {
		case "margin-block", "padding-block":
			prefix := strings.TrimSuffix(property, "-block")
			addPair(prefix+"-"+axes.blockStart.name(), prefix+"-"+axes.blockEnd.name(), candidate)
		case "margin-inline", "padding-inline":
			prefix := strings.TrimSuffix(property, "-inline")
			addPair(prefix+"-"+axes.inlineStart.name(), prefix+"-"+axes.inlineEnd.name(), candidate)
		case "margin-block-start", "padding-block-start", "margin-block-end", "padding-block-end",
			"margin-inline-start", "padding-inline-start", "margin-inline-end", "padding-inline-end":
			prefix, side := logicalEdgeTarget(property, axes)
			add(prefix+"-"+side.name(), candidate)
		case "border-block", "border-inline":
			start, end := axes.blockStart, axes.blockEnd
			if property == "border-inline" {
				start, end = axes.inlineStart, axes.inlineEnd
			}
			addBorderSide(start, candidate)
			addBorderSide(end, candidate)
		case "border-block-width", "border-block-style", "border-block-color",
			"border-inline-width", "border-inline-style", "border-inline-color":
			parts := strings.Split(property, "-")
			start, end := axes.blockStart, axes.blockEnd
			if parts[1] == "inline" {
				start, end = axes.inlineStart, axes.inlineEnd
			}
			addBorderPair(start, end, parts[2], candidate)
		case "border-block-start", "border-block-end", "border-inline-start", "border-inline-end":
			addBorderSide(logicalBorderSide(property, axes), candidate)
		case "border-block-start-width", "border-block-start-style", "border-block-start-color",
			"border-block-end-width", "border-block-end-style", "border-block-end-color",
			"border-inline-start-width", "border-inline-start-style", "border-inline-start-color",
			"border-inline-end-width", "border-inline-end-style", "border-inline-end-color":
			parts := strings.Split(property, "-")
			side := logicalBorderSide(strings.Join(parts[:3], "-"), axes)
			add("border-"+side.name()+"-"+parts[3], candidate)
		case "border-start-start-radius", "border-start-end-radius", "border-end-start-radius", "border-end-end-radius":
			blockStart := strings.HasPrefix(property, "border-start-")
			inlineStart := strings.Contains(strings.TrimSuffix(property, "-radius"), "-start-start") || strings.Contains(property, "-end-start-")
			blockSide, inlineSide := axes.blockEnd, axes.inlineEnd
			if blockStart {
				blockSide = axes.blockStart
			}
			if inlineStart {
				inlineSide = axes.inlineStart
			}
			add("border-"+cornerName(blockSide, inlineSide)+"-radius", candidate)
		case "inset-block":
			addPair(axes.blockStart.name(), axes.blockEnd.name(), candidate)
		case "inset-inline":
			addPair(axes.inlineStart.name(), axes.inlineEnd.name(), candidate)
		case "inset-block-start":
			add(axes.blockStart.name(), candidate)
		case "inset-block-end":
			add(axes.blockEnd.name(), candidate)
		case "inset-inline-start":
			add(axes.inlineStart.name(), candidate)
		case "inset-inline-end":
			add(axes.inlineEnd.name(), candidate)
		case "inline-size", "min-inline-size", "max-inline-size":
			add(logicalSizeTarget(property, writingMode == WritingModeHorizontalTB), candidate)
		case "block-size", "min-block-size", "max-block-size":
			add(logicalSizeTarget(property, writingMode != WritingModeHorizontalTB), candidate)
		}
	}
	return result
}

func isLogicalProperty(property string) bool {
	return strings.HasPrefix(property, "margin-block") || strings.HasPrefix(property, "margin-inline") ||
		strings.HasPrefix(property, "padding-block") || strings.HasPrefix(property, "padding-inline") ||
		strings.HasPrefix(property, "border-block") || strings.HasPrefix(property, "border-inline") ||
		strings.HasPrefix(property, "border-start-") || strings.HasPrefix(property, "border-end-") ||
		strings.HasPrefix(property, "inset-block") || strings.HasPrefix(property, "inset-inline") ||
		property == "inline-size" || property == "block-size" || strings.HasSuffix(property, "-inline-size") || strings.HasSuffix(property, "-block-size")
}

func logicalEdgeTarget(property string, axes logicalAxes) (string, physicalSide) {
	prefix := strings.Split(property, "-")[0]
	switch {
	case strings.Contains(property, "block-start"):
		return prefix, axes.blockStart
	case strings.Contains(property, "block-end"):
		return prefix, axes.blockEnd
	case strings.Contains(property, "inline-start"):
		return prefix, axes.inlineStart
	default:
		return prefix, axes.inlineEnd
	}
}

func logicalBorderSide(property string, axes logicalAxes) physicalSide {
	switch property {
	case "border-block-start":
		return axes.blockStart
	case "border-block-end":
		return axes.blockEnd
	case "border-inline-start":
		return axes.inlineStart
	default:
		return axes.inlineEnd
	}
}

func logicalSizeTarget(property string, horizontal bool) string {
	prefix := strings.TrimSuffix(strings.TrimSuffix(property, "inline-size"), "block-size")
	if horizontal {
		return prefix + "width"
	}
	return prefix + "height"
}

func cornerName(first, second physicalSide) string {
	top := first == physicalTop || second == physicalTop
	left := first == physicalLeft || second == physicalLeft
	vertical := "bottom"
	if top {
		vertical = "top"
	}
	horizontal := "right"
	if left {
		horizontal = "left"
	}
	return vertical + "-" + horizontal
}
