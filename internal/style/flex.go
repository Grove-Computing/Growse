package style

import (
	"math"
	"strconv"
	"strings"
)

func applyFlexProperties(computed, parent ComputedStyle, winners map[string]winner, customProperties map[string]string, context LengthContext) ComputedStyle {
	computed.FlexDirection = resolveFlexDirectionWinner(computed.FlexDirection, parent.FlexDirection, winners["flex-direction"], customProperties)
	computed.FlexWrap = resolveFlexWrapWinner(computed.FlexWrap, parent.FlexWrap, winners["flex-wrap"], customProperties)
	computed.JustifyContent, computed.JustifyContentSafety = resolveJustifyWinner(computed.JustifyContent, parent.JustifyContent, computed.JustifyContentSafety, parent.JustifyContentSafety, winners["justify-content"], customProperties)
	computed.AlignItems, computed.AlignItemsSafety = resolveAlignWinner(computed.AlignItems, parent.AlignItems, AlignStretch, computed.AlignItemsSafety, parent.AlignItemsSafety, false, winners["align-items"], customProperties)
	computed.JustifyItems, computed.JustifyItemsSafety = resolveAlignWinner(computed.JustifyItems, parent.JustifyItems, AlignStretch, computed.JustifyItemsSafety, parent.JustifyItemsSafety, false, justifyPlaceCandidate(winners["justify-items"]), customProperties)
	computed.AlignContent, computed.AlignContentSafety = resolveAlignWinner(computed.AlignContent, parent.AlignContent, AlignStretch, computed.AlignContentSafety, parent.AlignContentSafety, true, winners["align-content"], customProperties)
	computed.AlignSelf, computed.AlignSelfSafety = resolveAlignWinner(computed.AlignSelf, parent.AlignSelf, AlignAuto, computed.AlignSelfSafety, parent.AlignSelfSafety, false, winners["align-self"], customProperties)
	computed.JustifySelf, computed.JustifySelfSafety = resolveAlignWinner(computed.JustifySelf, parent.JustifySelf, AlignAuto, computed.JustifySelfSafety, parent.JustifySelfSafety, false, justifyPlaceCandidate(winners["justify-self"]), customProperties)
	computed.Order = resolveIntegerWinner(computed.Order, parent.Order, 0, winners["order"], customProperties)
	computed.FlexGrow = resolveFactorWinner("flex-grow", computed.FlexGrow, parent.FlexGrow, 0, winners["flex-grow"], customProperties)
	computed.FlexShrink = resolveFactorWinner("flex-shrink", computed.FlexShrink, parent.FlexShrink, 1, winners["flex-shrink"], customProperties)
	computed.FlexBasis = resolveBasisWinner(computed.FlexBasis, parent.FlexBasis, winners["flex-basis"], customProperties, context)
	computed.RowGap = resolveGapWinner("row-gap", computed.RowGap, parent.RowGap, winners["row-gap"], customProperties, context)
	computed.ColumnGap = resolveGapWinner("column-gap", computed.ColumnGap, parent.ColumnGap, winners["column-gap"], customProperties, context)
	computed.AspectRatio = resolveAspectRatioWinner(computed.AspectRatio, parent.AspectRatio, winners["aspect-ratio"], customProperties)
	return computed
}

func resolveFlexDirectionWinner(current, parent FlexDirection, candidate winner, custom map[string]string) FlexDirection {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current
	}
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent
	case globalInitial, globalUnset:
		return FlexDirectionRow
	}
	if candidate.source == "flex-flow" {
		value, ok = flexFlowComponent(value, true)
		if !ok {
			return current
		}
	}
	if parsed, valid := parseFlexDirection(value); valid {
		return parsed
	}
	return current
}

func resolveFlexWrapWinner(current, parent FlexWrap, candidate winner, custom map[string]string) FlexWrap {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current
	}
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent
	case globalInitial, globalUnset:
		return FlexNoWrap
	}
	if candidate.source == "flex-flow" {
		value, ok = flexFlowComponent(value, false)
		if !ok {
			return current
		}
	}
	if parsed, valid := parseFlexWrap(value); valid {
		return parsed
	}
	return current
}

