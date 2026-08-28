package style

import (
	"math"
	"strings"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func matchesContainerGroups(node *dom.Node, groups []css.ContainerQuery, computed Map, environment Environment) bool {
	for _, query := range groups {
		if !matchesContainerQuery(node, query, computed, environment) {
			return false
		}
	}
	return true
}

func matchesContainerQuery(node *dom.Node, query css.ContainerQuery, computed Map, environment Environment) bool {
	if !query.Valid {
		return false
	}
	for ancestor := node.Parent; ancestor != nil; ancestor = ancestor.Parent {
		ancestorStyle, ok := computed.For(ancestor)
		if !ok || ancestorStyle.ContainerType != ContainerTypeInlineSize || query.Name != "" && ancestorStyle.ContainerName != query.Name {
			continue
		}
		size, measured := environment.ContainerSizes[ancestor.ID]
		if !measured || size.Width < 0 || size.Height < 0 {
			return false
		}
		for _, feature := range query.Features {
			context := LengthContext{
				FontSize: 16, RootFontSize: environment.RootFontSize,
				ViewportWidth: environment.ViewportWidth, ViewportHeight: environment.ViewportHeight,
				ContainerWidth: size.Width, ContainerHeight: size.Height,
			}
			expected, valid := ResolveLength(feature.Value, context)
			if !valid || expected.Percentage != 0 {
				return false
			}
			actual := size.Width
			switch strings.ToLower(feature.Name) {
			case "min-width":
				if actual < expected.Pixels {
					return false
				}
			case "max-width":
				if actual > expected.Pixels {
					return false
				}
			case "width":
				if math.Abs(float64(actual-expected.Pixels)) >= 0.001 {
					return false
				}
			default:
				return false
			}
		}
		return true
	}
	return false
}
