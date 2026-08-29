package layout

import (
	"math"
	"strings"
	"sync"
	"unicode"

	giofont "gioui.org/font"
	giogofont "gioui.org/font/gofont"
	"gioui.org/text"
	"github.com/go-text/typesetting/di"
	textfont "github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

var (
	fontOnce    sync.Once
	regularFont *opentype.Font
	boldFont    *opentype.Font
)

// FontRange is one inclusive unicode-range interval for a web font face.
type FontRange struct{ Start, End rune }

// WebFontFace is a decoded face and the descriptors used to select it.
type WebFontFace struct {
	Family, Style, Weight string
	UnicodeRanges         []FontRange
	Face                  *textfont.Face
}

// FontSet is an immutable page-scoped collection of decoded web fonts.
// Shaping is serialized because HarfbuzzShaper retains reusable buffers.
type FontSet struct {
	faces        []WebFontFace
	mu           sync.Mutex
	shaper       shaping.HarfbuzzShaper
	systemShaper *text.Shaper
}

// NewFontSet creates a page-scoped font collection. Invalid faces are omitted.
func NewFontSet(faces []WebFontFace) *FontSet {
	filtered := filterWebFontFaces(faces)
	if len(filtered) == 0 {
		return nil
	}
	return &FontSet{faces: filtered}
}

// NewFontSetWithSystemFallback creates a page-scoped font set whose shaping
// falls back from decoded Web Fonts and bundled Go fonts to operating-system
// glyph coverage. The set remains usable when system discovery fails.
func NewFontSetWithSystemFallback(faces []WebFontFace) *FontSet {
	return newFontSetWithSystemFallback(faces, true)
}

func newFontSetWithSystemFallback(faces []WebFontFace, systemFonts bool) *FontSet {
	filtered := filterWebFontFaces(faces)
	// Web Font selection remains in selectFace so unicode-range descriptors are
	// honored. The fallback shaper owns only deterministic bundled faces plus
	// system faces for text not fully covered by the selected Web Font.
	collection := append([]giofont.FontFace(nil), giogofont.Collection()...)
	options := []text.ShaperOption{text.WithCollection(collection)}
	if !systemFonts {
		options = append(options, text.NoSystemFonts())
	}
	return &FontSet{faces: filtered, systemShaper: text.NewShaper(options...)}
}

func filterWebFontFaces(faces []WebFontFace) []WebFontFace {
	filtered := make([]WebFontFace, 0, len(faces))
	for _, face := range faces {
		if face.Face == nil || strings.TrimSpace(face.Family) == "" {
			continue
		}
		face.UnicodeRanges = append([]FontRange(nil), face.UnicodeRanges...)
		filtered = append(filtered, face)
	}
	return filtered
}

func (fonts *FontSet) measure(text string, style blockStyle) (width, height, ascent float32, ok bool) {
	if fonts != nil && fonts.systemShaper != nil {
		if face := fonts.selectFace(text, style); face != nil {
			return fonts.measureFace(text, style, face)
		}
		return fonts.measureWithSystemFallback(text, style)
	}
	face := fonts.selectFace(text, style)
	if face == nil || text == "" || style.fontSize <= 0 {
		return 0, 0, 0, false
	}
	return fonts.measureFace(text, style, face)
}

func (fonts *FontSet) measureFace(text string, style blockStyle, face *textfont.Face) (width, height, ascent float32, ok bool) {
	if face == nil || text == "" || style.fontSize <= 0 {
		return 0, 0, 0, false
	}
	runes := []rune(text)
	script := language.Unknown
	for _, character := range runes {
		candidate := language.LookupScript(character)
		if candidate != language.Common && candidate != language.Inherited && candidate != language.Unknown {
			script = candidate
			break
		}
	}
	fonts.mu.Lock()
	output := fonts.shaper.Shape(shaping.Input{
		Text: runes, RunStart: 0, RunEnd: len(runes), Direction: di.DirectionLTR,
		Face: face, Size: fixed.Int26_6(style.fontSize * 64), Script: script,
	})
	fonts.mu.Unlock()
	extents, hasExtents := face.FontHExtents()
	if !hasExtents || face.Upem() == 0 {
		return float32(output.Advance) / 64, style.fontSize * 1.2, style.fontSize * .9, true
	}
	scale := style.fontSize / float32(face.Upem())
	ascent = extents.Ascender * scale
	height = (extents.Ascender - extents.Descender + extents.LineGap) * scale
	if height <= 0 || ascent <= 0 {
		height, ascent = style.fontSize*1.2, style.fontSize*.9
	}
	return float32(output.Advance) / 64, height, ascent, true
}

func (fonts *FontSet) measureWithSystemFallback(value string, style blockStyle) (width, height, ascent float32, ok bool) {
	if value == "" || style.fontSize <= 0 {
		return 0, 0, 0, false
	}
	description := giofont.Font{Weight: giofont.Normal}
	if len(style.fontFamilies) != 0 {
		description.Typeface = giofont.Typeface(strings.Trim(strings.TrimSpace(style.fontFamilies[0]), `"'`))
	}
	if style.bold {
		description.Weight = giofont.Bold
	}
	if style.fontStyle == "italic" || strings.HasPrefix(style.fontStyle, "oblique") {
		description.Style = giofont.Italic
	}
	fonts.mu.Lock()
	fonts.systemShaper.LayoutString(text.Parameters{
		Font: description, PxPerEm: fixed.Int26_6(style.fontSize * 64), MaxLines: 1,
		MaxWidth: 1 << 30, DisableSpaceTrim: true,
	}, value)
	count, runeCount := 0, 0
	for {
		glyph, exists := fonts.systemShaper.NextGlyph()
		if !exists {
			break
		}
		count++
		runeCount += int(glyph.Runes)
		width = max(width, float32(glyph.X+glyph.Advance)/64)
		ascent = max(ascent, float32(glyph.Ascent)/64)
		height = max(height, float32(glyph.Ascent+glyph.Descent)/64)
		if count > len([]rune(value))*4+8 {
			fonts.mu.Unlock()
			return 0, 0, 0, false
		}
	}
	fonts.mu.Unlock()
	if count == 0 || runeCount != len([]rune(value)) || !finiteFontMetric(width) || !finiteFontMetric(height) || !finiteFontMetric(ascent) {
		return 0, 0, 0, false
	}
	if height <= 0 || ascent <= 0 {
		height, ascent = style.fontSize*1.2, style.fontSize*.9
	}
	return width, height, ascent, true
}

func finiteFontMetric(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0) && value >= 0
}

