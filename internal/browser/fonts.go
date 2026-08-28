package browser

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Grove-Computing/Growse/internal/css"
	layoutmodel "github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	textfont "github.com/go-text/typesetting/font"
	woff2 "github.com/pgaskin/go-woff2"
)

const (
	maxFontBytes     = 2 << 20
	maxPageFontBytes = 16 << 20
	maxFontFaces     = 128
	fontBlockTimeout = 3 * time.Second
	fontFallbackWait = 100 * time.Millisecond
	fontOptionalWait = 50 * time.Millisecond
	pageFontTimeout  = 5 * time.Second
)

// FontRange is one inclusive @font-face unicode-range interval.
type FontRange struct{ Start, End rune }

// FontResource is one validated and decoded @font-face source.
type FontResource struct {
	Family, Style, Weight, Stretch string
	Display                        string
	UnicodeRanges                  []FontRange
	URL, FinalURL, Format          string
	Loaded, Decoded                bool
	Error                          string
	Face                           *textfont.Face
}

type fontSource struct {
	url    *url.URL
	format string
}

func loadWebFonts(ctx context.Context, client ResourceLoader, pageURL *url.URL, stylesheet *css.Stylesheet) ([]FontResource, []string) {
	if client == nil || pageURL == nil || stylesheet == nil {
		return nil, nil
	}
	faces := stylesheet.FontFaces
	if len(faces) > maxFontFaces {
		faces = faces[:maxFontFaces]
	}
	resources := make([]FontResource, 0, len(faces))
	var failures []string
	totalBytes := 0
	pageContext, cancelPage := context.WithTimeout(ctx, pageFontTimeout)
	defer cancelPage()
	for _, rule := range faces {
		resource := FontResource{Family: rule.Family, Style: rule.Style, Weight: rule.Weight, Stretch: rule.Stretch, Display: rule.Display}
		ranges, descriptorErr := parseFontUnicodeRanges(rule.UnicodeRange)
		if descriptorErr == nil {
			descriptorErr = validateFontDescriptors(rule)
		}
		resource.UnicodeRanges = ranges
		sources := parseFontSources(rule.Source, pageURL)
		if descriptorErr != nil || len(sources) == 0 {
			resource.Error = "font descriptors are invalid"
			failures = append(failures, resource.Error+": "+resource.Family)
			resources = append(resources, resource)
			continue
		}
		for _, source := range sources {
			resource.URL, resource.Format = source.url.String(), source.format
			loadContext, cancel := context.WithTimeout(pageContext, fontLoadTimeout(rule.Display))
			response, err := fetchFontResource(loadContext, client, pageURL, source.url)
			cancel()
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					resource.Error = "font load timed out"
				} else {
					resource.Error = err.Error()
				}
				continue
			}
			finalURL := response.URL
			if finalURL == nil {
				finalURL = source.url
			}
			resource.FinalURL = finalURL.String()
			if err := validateFontResponse(pageURL, finalURL, response, source.format, totalBytes); err != nil {
				resource.Error = err.Error()
				continue
			}
			face, decoded, err := decodeWebFont(response.Body, source.format)
			if err != nil {
				resource.Error = "font decode failed"
				continue
			}
			totalBytes += len(response.Body)
			resource.Face, resource.Loaded, resource.Decoded, resource.Error = face, true, decoded, ""
			break
		}
		if !resource.Loaded {
			if resource.Error == "" {
				resource.Error = "font source could not be loaded"
			}
			failures = append(failures, resource.Error+": "+resource.Family)
		}
		resources = append(resources, resource)
	}
	return resources, failures
}

func fontLoadTimeout(display string) time.Duration {
	switch strings.ToLower(strings.TrimSpace(display)) {
	case "fallback":
		return fontFallbackWait
	case "optional":
		return fontOptionalWait
	default:
		return fontBlockTimeout
	}
}

func layoutWebFonts(resources []FontResource) *layoutmodel.FontSet {
	faces := make([]layoutmodel.WebFontFace, 0, len(resources))
	for _, resource := range resources {
		if !resource.Decoded || resource.Face == nil {
			continue
		}
		ranges := make([]layoutmodel.FontRange, len(resource.UnicodeRanges))
		for index, interval := range resource.UnicodeRanges {
			ranges[index] = layoutmodel.FontRange{Start: interval.Start, End: interval.End}
		}
		faces = append(faces, layoutmodel.WebFontFace{
			Family: resource.Family, Style: resource.Style, Weight: resource.Weight,
			UnicodeRanges: ranges, Face: resource.Face,
		})
	}
	return layoutmodel.NewFontSet(faces)
}

