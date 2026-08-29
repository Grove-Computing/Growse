package style

import (
	"math"
	"strconv"
	"strings"

	"github.com/Grove-Computing/Growse/internal/css"
)

func matchesMediaGroups(groups [][]css.MediaQuery, environment Environment) bool {
	for _, group := range groups {
		matched := false
		for _, query := range group {
			if matchesMediaQuery(query, environment) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// MatchesMediaQueryList evaluates one comma-separated query list.
func MatchesMediaQueryList(queries []css.MediaQuery, environment Environment) bool {
	for _, query := range queries {
		if matchesMediaQuery(query, environment) {
			return true
		}
	}
	return false
}

func matchesMediaQuery(query css.MediaQuery, environment Environment) bool {
	matched := query.Type == "all" || query.Type == "screen"
	if matched {
		for _, feature := range query.Features {
			if !matchesMediaFeature(feature, environment) {
				matched = false
				break
			}
		}
	}
	if query.Modifier == css.MediaModifierNot {
		return !matched
	}
	return matched
}

func matchesMediaFeature(feature css.MediaFeature, environment Environment) bool {
	name, value := feature.Name, strings.ToLower(strings.TrimSpace(feature.Value))
	switch name {
	case "width", "min-width", "max-width":
		if feature.Comparator != "" {
			return compareMediaLengthRange(environment.ViewportWidth, value, feature.Comparator, environment)
		}
		return compareMediaLength(name, environment.ViewportWidth, value, environment)
	case "height", "min-height", "max-height":
		if feature.Comparator != "" {
			return compareMediaLengthRange(environment.ViewportHeight, value, feature.Comparator, environment)
		}
		return compareMediaLength(name, environment.ViewportHeight, value, environment)
	case "orientation":
		if value == "portrait" {
			return environment.ViewportHeight >= environment.ViewportWidth
		}
		if value == "landscape" {
			return environment.ViewportWidth > environment.ViewportHeight
		}
		return false
	case "resolution", "min-resolution", "max-resolution":
		expected, ok := parseResolution(value)
		if feature.Comparator != "" {
			return ok && compareMediaRange(environment.ResolutionDPI, expected, feature.Comparator)
		}
		return ok && compareMediaNumber(name, environment.ResolutionDPI, expected)
	case "prefers-color-scheme":
		return (value == "light" || value == "dark") && value == strings.ToLower(environment.ColorScheme)
	case "prefers-reduced-motion":
		if value == "reduce" {
			return environment.ReducedMotion
		}
		return value == "no-preference" && !environment.ReducedMotion
	case "hover":
		if value == "" || value == "hover" {
			return environment.Hover
		}
		return value == "none" && !environment.Hover
	case "pointer":
		if value == "" {
			return environment.Pointer != "none"
		}
		return value == strings.ToLower(environment.Pointer) && (value == "none" || value == "coarse" || value == "fine")
	default:
		return false
	}
}

func compareMediaLengthRange(actual float32, value, comparator string, environment Environment) bool {
	length, ok := ResolveLength(value, LengthContext{
		FontSize: 16, RootFontSize: 16,
		ViewportWidth: environment.ViewportWidth, ViewportHeight: environment.ViewportHeight,
	})
	return ok && length.Percentage == 0 && compareMediaRange(actual, length.Pixels, comparator)
}

func compareMediaRange(actual, expected float32, comparator string) bool {
	switch comparator {
	case "<":
		return actual < expected
	case "<=":
		return actual <= expected
	case ">":
		return actual > expected
	case ">=":
		return actual >= expected
	case "=":
		return math.Abs(float64(actual-expected)) < 0.001
	default:
		return false
	}
}

func compareMediaLength(name string, actual float32, value string, environment Environment) bool {
	if value == "" {
		return !strings.HasPrefix(name, "min-") && !strings.HasPrefix(name, "max-") && actual != 0
	}
	length, ok := ResolveLength(value, LengthContext{
		FontSize: 16, RootFontSize: 16,
		ViewportWidth: environment.ViewportWidth, ViewportHeight: environment.ViewportHeight,
	})
	if !ok || length.Percentage != 0 {
		return false
	}
	return compareMediaNumber(name, actual, length.Pixels)
}

func compareMediaNumber(name string, actual, expected float32) bool {
	if strings.HasPrefix(name, "min-") {
		return actual >= expected
	}
	if strings.HasPrefix(name, "max-") {
		return actual <= expected
	}
	return math.Abs(float64(actual-expected)) < 0.001
}

func parseResolution(value string) (float32, bool) {
	units := []struct {
		suffix string
		factor float64
	}{{"dppx", 96}, {"dpcm", 2.54}, {"dpi", 1}}
	for _, unit := range units {
		if !strings.HasSuffix(value, unit.suffix) {
			continue
		}
		number, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, unit.suffix)), 32)
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
			return 0, false
		}
		return float32(number * unit.factor), true
	}
	return 0, false
}