func (fonts *FontSet) selectFace(text string, style blockStyle) *textfont.Face {
	if fonts == nil {
		return nil
	}
	for _, family := range style.fontFamilies {
		family = strings.Trim(strings.TrimSpace(family), `"'`)
		for index := range fonts.faces {
			candidate := &fonts.faces[index]
			if !strings.EqualFold(candidate.Family, family) || !fontStyleMatches(candidate.Style, style.fontStyle) || !fontWeightMatches(candidate.Weight, style.bold) {
				continue
			}
			if fontCoversText(candidate, text) {
				return candidate.Face
			}
		}
	}
	return nil
}

func fontStyleMatches(candidate, requested string) bool {
	candidate, requested = strings.ToLower(strings.TrimSpace(candidate)), strings.ToLower(strings.TrimSpace(requested))
	if requested == "" {
		requested = "normal"
	}
	if strings.HasPrefix(requested, "oblique") {
		return strings.HasPrefix(candidate, "oblique")
	}
	return candidate == requested
}

func fontWeightMatches(candidate string, bold bool) bool {
	candidate = strings.ToLower(strings.TrimSpace(candidate))
	candidateBold := candidate == "bold"
	if !candidateBold && candidate != "normal" {
		var numeric int
		for _, character := range candidate {
			if character < '0' || character > '9' {
				numeric = 0
				break
			}
			numeric = numeric*10 + int(character-'0')
		}
		candidateBold = numeric >= 600
	}
	return candidateBold == bold
}

func fontCoversText(candidate *WebFontFace, text string) bool {
	for _, character := range text {
		covered := len(candidate.UnicodeRanges) == 0
		for _, interval := range candidate.UnicodeRanges {
			if character >= interval.Start && character <= interval.End {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
		if _, exists := candidate.Face.NominalGlyph(character); !exists {
			return false
		}
	}
	return true
}

func loadFonts() {
	regularFont, _ = opentype.Parse(goregular.TTF)
	boldFont, _ = opentype.Parse(gobold.TTF)
}

func measureText(text string, size float32, bold bool) (width, height, ascent float32) {
	fontOnce.Do(loadFonts)
	parsed := regularFont
	if bold {
		parsed = boldFont
	}
	if parsed == nil || size <= 0 {
		return 0, size * 1.2, size * 0.9
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return 0, size * 1.2, size * 0.9
	}
	defer face.Close()
	metrics := face.Metrics()
	return float32(font.MeasureString(face, text)) / 64,
		float32(metrics.Height) / 64, float32(metrics.Ascent) / 64
}

func measureStyledText(text string, style blockStyle) (width, height, ascent float32) {
	var measured bool
	if style.fonts != nil {
		width, height, ascent, measured = style.fonts.measure(text, style)
	}
	if !measured {
		width, height, ascent = measureText(text, style.fontSize, style.bold)
	}
	runes := []rune(text)
	if len(runes) > 1 {
		width += float32(len(runes)-1) * style.letterSpacing
	}
	for _, character := range runes {
		if unicode.IsSpace(character) {
			width += style.wordSpacing
		}
	}
	width *= fontStretchScale(style.fontStretch)
	return max(width, float32(0)), height, ascent
}

func fontStretchScale(value string) float32 {
	switch value {
	case "ultra-condensed":
		return .5
	case "extra-condensed":
		return .625
	case "condensed":
		return .75
	case "semi-condensed":
		return .875
	case "semi-expanded":
		return 1.125
	case "expanded":
		return 1.25
	case "extra-expanded":
		return 1.5
	case "ultra-expanded":
		return 2
	default:
		return 1
	}
}

func usedLineMetrics(run inlineRun) (height, ascent float32) {
	if run.flex && run.height > 0 {
		return run.height, min(max(run.baseline, float32(0)), run.height)
	}
	_, measuredHeight, measuredAscent := measureStyledText("Mg", run.style)
	height = measuredHeight
	if run.style.lineHeight > 0 {
		height = run.style.lineHeight
	}
	leading := max(height-measuredHeight, float32(0))
	ascent = measuredAscent + leading/2
	if run.height > height {
		height = run.height
	}
	if math.IsNaN(float64(height)) || math.IsInf(float64(height), 0) {
		return 0, 0
	}
	return height, ascent
}
