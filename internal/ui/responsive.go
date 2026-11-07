package ui

import (
	"image"
	"net/url"
	"strings"

	"github.com/Grove-Computing/Growse/internal/omnibox"
)

func candidateOrigin(candidate omnibox.Candidate) string {
	for _, raw := range []string{candidate.URL, candidate.Secondary} {
		parsed, err := url.Parse(strings.TrimSpace(raw))
		if err == nil && parsed.Hostname() != "" {
			return parsed.Hostname()
		}
	}
	if candidate.Source == omnibox.TabSource {
		return "Open tabs"
	}
	if candidate.Query != "" {
		return "Web"
	}
	return "Local"
}

func candidateAction(candidate omnibox.Candidate) string {
	if candidate.Source == omnibox.TabSource {
		return "Switch"
	}
	if candidate.URL != "" {
		return "Open"
	}
	return "Search"
}

func candidateMetadata(candidate omnibox.Candidate) string {
	return "Source: " + candidate.Source.Label() + " · Origin: " + candidateOrigin(candidate) + " · Action: " + candidateAction(candidate)
}

func findOverlayBounds(viewport image.Rectangle, maxWidth, height, preferredMargin int) image.Rectangle {
	if viewport.Empty() || maxWidth <= 0 || height <= 0 {
		return image.Rectangle{}
	}
	margin := min(max(preferredMargin, 0), max((viewport.Dx()-1)/2, 0))
	width := min(maxWidth, max(viewport.Dx()-2*margin, 0))
	height = min(height, viewport.Dy())
	if width <= 0 || height <= 0 {
		return image.Rectangle{}
	}
	return image.Rect(viewport.Max.X-margin-width, viewport.Min.Y, viewport.Max.X-margin, viewport.Min.Y+height)
}
