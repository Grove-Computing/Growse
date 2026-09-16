package style

import "strings"

func applyWritingProperties(computed, parent ComputedStyle, winners map[string]winner, custom map[string]string) ComputedStyle {
	if value, ok := winnerValue(winners["writing-mode"], custom); ok {
		switch parseGlobalKeyword(value) {
		case globalInherit, globalUnset:
			computed.WritingMode = parent.WritingMode
		case globalInitial:
			computed.WritingMode = WritingModeHorizontalTB
		default:
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "horizontal-tb":
				computed.WritingMode = WritingModeHorizontalTB
			case "vertical-rl":
				computed.WritingMode = WritingModeVerticalRL
			case "vertical-lr":
				computed.WritingMode = WritingModeVerticalLR
			}
		}
	}
	if value, ok := winnerValue(winners["direction"], custom); ok {
		switch parseGlobalKeyword(value) {
		case globalInherit, globalUnset:
			computed.Direction = parent.Direction
		case globalInitial:
			computed.Direction = DirectionLTR
		default:
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "ltr":
				computed.Direction = DirectionLTR
			case "rtl":
				computed.Direction = DirectionRTL
			}
		}
	}
	return computed
}
