package style

import (
	"math"
	"strconv"
	"strings"

	"github.com/Grove-Computing/Growse/internal/css"
)

type calculationKind uint8

const (
	calculationNumber calculationKind = iota
	calculationLength
)

type calculationValue struct {
	kind   calculationKind
	number float32
	length LengthPercentage
}

type calculationParser struct {
	input   string
	context LengthContext
	pos     int
	depth   int
}

func resolveCalculation(value string, context LengthContext) (LengthPercentage, bool) {
	parser := calculationParser{input: value, context: context}
	result, ok := parser.parseExpression()
	parser.skipWhitespace()
	if !ok || parser.pos != len(parser.input) || result.kind != calculationLength || !finiteCalculation(result) {
		return LengthPercentage{}, false
	}
	return result.length, true
}

func (parser *calculationParser) parseExpression() (calculationValue, bool) {
	left, ok := parser.parseProduct()
	if !ok {
		return calculationValue{}, false
	}
	for {
		parser.skipWhitespace()
		if parser.pos >= len(parser.input) || parser.input[parser.pos] != '+' && parser.input[parser.pos] != '-' {
			return left, true
		}
		operator := parser.input[parser.pos]
		parser.pos++
		right, ok := parser.parseProduct()
		if !ok || left.kind != right.kind {
			return calculationValue{}, false
		}
		if left.kind == calculationNumber {
			if operator == '+' {
				left.number += right.number
			} else {
				left.number -= right.number
			}
		} else if operator == '+' {
			left.length.Pixels += right.length.Pixels
			left.length.Percentage += right.length.Percentage
		} else {
			left.length.Pixels -= right.length.Pixels
			left.length.Percentage -= right.length.Percentage
		}
		if !finiteCalculation(left) {
			return calculationValue{}, false
		}
	}
}

func (parser *calculationParser) parseProduct() (calculationValue, bool) {
	left, ok := parser.parseUnary()
	if !ok {
		return calculationValue{}, false
	}
	for {
		parser.skipWhitespace()
		if parser.pos >= len(parser.input) || parser.input[parser.pos] != '*' && parser.input[parser.pos] != '/' {
			return left, true
		}
		operator := parser.input[parser.pos]
		parser.pos++
		right, ok := parser.parseUnary()
		if !ok {
			return calculationValue{}, false
		}
		if operator == '*' {
			switch {
			case left.kind == calculationNumber && right.kind == calculationNumber:
				left.number *= right.number
			case left.kind == calculationLength && right.kind == calculationNumber:
				left.length.Pixels *= right.number
				left.length.Percentage *= right.number
			case left.kind == calculationNumber && right.kind == calculationLength:
				factor := left.number
				left = right
				left.length.Pixels *= factor
				left.length.Percentage *= factor
			default:
				return calculationValue{}, false
			}
		} else {
			if right.kind != calculationNumber || right.number == 0 {
				return calculationValue{}, false
			}
			if left.kind == calculationNumber {
				left.number /= right.number
			} else {
				left.length.Pixels /= right.number
				left.length.Percentage /= right.number
			}
		}
		if !finiteCalculation(left) {
			return calculationValue{}, false
		}
	}
}

func (parser *calculationParser) parseUnary() (calculationValue, bool) {
	parser.skipWhitespace()
	sign := float32(1)
	for parser.pos < len(parser.input) && (parser.input[parser.pos] == '+' || parser.input[parser.pos] == '-') {
		if parser.input[parser.pos] == '-' {
			sign = -sign
		}
		parser.pos++
		parser.skipWhitespace()
	}
	value, ok := parser.parsePrimary()
	if !ok {
		return calculationValue{}, false
	}
	if value.kind == calculationNumber {
		value.number *= sign
	} else {
		value.length.Pixels *= sign
		value.length.Percentage *= sign
	}
	return value, finiteCalculation(value)
}

