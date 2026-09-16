package style

import "strings"

// applyTableProperties resolves the table formatting properties consumed by
// layout. border-collapse, border-spacing, and caption-side are inherited by
// internal table boxes; table-layout is reset for every element.
func applyTableProperties(computed, parent ComputedStyle, winners map[string]winner, custom map[string]string, context LengthContext) ComputedStyle {
	computed.TableLayout = resolveTableLayout(computed.TableLayout, parent.TableLayout, winners["table-layout"], custom)
	computed.BorderCollapse = resolveBorderCollapse(computed.BorderCollapse, parent.BorderCollapse, winners["border-collapse"], custom)
	computed.BorderSpacingX, computed.BorderSpacingY = resolveBorderSpacing(
		computed.BorderSpacingX, computed.BorderSpacingY,
		parent.BorderSpacingX, parent.BorderSpacingY,
		winners["border-spacing"], custom, context,
	)
	computed.CaptionSide = resolveCaptionSide(computed.CaptionSide, parent.CaptionSide, winners["caption-side"], custom)
	return computed
}

func resolveTableLayout(current, parent TableLayout, candidate winner, custom map[string]string) TableLayout {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current
	}
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent
	case globalInitial, globalUnset:
		return TableLayoutAuto
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto":
		return TableLayoutAuto
	case "fixed":
		return TableLayoutFixed
	default:
		return current
	}
}

func resolveBorderCollapse(current, parent BorderCollapse, candidate winner, custom map[string]string) BorderCollapse {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current
	}
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent
	case globalInitial:
		return BorderCollapseSeparate
	case globalUnset:
		return parent
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "separate":
		return BorderCollapseSeparate
	case "collapse":
		return BorderCollapseCollapse
	default:
		return current
	}
}

func resolveBorderSpacing(currentX, currentY, parentX, parentY float32, candidate winner, custom map[string]string, context LengthContext) (float32, float32) {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return currentX, currentY
	}
	switch parseGlobalKeyword(value) {
	case globalInherit, globalUnset:
		return parentX, parentY
	case globalInitial:
		return 0, 0
	}
	parts, ok := splitCSSSpaceSeparated(value)
	if !ok || len(parts) < 1 || len(parts) > 2 {
		return currentX, currentY
	}
	horizontal, valid := ResolveLength(parts[0], context)
	if !valid || horizontal.Percentage != 0 || horizontal.Pixels < 0 {
		return currentX, currentY
	}
	vertical := horizontal
	if len(parts) == 2 {
		vertical, valid = ResolveLength(parts[1], context)
		if !valid || vertical.Percentage != 0 || vertical.Pixels < 0 {
			return currentX, currentY
		}
	}
	return horizontal.Pixels, vertical.Pixels
}

func resolveCaptionSide(current, parent CaptionSide, candidate winner, custom map[string]string) CaptionSide {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current
	}
	switch parseGlobalKeyword(value) {
	case globalInherit, globalUnset:
		return parent
	case globalInitial:
		return CaptionSideTop
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "top":
		return CaptionSideTop
	case "bottom":
		return CaptionSideBottom
	default:
		return current
	}
}