func fetchFontResource(ctx context.Context, client ResourceLoader, pageURL, target *url.URL) (*network.Response, error) {
	if target == nil || target.Scheme != "http" && target.Scheme != "https" {
		return nil, errors.New("font URL is not HTTP(S)")
	}
	if pageURL.Scheme == "https" && target.Scheme != "https" {
		return nil, errors.New("mixed-content font was blocked")
	}
	if requestClient, ok := client.(requestLoader); ok {
		return requestClient.Do(ctx, &network.Request{
			Method: http.MethodGet, URL: target, SiteURL: pageURL, Kind: network.RequestFont,
			CORS: !network.SameOrigin(pageURL, target), Credentials: network.CredentialsSameOrigin,
		})
	}
	return client.Get(ctx, target)
}

func validateFontResponse(pageURL, finalURL *url.URL, response *network.Response, format string, currentBytes int) error {
	if response == nil || finalURL == nil || pageURL.Scheme == "https" && finalURL.Scheme != "https" {
		return errors.New("font redirect or mixed-content policy rejected response")
	}
	if len(response.Body) == 0 || len(response.Body) > maxFontBytes || currentBytes > maxPageFontBytes-len(response.Body) {
		return errors.New("font size limit exceeded")
	}
	if !network.SameOrigin(pageURL, finalURL) {
		allowed := ""
		if response.Header != nil {
			allowed = strings.TrimSpace(response.Header.Get("Access-Control-Allow-Origin"))
		}
		origin, originErr := network.OriginFromURL(pageURL)
		if originErr != nil || allowed != "*" && allowed != origin.String() {
			return errors.New("font CORS policy rejected response")
		}
	}
	mediaType, _, err := mime.ParseMediaType(response.ContentType)
	if err != nil || !fontMIMEMatches(mediaType, format) {
		return errors.New("font MIME type was rejected")
	}
	return nil
}

func fontMIMEMatches(mediaType, format string) bool {
	switch format {
	case "woff":
		return mediaType == "font/woff" || mediaType == "application/font-woff"
	case "woff2":
		return mediaType == "font/woff2" || mediaType == "application/font-woff2"
	default:
		return false
	}
}

func decodeWebFont(source []byte, format string) (*textfont.Face, bool, error) {
	switch format {
	case "woff":
		if len(source) < 44 || string(source[:4]) != "wOFF" {
			return nil, false, errors.New("invalid WOFF signature")
		}
		face, err := textfont.ParseTTF(bytes.NewReader(source))
		return face, err == nil, err
	case "woff2":
		if err := validateWOFF2(source); err != nil {
			return nil, false, err
		}
		decoded, err := woff2.DecodeBytes(source)
		if err != nil || len(decoded) == 0 || len(decoded) > 32<<20 {
			return nil, false, errors.New("invalid WOFF2 payload")
		}
		face, err := textfont.ParseTTF(bytes.NewReader(decoded))
		return face, err == nil, err
	default:
		return nil, false, errors.New("unsupported web font format")
	}
}

func validateWOFF2(source []byte) error {
	if len(source) < 48 || string(source[:4]) != "wOF2" || binary.BigEndian.Uint32(source[8:12]) != uint32(len(source)) {
		return errors.New("invalid WOFF2 header")
	}
	numTables := binary.BigEndian.Uint16(source[12:14])
	totalSFNT := binary.BigEndian.Uint32(source[16:20])
	compressed := binary.BigEndian.Uint32(source[20:24])
	if numTables == 0 || numTables > 256 || totalSFNT == 0 || totalSFNT > 32<<20 || compressed == 0 || int(compressed) > len(source)-48 {
		return errors.New("invalid WOFF2 table bounds")
	}
	return nil
}