func resolveJustifyWinner(current, parent JustifyContent, currentSafety, parentSafety OverflowAlignment, candidate winner, custom map[string]string) (JustifyContent, OverflowAlignment) {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current, currentSafety
	}
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent, parentSafety
	case globalInitial, globalUnset:
		return JustifyFlexStart, OverflowAlignmentDefault
	}
	value = placeShorthandComponent(candidate.source, value, false)
	value, safety, valid := parseOverflowAlignment(value)
	if !valid {
		return current, currentSafety
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "normal", "flex-start":
		return JustifyFlexStart, safety
	case "flex-end":
		return JustifyFlexEnd, safety
	case "start":
		return JustifyStart, safety
	case "end":
		return JustifyEnd, safety
	case "left":
		return JustifyLeft, safety
	case "right":
		return JustifyRight, safety
	case "center":
		return JustifyCenter, safety
	case "space-between":
		return JustifySpaceBetween, safety
	case "space-around":
		return JustifySpaceAround, safety
	case "space-evenly":
		return JustifySpaceEvenly, safety
	default:
		return current, currentSafety
	}
}

func resolveAlignWinner(current, parent, initial Align, currentSafety, parentSafety OverflowAlignment, distributed bool, candidate winner, custom map[string]string) (Align, OverflowAlignment) {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current, currentSafety
	}
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent, parentSafety
	case globalInitial, globalUnset:
		return initial, OverflowAlignmentDefault
	}
	value = placeShorthandComponent(candidate.source, value, true)
	parsed, safety, valid := parseAlign(value, initial == AlignAuto, distributed)
	if valid {
		return parsed, safety
	}
	return current, currentSafety
}

func placeShorthandComponent(source, value string, first bool) string {
	if source != "place-content" && source != "place-items" && source != "place-self" {
		return value
	}
	parts, valid := splitCSSSpaceSeparated(value)
	if !valid || len(parts) == 0 || len(parts) > 2 {
		return value
	}
	if first || len(parts) == 1 {
		return parts[0]
	}
	return parts[1]
}

func justifyPlaceCandidate(candidate winner) winner {
	if candidate.source == "place-items" || candidate.source == "place-self" {
		candidate.value = placeShorthandComponent(candidate.source, candidate.value, false)
		candidate.source = "justify"
	}
	return candidate
}

func resolveIntegerWinner(current, parent, initial int, candidate winner, custom map[string]string) int {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current
	}
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent
	case globalInitial, globalUnset:
		return initial
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err == nil {
		return parsed
	}
	return current
}

func resolveFactorWinner(property string, current, parent, initial float32, candidate winner, custom map[string]string) float32 {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current
	}
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent
	case globalInitial, globalUnset:
		return initial
	}
	if candidate.source == "flex" {
		components, valid := parseFlexShorthand(value, LengthContext{})
		if !valid {
			return current
		}
		if property == "flex-grow" {
			return components.grow
		}
		return components.shrink
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 32)
	if err == nil && parsed >= 0 && !math.IsInf(parsed, 0) && !math.IsNaN(parsed) {
		return float32(parsed)
	}
	return current
}

func resolveBasisWinner(current, parent FlexBasis, candidate winner, custom map[string]string, context LengthContext) FlexBasis {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current
	}
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent
	case globalInitial, globalUnset:
		return FlexBasis{}
	}
	if candidate.source == "flex" {
		components, valid := parseFlexShorthand(value, context)
		if valid {
			return components.basis
		}
		return current
	}
	if parsed, valid := parseFlexBasis(value, context); valid {
		return parsed
	}
	return current
}

func resolveGapWinner(property string, current, parent LengthPercentage, candidate winner, custom map[string]string, context LengthContext) LengthPercentage {
	value, ok := winnerValue(candidate, custom)
	if !ok {
		return current
	}
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent
	case globalInitial, globalUnset:
		return LengthPercentage{}
	}
	if candidate.source == "gap" {
		parts, valid := splitCSSSpaceSeparated(value)
		if !valid || len(parts) < 1 || len(parts) > 2 {
			return current
		}
		if property == "column-gap" && len(parts) == 2 {
			value = parts[1]
		} else {
			value = parts[0]
		}
	}
	if strings.EqualFold(strings.TrimSpace(value), "normal") {
		return LengthPercentage{}
	}
	parsed, valid := ResolveLength(value, context)
	if valid && parsed.Pixels >= 0 && parsed.Percentage >= 0 {
		return parsed
	}
	return current
}

