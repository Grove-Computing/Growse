package style

import (
	"math"
	"strconv"
	"strings"
)

func applyTransformProperties(computed, parent ComputedStyle, winners map[string]winner, custom map[string]string, context LengthContext) ComputedStyle {
	if candidate, ok := winners["transform"]; ok {
		if value, valid := winnerValue(candidate, custom); valid {
			switch parseGlobalKeyword(value) {
			case globalInherit:
				computed.Transform = append([]TransformFunction(nil), parent.Transform...)
			case globalInitial, globalUnset:
				computed.Transform = nil
			default:
				if parsed, valid := parseTransform(value, context); valid {
					computed.Transform = parsed
				}
			}
		}
	}
	computed.TransformOrigin = BackgroundPosition{X: LengthPercentage{Percentage: 50}, Y: LengthPercentage{Percentage: 50}}
	if candidate, ok := winners["transform-origin"]; ok {
		if value, valid := winnerValue(candidate, custom); valid {
			if parsed, valid := parseBackgroundPosition(value, context); valid {
				computed.TransformOrigin = parsed
			}
		}
	}
	return computed
}

func parseTransform(value string, context LengthContext) ([]TransformFunction, bool) {
	if strings.EqualFold(strings.TrimSpace(value), "none") {
		return nil, true
	}
	parts, valid := splitCSSSpaceSeparated(value)
	if !valid || len(parts) == 0 {
		return nil, false
	}
	result := make([]TransformFunction, 0, len(parts))
	for _, part := range parts {
		open := strings.IndexByte(part, '(')
		if open <= 0 || !strings.HasSuffix(part, ")") {
			return nil, false
		}
		name := strings.ToLower(strings.TrimSpace(part[:open]))
		args := transformArguments(part[open+1 : len(part)-1])
		function, valid := parseTransformFunction(name, args, context)
		if !valid {
			return nil, false
		}
		result = append(result, function)
	}
	return result, true
}

func transformArguments(value string) []string {
	value = strings.ReplaceAll(value, ",", " ")
	return strings.Fields(value)
}

func parseTransformFunction(name string, args []string, context LengthContext) (TransformFunction, bool) {
	length := func(index int) (LengthPercentage, bool) {
		if index >= len(args) {
			return LengthPercentage{}, false
		}
		return ResolveLength(args[index], context)
	}
	number := func(index int) (float32, bool) {
		if index >= len(args) {
			return 0, false
		}
		value, err := strconv.ParseFloat(args[index], 32)
		return float32(value), err == nil
	}
	angle := func(index int) (float32, bool) {
		if index >= len(args) {
			return 0, false
		}
		value := strings.ToLower(args[index])
		if !strings.HasSuffix(value, "deg") {
			return 0, false
		}
		degrees, err := strconv.ParseFloat(strings.TrimSuffix(value, "deg"), 32)
		return float32(degrees) * math.Pi / 180, err == nil
	}
	switch name {
	case "translate", "translatex", "translatey":
		x, y := LengthPercentage{}, LengthPercentage{}
		var ok bool
		if name != "translatey" {
			x, ok = length(0)
		} else {
			y, ok = length(0)
		}
		if !ok {
			return TransformFunction{}, false
		}
		if name == "translate" && len(args) == 2 {
			y, ok = length(1)
		}
		return TransformFunction{Kind: TransformTranslate, X: x, Y: y}, ok
	case "scale", "scalex", "scaley":
		x, y, ok := float32(1), float32(1), false
		if name != "scaley" {
			x, ok = number(0)
		} else {
			y, ok = number(0)
		}
		if name == "scale" {
			y = x
			if len(args) == 2 {
				y, ok = number(1)
			}
		}
		return TransformFunction{Kind: TransformScale, A: x, D: y}, ok
	case "rotate":
		value, ok := angle(0)
		return TransformFunction{Kind: TransformRotate, A: value}, ok && len(args) == 1
	case "skew", "skewx", "skewy":
		x, y, ok := float32(0), float32(0), false
		if name != "skewy" {
			x, ok = angle(0)
		} else {
			y, ok = angle(0)
		}
		if name == "skew" && len(args) == 2 {
			y, ok = angle(1)
		}
		return TransformFunction{Kind: TransformSkew, A: x, B: y}, ok
	case "matrix":
		if len(args) != 6 {
			return TransformFunction{}, false
		}
		values := [6]float32{}
		for index := range values {
			var ok bool
			values[index], ok = number(index)
			if !ok {
				return TransformFunction{}, false
			}
		}
		return TransformFunction{Kind: TransformMatrix, A: values[0], B: values[1], C: values[2], D: values[3], E: values[4], F: values[5]}, true
	}
	return TransformFunction{}, false
}

func IdentityMatrix() Matrix { return Matrix{A: 1, D: 1} }

func (left Matrix) Multiply(right Matrix) Matrix {
	return Matrix{A: left.A*right.A + left.C*right.B, B: left.B*right.A + left.D*right.B, C: left.A*right.C + left.C*right.D, D: left.B*right.C + left.D*right.D, E: left.A*right.E + left.C*right.F + left.E, F: left.B*right.E + left.D*right.F + left.F}
}

// TransformPoint maps a point through the matrix.
func (matrix Matrix) TransformPoint(x, y float32) (float32, float32) {
	return matrix.A*x + matrix.C*y + matrix.E, matrix.B*x + matrix.D*y + matrix.F
}

// Inverse returns the inverse affine matrix when it is non-singular.
func (matrix Matrix) Inverse() (Matrix, bool) {
	determinant := matrix.A*matrix.D - matrix.B*matrix.C
	if float32(math.Abs(float64(determinant))) < 1e-8 {
		return Matrix{}, false
	}
	return Matrix{A: matrix.D / determinant, B: -matrix.B / determinant, C: -matrix.C / determinant, D: matrix.A / determinant, E: (matrix.C*matrix.F - matrix.D*matrix.E) / determinant, F: (matrix.B*matrix.E - matrix.A*matrix.F) / determinant}, true
}

func ResolveTransform(functions []TransformFunction, width, height float32) Matrix {
	result := IdentityMatrix()
	for _, function := range functions {
		matrix := IdentityMatrix()
		switch function.Kind {
		case TransformTranslate:
			matrix.E, matrix.F = function.X.Resolve(width), function.Y.Resolve(height)
		case TransformScale:
			matrix.A, matrix.D = function.A, function.D
		case TransformRotate:
			cosine, sine := float32(math.Cos(float64(function.A))), float32(math.Sin(float64(function.A)))
			matrix.A, matrix.B, matrix.C, matrix.D = cosine, sine, -sine, cosine
		case TransformSkew:
			matrix.C, matrix.B = float32(math.Tan(float64(function.A))), float32(math.Tan(float64(function.B)))
		case TransformMatrix:
			matrix = Matrix{A: function.A, B: function.B, C: function.C, D: function.D, E: function.E, F: function.F}
		}
		result = result.Multiply(matrix)
	}
	return result
}