func parseFontSources(raw string, baseURL *url.URL) []fontSource {
	var result []fontSource
	for _, item := range splitImageList(raw) {
		lower := strings.ToLower(item)
		urlStart := strings.Index(lower, "url(")
		if urlStart < 0 {
			continue
		}
		urlEnd := strings.IndexByte(item[urlStart+4:], ')')
		if urlEnd < 0 {
			continue
		}
		urlEnd += urlStart + 4
		rawURL := strings.TrimSpace(item[urlStart+4 : urlEnd])
		if decoded, ok := css.DecodeString(rawURL); ok {
			rawURL = decoded
		}
		target, err := url.Parse(rawURL)
		if err != nil || target.String() == "" {
			continue
		}
		target = baseURL.ResolveReference(target)
		format := ""
		if formatStart := strings.Index(strings.ToLower(item[urlEnd+1:]), "format("); formatStart >= 0 {
			formatStart += urlEnd + 1
			formatEnd := strings.IndexByte(item[formatStart+7:], ')')
			if formatEnd >= 0 {
				formatEnd += formatStart + 7
				format = strings.Trim(strings.ToLower(strings.TrimSpace(item[formatStart+7:formatEnd])), `"'`)
			}
		}
		if format == "" {
			path := strings.ToLower(target.Path)
			if strings.HasSuffix(path, ".woff2") {
				format = "woff2"
			} else if strings.HasSuffix(path, ".woff") {
				format = "woff"
			}
		}
		if format == "woff" || format == "woff2" {
			result = append(result, fontSource{url: target, format: format})
		}
	}
	return result
}

func validateFontDescriptors(rule css.FontFaceRule) error {
	styleValue := strings.ToLower(strings.TrimSpace(rule.Style))
	if styleValue != "normal" && styleValue != "italic" && !strings.HasPrefix(styleValue, "oblique") {
		return errors.New("invalid font-style")
	}
	weight := strings.ToLower(strings.TrimSpace(rule.Weight))
	if weight != "normal" && weight != "bold" {
		value, err := strconv.Atoi(weight)
		if err != nil || value < 1 || value > 1000 {
			return errors.New("invalid font-weight")
		}
	}
	validStretch := map[string]bool{"normal": true, "ultra-condensed": true, "extra-condensed": true, "condensed": true, "semi-condensed": true, "semi-expanded": true, "expanded": true, "extra-expanded": true, "ultra-expanded": true}
	if !validStretch[strings.ToLower(strings.TrimSpace(rule.Stretch))] {
		return errors.New("invalid font-stretch")
	}
	validDisplay := map[string]bool{"auto": true, "block": true, "swap": true, "fallback": true, "optional": true}
	if !validDisplay[strings.ToLower(strings.TrimSpace(rule.Display))] {
		return errors.New("invalid font-display")
	}
	return nil
}

func parseFontUnicodeRanges(raw string) ([]FontRange, error) {
	if strings.TrimSpace(raw) == "" {
		return []FontRange{{Start: 0, End: 0x10ffff}}, nil
	}
	var result []FontRange
	for _, item := range strings.Split(raw, ",") {
		item = strings.ToUpper(strings.TrimSpace(item))
		if !strings.HasPrefix(item, "U+") {
			return nil, errors.New("invalid unicode-range")
		}
		value := strings.TrimPrefix(item, "U+")
		if strings.Contains(value, "?") {
			if strings.Contains(value, "-") || len(value) > 6 {
				return nil, errors.New("invalid unicode-range wildcard")
			}
			startText, endText := strings.ReplaceAll(value, "?", "0"), strings.ReplaceAll(value, "?", "F")
			start, startErr := strconv.ParseInt(startText, 16, 32)
			end, endErr := strconv.ParseInt(endText, 16, 32)
			if startErr != nil || endErr != nil || end > 0x10ffff {
				return nil, errors.New("invalid unicode-range wildcard")
			}
			result = append(result, FontRange{Start: rune(start), End: rune(end)})
			continue
		}
		parts := strings.Split(value, "-")
		if len(parts) > 2 {
			return nil, errors.New("invalid unicode-range interval")
		}
		start, err := strconv.ParseInt(parts[0], 16, 32)
		end := start
		if len(parts) == 2 {
			end, err = strconv.ParseInt(parts[1], 16, 32)
		}
		if err != nil || start < 0 || start > end || end > 0x10ffff {
			return nil, errors.New("invalid unicode-range interval")
		}
		result = append(result, FontRange{Start: rune(start), End: rune(end)})
	}
	if len(result) == 0 || len(result) > 256 {
		return nil, fmt.Errorf("unicode-range count is invalid")
	}
	return result, nil
}