func (parser *calculationParser) parsePrimary() (calculationValue, bool) {
	parser.skipWhitespace()
	if parser.pos >= len(parser.input) {
		return calculationValue{}, false
	}
	if parser.input[parser.pos] == '(' {
		if parser.depth >= css.MaxCSSFunctionDepth {
			return calculationValue{}, false
		}
		parser.depth++
		parser.pos++
		value, ok := parser.parseExpression()
		parser.skipWhitespace()
		if !ok || parser.pos >= len(parser.input) || parser.input[parser.pos] != ')' {
			return calculationValue{}, false
		}
		parser.pos++
		parser.depth--
		return value, true
	}
	for _, name := range []string{"calc", "min", "max", "clamp"} {
		prefix := name + "("
		if parser.pos+len(prefix) <= len(parser.input) && strings.EqualFold(parser.input[parser.pos:parser.pos+len(prefix)], prefix) {
			if parser.depth >= css.MaxCSSFunctionDepth {
				return calculationValue{}, false
			}
			parser.depth++
			parser.pos += len(prefix)
			result, ok := parser.parseMathFunction(name)
			parser.depth--
			return result, ok
		}
	}
	number, unit, next, ok := scanCalculationDimension(parser.input, parser.pos)
	if !ok {
		return calculationValue{}, false
	}
	parser.pos = next
	if unit == "" {
		return calculationValue{kind: calculationNumber, number: number}, true
	}
	length, ok := resolveSimpleLength(strconv.FormatFloat(float64(number), 'g', -1, 32)+unit, parser.context)
	return calculationValue{kind: calculationLength, length: length}, ok
}

func (parser *calculationParser) parseMathFunction(name string) (calculationValue, bool) {
	values := make([]calculationValue, 0, 3)
	for {
		value, ok := parser.parseExpression()
		if !ok || !finiteCalculation(value) {
			return calculationValue{}, false
		}
		values = append(values, value)
		parser.skipWhitespace()
		if parser.pos >= len(parser.input) {
			return calculationValue{}, false
		}
		if parser.input[parser.pos] == ')' {
			parser.pos++
			break
		}
		if parser.input[parser.pos] != ',' {
			return calculationValue{}, false
		}
		parser.pos++
	}
	if name == "calc" {
		return values[0], len(values) == 1
	}
	if len(values) == 0 || name == "clamp" && len(values) != 3 || name != "clamp" && len(values) < 1 {
		return calculationValue{}, false
	}
	for _, value := range values[1:] {
		if value.kind != values[0].kind {
			return calculationValue{}, false
		}
	}
	compare := func(value calculationValue) float32 {
		if value.kind == calculationNumber {
			return value.number
		}
		return value.length.Resolve(parser.context.PercentageBase)
	}
	selected := values[0]
	switch name {
	case "min":
		for _, value := range values[1:] {
			if compare(value) < compare(selected) {
				selected = value
			}
		}
	case "max":
		for _, value := range values[1:] {
			if compare(value) > compare(selected) {
				selected = value
			}
		}
	case "clamp":
		selected = values[1]
		if compare(selected) < compare(values[0]) {
			selected = values[0]
		}
		if compare(selected) > compare(values[2]) {
			selected = values[2]
		}
	default:
		return calculationValue{}, false
	}
	return selected, finiteCalculation(selected)
}

func scanCalculationDimension(value string, start int) (float32, string, int, bool) {
	position := start
	digits := 0
	for position < len(value) && value[position] >= '0' && value[position] <= '9' {
		position++
		digits++
	}
	if position < len(value) && value[position] == '.' {
		position++
		for position < len(value) && value[position] >= '0' && value[position] <= '9' {
			position++
			digits++
		}
	}
	if digits == 0 {
		return 0, "", start, false
	}
	if position < len(value) && (value[position] == 'e' || value[position] == 'E') {
		exponentStart := position
		position++
		if position < len(value) && (value[position] == '+' || value[position] == '-') {
			position++
		}
		exponentDigits := 0
		for position < len(value) && value[position] >= '0' && value[position] <= '9' {
			position++
			exponentDigits++
		}
		if exponentDigits == 0 {
			position = exponentStart
		}
	}
	numberEnd := position
	for position < len(value) && (value[position] >= 'a' && value[position] <= 'z' || value[position] >= 'A' && value[position] <= 'Z' || value[position] == '%') {
		position++
	}
	number, err := strconv.ParseFloat(value[start:numberEnd], 32)
	return float32(number), strings.ToLower(value[numberEnd:position]), position, err == nil
}

func (parser *calculationParser) skipWhitespace() {
	for parser.pos < len(parser.input) && isCSSWhitespaceByte(parser.input[parser.pos]) {
		parser.pos++
	}
}

func finiteCalculation(value calculationValue) bool {
	if value.kind == calculationNumber {
		return !math.IsNaN(float64(value.number)) && !math.IsInf(float64(value.number), 0)
	}
	return !math.IsNaN(float64(value.length.Pixels)) && !math.IsInf(float64(value.length.Pixels), 0) &&
		!math.IsNaN(float64(value.length.Percentage)) && !math.IsInf(float64(value.length.Percentage), 0)
}
