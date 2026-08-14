package style

import "math"

// InterpolateTransform interpolates two computed 2D transform lists. Missing
// functions are padded with the corresponding identity function. When the
// function kinds differ, the remaining suffixes are resolved and interpolated
// through decomposed affine matrices.
func InterpolateTransform(from, to []TransformFunction, progress float64, width, height float32) []TransformFunction {
	length := max(len(from), len(to))
	result := make([]TransformFunction, 0, length)
	for index := 0; index < length; index++ {
		left, leftOK := transformAt(from, index)
		right, rightOK := transformAt(to, index)
		if !leftOK {
			left = identityTransform(right.Kind)
		}
		if !rightOK {
			right = identityTransform(left.Kind)
		}
		if left.Kind != right.Kind {
			leftSuffix := []TransformFunction{left}
			if index+1 < len(from) {
				leftSuffix = append(leftSuffix, from[index+1:]...)
			}
			rightSuffix := []TransformFunction{right}
			if index+1 < len(to) {
				rightSuffix = append(rightSuffix, to[index+1:]...)
			}
			leftMatrix := ResolveTransform(leftSuffix, width, height)
			rightMatrix := ResolveTransform(rightSuffix, width, height)
			result = append(result, matrixTransform(interpolateMatrix(leftMatrix, rightMatrix, progress)))
			return result
		}
		if left.Kind == TransformMatrix {
			result = append(result, matrixTransform(interpolateMatrix(matrixFromTransform(left), matrixFromTransform(right), progress)))
			continue
		}
		result = append(result, interpolateTransformFunction(left, right, progress))
	}
	return result
}

func transformAt(functions []TransformFunction, index int) (TransformFunction, bool) {
	if index >= len(functions) {
		return TransformFunction{}, false
	}
	return functions[index], true
}

func identityTransform(kind TransformFunctionKind) TransformFunction {
	identity := TransformFunction{Kind: kind}
	if kind == TransformScale || kind == TransformMatrix {
		identity.A, identity.D = 1, 1
	}
	return identity
}

func interpolateTransformFunction(from, to TransformFunction, progress float64) TransformFunction {
	result := TransformFunction{Kind: from.Kind}
	result.X = interpolateLengthPercentage(from.X, to.X, progress)
	result.Y = interpolateLengthPercentage(from.Y, to.Y, progress)
	result.A = interpolateFloat(from.A, to.A, progress)
	result.B = interpolateFloat(from.B, to.B, progress)
	result.C = interpolateFloat(from.C, to.C, progress)
	result.D = interpolateFloat(from.D, to.D, progress)
	result.E = interpolateFloat(from.E, to.E, progress)
	result.F = interpolateFloat(from.F, to.F, progress)
	return result
}

func interpolateLengthPercentage(from, to LengthPercentage, progress float64) LengthPercentage {
	return LengthPercentage{
		Pixels:     interpolateFloat(from.Pixels, to.Pixels, progress),
		Percentage: interpolateFloat(from.Percentage, to.Percentage, progress),
	}
}

func interpolateFloat(from, to float32, progress float64) float32 {
	return from + (to-from)*float32(progress)
}

type decomposedMatrix struct {
	translateX, translateY float64
	rotation               float64
	scaleX, scaleY         float64
	skewX                  float64
}

func interpolateMatrix(from, to Matrix, progress float64) Matrix {
	left, leftOK := decomposeMatrix(from)
	right, rightOK := decomposeMatrix(to)
	if !leftOK || !rightOK {
		return Matrix{
			A: interpolateFloat(from.A, to.A, progress), B: interpolateFloat(from.B, to.B, progress),
			C: interpolateFloat(from.C, to.C, progress), D: interpolateFloat(from.D, to.D, progress),
			E: interpolateFloat(from.E, to.E, progress), F: interpolateFloat(from.F, to.F, progress),
		}
	}
	deltaRotation := right.rotation - left.rotation
	for deltaRotation > math.Pi {
		deltaRotation -= 2 * math.Pi
	}
	for deltaRotation < -math.Pi {
		deltaRotation += 2 * math.Pi
	}
	value := decomposedMatrix{
		translateX: lerp64(left.translateX, right.translateX, progress),
		translateY: lerp64(left.translateY, right.translateY, progress),
		rotation:   left.rotation + deltaRotation*progress,
		scaleX:     lerp64(left.scaleX, right.scaleX, progress),
		scaleY:     lerp64(left.scaleY, right.scaleY, progress),
		skewX:      lerp64(left.skewX, right.skewX, progress),
	}
	return composeMatrix(value)
}

func decomposeMatrix(matrix Matrix) (decomposedMatrix, bool) {
	a, b := float64(matrix.A), float64(matrix.B)
	c, d := float64(matrix.C), float64(matrix.D)
	scaleX := math.Hypot(a, b)
	if scaleX < 1e-12 {
		return decomposedMatrix{}, false
	}
	a, b = a/scaleX, b/scaleX
	shear := a*c + b*d
	c, d = c-a*shear, d-b*shear
	scaleY := math.Hypot(c, d)
	if scaleY < 1e-12 {
		return decomposedMatrix{}, false
	}
	c, d = c/scaleY, d/scaleY
	shear /= scaleY
	if a*d-b*c < 0 {
		scaleX = -scaleX
		a, b = -a, -b
		shear = -shear
	}
	return decomposedMatrix{
		translateX: float64(matrix.E), translateY: float64(matrix.F),
		rotation: math.Atan2(b, a), scaleX: scaleX, scaleY: scaleY, skewX: math.Atan(shear),
	}, true
}

func composeMatrix(value decomposedMatrix) Matrix {
	cosine, sine := math.Cos(value.rotation), math.Sin(value.rotation)
	tangent := math.Tan(value.skewX)
	return Matrix{
		A: float32(cosine * value.scaleX),
		B: float32(sine * value.scaleX),
		C: float32(value.scaleY * (cosine*tangent - sine)),
		D: float32(value.scaleY * (sine*tangent + cosine)),
		E: float32(value.translateX), F: float32(value.translateY),
	}
}

func matrixFromTransform(function TransformFunction) Matrix {
	return Matrix{A: function.A, B: function.B, C: function.C, D: function.D, E: function.E, F: function.F}
}

func matrixTransform(matrix Matrix) TransformFunction {
	return TransformFunction{Kind: TransformMatrix, A: matrix.A, B: matrix.B, C: matrix.C, D: matrix.D, E: matrix.E, F: matrix.F}
}

func lerp64(from, to, progress float64) float64 {
	return from + (to-from)*progress
}