func resolveAspectRatioWinner(current, parent float32, candidate winner, custom map[string]string) float32 {
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
	if parsed, valid := parseAspectRatio(value); valid {
		return parsed
	}
	return current
}

func parseAspectRatio(value string) (float32, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "auto" {
		return 0, true
	}
	if strings.HasPrefix(value, "auto") {
		remainder := strings.TrimSpace(strings.TrimPrefix(value, "auto"))
		if remainder == "" {
			return 0, true
		}
		value = remainder
	}
	ratioParts := strings.Split(strings.TrimSpace(value), "/")
	numerator, err := strconv.ParseFloat(strings.TrimSpace(ratioParts[0]), 32)
	if err != nil || numerator <= 0 || math.IsInf(numerator, 0) || math.IsNaN(numerator) {
		return 0, false
	}
	denominator := 1.0
	if len(ratioParts) == 2 {
		denominator, err = strconv.ParseFloat(strings.TrimSpace(ratioParts[1]), 32)
	} else if len(ratioParts) > 2 {
		return 0, false
	}
	if err != nil || denominator <= 0 || math.IsInf(denominator, 0) || math.IsNaN(denominator) {
		return 0, false
	}
	return float32(numerator / denominator), true
}

func winnerValue(candidate winner, custom map[string]string) (string, bool) {
	if candidate.source == "" {
		return "", false
	}
	return resolveVariables(candidate.value, custom)
}

func parseFlexDirection(value string) (FlexDirection, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "row":
		return FlexDirectionRow, true
	case "row-reverse":
		return FlexDirectionRowReverse, true
	case "column":
		return FlexDirectionColumn, true
	case "column-reverse":
		return FlexDirectionColumnReverse, true
	default:
		return 0, false
	}
}

func parseFlexWrap(value string) (FlexWrap, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "nowrap":
		return FlexNoWrap, true
	case "wrap":
		return FlexWrapLines, true
	case "wrap-reverse":
		return FlexWrapReverse, true
	default:
		return 0, false
	}
}

func flexFlowComponent(value string, direction bool) (string, bool) {
	parts, ok := splitCSSSpaceSeparated(value)
	if !ok || len(parts) < 1 || len(parts) > 2 {
		return "", false
	}
	for _, part := range parts {
		if direction {
			if _, valid := parseFlexDirection(part); valid {
				return part, true
			}
		} else if _, valid := parseFlexWrap(part); valid {
			return part, true
		}
	}
	if direction {
		return "row", true
	}
	return "nowrap", true
}

func parseAlign(value string, allowAuto, distributed bool) (Align, OverflowAlignment, bool) {
	value, safety, valid := parseOverflowAlignment(value)
	if !valid {
		return 0, 0, false
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "normal", "stretch":
		return AlignStretch, safety, true
	case "flex-start":
		return AlignFlexStart, safety, true
	case "flex-end":
		return AlignFlexEnd, safety, true
	case "start":
		return AlignStart, safety, true
	case "end":
		return AlignEnd, safety, true
	case "self-start":
		return AlignSelfStart, safety, true
	case "self-end":
		return AlignSelfEnd, safety, true
	case "center":
		return AlignCenter, safety, true
	case "baseline":
		return AlignBaseline, safety, true
	case "space-between":
		return AlignSpaceBetween, safety, distributed
	case "space-around":
		return AlignSpaceAround, safety, distributed
	case "space-evenly":
		return AlignSpaceEvenly, safety, distributed
	case "auto":
		return AlignAuto, safety, allowAuto
	default:
		return 0, 0, false
	}
}

func parseOverflowAlignment(value string) (string, OverflowAlignment, bool) {
	parts, valid := splitCSSSpaceSeparated(strings.ToLower(strings.TrimSpace(value)))
	if !valid || len(parts) == 0 || len(parts) > 2 {
		return "", OverflowAlignmentDefault, false
	}
	safety := OverflowAlignmentDefault
	if parts[0] == "safe" || parts[0] == "unsafe" {
		if len(parts) != 2 {
			return "", OverflowAlignmentDefault, false
		}
		if parts[0] == "safe" {
			safety = OverflowAlignmentSafe
		} else {
			safety = OverflowAlignmentUnsafe
		}
		parts = parts[1:]
	}
	return parts[0], safety, true
}

