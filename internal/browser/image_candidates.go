package browser

import (
	"mime"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
)

type imageCandidate struct {
	url     *url.URL
	density float32
	order   int
}

func imageCandidates(imageNode *dom.Node, baseURL *url.URL, viewportWidth, deviceScale float32) []*url.URL {
	if imageNode == nil || baseURL == nil {
		return nil
	}
	if viewportWidth <= 0 {
		viewportWidth = 1280
	}
	if deviceScale <= 0 {
		deviceScale = 1
	}
	var groups [][]imageCandidate
	if parent := imageNode.Parent; parent != nil && parent.Type == dom.NodeElement && parent.TagName == "picture" {
		for _, sibling := range parent.Children {
			if sibling == imageNode {
				break
			}
			if sibling.Type != dom.NodeElement || sibling.TagName != "source" || !sourceMatches(sibling, viewportWidth) {
				continue
			}
			if srcset, ok := sibling.Attribute("srcset"); ok {
				if candidates := parseImageSrcset(srcset, baseURL, sourceSize(sibling, viewportWidth), deviceScale); len(candidates) != 0 {
					groups = append(groups, candidates)
				}
			}
		}
	}
	if srcset, ok := imageNode.Attribute("srcset"); ok {
		if candidates := parseImageSrcset(srcset, baseURL, sourceSize(imageNode, viewportWidth), deviceScale); len(candidates) != 0 {
			groups = append(groups, candidates)
		}
	}
	if source, ok := imageNode.Attribute("src"); ok {
		if target := resolveImageCandidate(baseURL, source); target != nil {
			groups = append(groups, []imageCandidate{{url: target, density: 1}})
		}
	}
	seen := make(map[string]bool)
	var result []*url.URL
	for _, group := range groups {
		for _, candidate := range orderedImageCandidates(group, deviceScale) {
			if candidate.url == nil || seen[candidate.url.String()] {
				continue
			}
			seen[candidate.url.String()] = true
			result = append(result, candidate.url)
		}
	}
	return result
}

func sourceMatches(node *dom.Node, viewportWidth float32) bool {
	if rawType, ok := node.Attribute("type"); ok && strings.TrimSpace(rawType) != "" {
		mediaType, _, err := mime.ParseMediaType(rawType)
		if err != nil || !isImageContentType(mediaType) {
			return false
		}
	}
	media, ok := node.Attribute("media")
	if !ok || strings.TrimSpace(media) == "" {
		return true
	}
	queries := css.ParseMediaQueryList(media)
	return len(queries) != 0 && style.MatchesMediaQueryList(queries, style.Environment{
		ViewportWidth: viewportWidth, ViewportHeight: 720, RootFontSize: 16, ResolutionDPI: 96,
	})
}

func parseImageSrcset(raw string, baseURL *url.URL, sourceWidth, deviceScale float32) []imageCandidate {
	parts := splitImageList(raw)
	result := make([]imageCandidate, 0, len(parts))
	for order, part := range parts {
		fields := strings.Fields(part)
		if len(fields) == 0 || len(fields) > 2 {
			continue
		}
		target := resolveImageCandidate(baseURL, fields[0])
		if target == nil {
			continue
		}
		density := float32(1)
		if len(fields) == 2 {
			descriptor := fields[1]
			value, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSuffix(descriptor, "x"), "w"), 32)
			if err != nil || value <= 0 {
				continue
			}
			switch {
			case strings.HasSuffix(descriptor, "w") && sourceWidth > 0:
				density = float32(value) / sourceWidth
			case strings.HasSuffix(descriptor, "x"):
				density = float32(value)
			default:
				continue
			}
		}
		result = append(result, imageCandidate{url: target, density: max(density, float32(.01)), order: order})
	}
	return result
}

func orderedImageCandidates(candidates []imageCandidate, deviceScale float32) []imageCandidate {
	result := append([]imageCandidate(nil), candidates...)
	sort.SliceStable(result, func(left, right int) bool {
		leftBelow, rightBelow := result[left].density < deviceScale, result[right].density < deviceScale
		if leftBelow != rightBelow {
			return !leftBelow
		}
		if leftBelow {
			return result[left].density > result[right].density
		}
		if result[left].density != result[right].density {
			return result[left].density < result[right].density
		}
		return result[left].order < result[right].order
	})
	return result
}

func sourceSize(node *dom.Node, viewportWidth float32) float32 {
	raw, ok := node.Attribute("sizes")
	if !ok || strings.TrimSpace(raw) == "" {
		return viewportWidth
	}
	context := style.LengthContext{FontSize: 16, RootFontSize: 16, ViewportWidth: viewportWidth, ViewportHeight: 720, PercentageBase: viewportWidth}
	for _, item := range splitImageList(raw) {
		item = strings.TrimSpace(item)
		lengthText := item
		matches := true
		if strings.HasPrefix(item, "(") {
			if end := strings.LastIndex(item, ")"); end >= 0 {
				mediaText := strings.TrimSpace(item[:end+1])
				lengthText = strings.TrimSpace(item[end+1:])
				queries := css.ParseMediaQueryList(mediaText)
				matches = len(queries) != 0 && style.MatchesMediaQueryList(queries, style.Environment{ViewportWidth: viewportWidth, ViewportHeight: 720, RootFontSize: 16, ResolutionDPI: 96})
			}
		}
		if !matches {
			continue
		}
		if length, valid := style.ResolveLength(lengthText, context); valid {
			return max(length.Resolve(viewportWidth), float32(1))
		}
	}
	return viewportWidth
}

func splitImageList(raw string) []string {
	var result []string
	start, depth := 0, 0
	for index, character := range raw {
		switch character {
		case '(':
			depth++
		case ')':
			depth = max(depth-1, 0)
		case ',':
			if depth == 0 {
				result = append(result, strings.TrimSpace(raw[start:index]))
				start = index + 1
			}
		}
	}
	result = append(result, strings.TrimSpace(raw[start:]))
	return result
}

func resolveImageCandidate(baseURL *url.URL, raw string) *url.URL {
	reference, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || reference.String() == "" {
		return nil
	}
	return baseURL.ResolveReference(reference)
}
