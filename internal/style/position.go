package style

import (
	"strconv"
	"strings"
)

func applyPositionProperties(computed, parent ComputedStyle, winners map[string]winner, custom map[string]string, context LengthContext) ComputedStyle {
	if candidate, ok := winners["position"]; ok {
		if value, valid := winnerValue(candidate, custom); valid {
			switch parseGlobalKeyword(value) {
			case globalInherit:
				computed.Position = parent.Position
			case globalInitial, globalUnset:
				computed.Position = PositionStatic
			default:
				switch strings.ToLower(strings.TrimSpace(value)) {
				case "static":
					computed.Position = PositionStatic
				case "relative":
					computed.Position = PositionRelative
				case "absolute":
					computed.Position = PositionAbsolute
				case "fixed":
					computed.Position = PositionFixed
				case "sticky":
					computed.Position = PositionSticky
				}
			}
		}
	}
	properties := []string{"top", "right", "bottom", "left"}
	values := []*SizeValue{&computed.Inset.Top, &computed.Inset.Right, &computed.Inset.Bottom, &computed.Inset.Left}
	parentValues := []SizeValue{parent.Inset.Top, parent.Inset.Right, parent.Inset.Bottom, parent.Inset.Left}
	for index, property := range properties {
		candidate, ok := winners[property]
		if !ok {
			continue
		}
		value, valid := winnerValue(candidate, custom)
		if !valid {
			continue
		}
		switch parseGlobalKeyword(value) {
		case globalInherit:
			*values[index] = parentValues[index]
		case globalInitial, globalUnset:
			*values[index] = SizeValue{Kind: SizeAuto}
		default:
			component, valid := insetShorthandComponent(candidate.source, value, index)
			if !valid {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(component), "auto") {
				*values[index] = SizeValue{Kind: SizeAuto}
			} else if length, valid := ResolveLength(component, context); valid {
				*values[index] = SizeValue{Kind: SizeLength, Value: length}
			}
		}
	}
	if candidate, ok := winners["z-index"]; ok {
		if value, valid := winnerValue(candidate, custom); valid {
			switch parseGlobalKeyword(value) {
			case globalInherit:
				computed.ZIndex, computed.ZIndexAuto = parent.ZIndex, parent.ZIndexAuto
			case globalInitial, globalUnset:
				computed.ZIndex, computed.ZIndexAuto = 0, true
			default:
				if strings.EqualFold(strings.TrimSpace(value), "auto") {
					computed.ZIndex, computed.ZIndexAuto = 0, true
				} else if index, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
					computed.ZIndex, computed.ZIndexAuto = index, false
				}
			}
		}
	}
	return computed
}

func insetShorthandComponent(source, value string, side int) (string, bool) {
	parts, valid := splitCSSSpaceSeparated(value)
	if !valid || len(parts) == 0 {
		return "", false
	}
	switch source {
	case "inset":
		if len(parts) > 4 {
			return "", false
		}
		return edgePart(parts, side), true
	case "inset-block":
		if len(parts) > 2 || side != 0 && side != 2 {
			return "", false
		}
		if side == 2 && len(parts) == 2 {
			return parts[1], true
		}
		return parts[0], true
	case "inset-inline":
		if len(parts) > 2 || side != 1 && side != 3 {
			return "", false
		}
		if side == 1 && len(parts) == 2 {
			return parts[1], true
		}
		return parts[0], true
	default:
		return value, true
	}
}