type flexShorthand struct {
	grow, shrink float32
	basis        FlexBasis
}

func parseFlexShorthand(value string, context LengthContext) (flexShorthand, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "none":
		return flexShorthand{basis: FlexBasis{}}, true
	case "auto":
		return flexShorthand{grow: 1, shrink: 1, basis: FlexBasis{}}, true
	}
	parts, ok := splitCSSSpaceSeparated(value)
	if !ok || len(parts) < 1 || len(parts) > 3 {
		return flexShorthand{}, false
	}
	result := flexShorthand{shrink: 1, basis: FlexBasis{Kind: FlexBasisLength, Value: LengthPercentage{Percentage: 0}}}
	numbers := 0
	for _, part := range parts {
		if number, err := strconv.ParseFloat(part, 32); err == nil && number >= 0 && !math.IsInf(number, 0) && !math.IsNaN(number) {
			if numbers == 2 && number == 0 {
				result.basis = FlexBasis{Kind: FlexBasisLength}
				continue
			}
			if numbers == 0 {
				result.grow = float32(number)
			} else if numbers == 1 {
				result.shrink = float32(number)
			} else {
				return flexShorthand{}, false
			}
			numbers++
			continue
		}
		basis, valid := parseFlexBasis(part, context)
		if !valid || result.basis.Kind != FlexBasisLength || result.basis.Value != (LengthPercentage{}) {
			return flexShorthand{}, false
		}
		result.basis = basis
	}
	if numbers == 0 {
		result.grow = 1
	}
	return result, true
}

func parseFlexBasis(value string, context LengthContext) (FlexBasis, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto":
		return FlexBasis{}, true
	case "content":
		return FlexBasis{Kind: FlexBasisContent}, true
	}
	length, valid := ResolveLength(value, context)
	if !valid || length.Pixels < 0 || length.Percentage < 0 {
		return FlexBasis{}, false
	}
	return FlexBasis{Kind: FlexBasisLength, Value: length}, true
}

func applyMargins(edges Edges, automatic AutoEdges, parent Edges, parentAuto AutoEdges, winners map[string]winner, custom map[string]string, context LengthContext) (Edges, AutoEdges) {
	properties := []string{"margin-top", "margin-right", "margin-bottom", "margin-left"}
	values := []*float32{&edges.Top, &edges.Right, &edges.Bottom, &edges.Left}
	autos := []*bool{&automatic.Top, &automatic.Right, &automatic.Bottom, &automatic.Left}
	parentValues := []float32{parent.Top, parent.Right, parent.Bottom, parent.Left}
	parentAutos := []bool{parentAuto.Top, parentAuto.Right, parentAuto.Bottom, parentAuto.Left}
	for index, property := range properties {
		candidate, exists := winners[property]
		if !exists {
			continue
		}
		resolved, ok := resolveVariables(candidate.value, custom)
		if !ok {
			continue
		}
		switch parseGlobalKeyword(resolved) {
		case globalInherit:
			*values[index], *autos[index] = parentValues[index], parentAutos[index]
			continue
		case globalInitial, globalUnset:
			*values[index], *autos[index] = 0, false
			continue
		}
		part := resolved
		if candidate.source == "margin" {
			parts, valid := splitCSSSpaceSeparated(resolved)
			if !valid || len(parts) < 1 || len(parts) > 4 {
				continue
			}
			part = edgePart(parts, index)
		} else {
			var valid bool
			part, valid = logicalEdgeComponent(candidate.source, resolved, index, "margin")
			if !valid {
				continue
			}
		}
		if strings.EqualFold(strings.TrimSpace(part), "auto") {
			*values[index], *autos[index] = 0, true
			continue
		}
		length, valid := ResolveLength(part, context)
		if valid {
			*values[index], *autos[index] = length.Resolve(context.PercentageBase), false
		}
	}
	return edges, automatic
}
