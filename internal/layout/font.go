package layout

import (
	"math"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

var (
	fontOnce    sync.Once
	regularFont *opentype.Font
	boldFont    *opentype.Font
)

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

func usedLineMetrics(run inlineRun) (height, ascent float32) {
	if run.flex && run.height > 0 {
		return run.height, min(max(run.baseline, float32(0)), run.height)
	}
	_, measuredHeight, measuredAscent := measureText("Mg", run.style.fontSize, run.style.bold)
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
