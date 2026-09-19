package realsitecompat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/dom"
	layoutmodel "github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	paintmodel "github.com/Grove-Computing/Growse/internal/paint"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

type visualEvidence struct {
	Case            string         `json:"case"`
	Viewport        string         `json:"viewport"`
	Width           int            `json:"width"`
	Height          int            `json:"height"`
	DisplayedBoxes  int            `json:"displayedBoxes"`
	PaintCommands   int            `json:"paintCommands"`
	DisplayedNodes  int            `json:"displayedNodes"`
	Stylesheets     int            `json:"stylesheets"`
	CSSRules        int            `json:"cssRules"`
	Glyphs          int            `json:"glyphs"`
	DifferenceRatio float64        `json:"differenceRatio"`
	Regions         []regionMetric `json:"regions"`
	Issues          []visualIssue  `json:"issues,omitempty"`
}

type regionMetric struct {
	Name         string  `json:"name"`
	NodeID       uint64  `json:"nodeId"`
	X            float32 `json:"x"`
	Y            float32 `json:"y"`
	Width        float32 `json:"width"`
	Height       float32 `json:"height"`
	VisibleBoxes int     `json:"visibleBoxes"`
	TextBytes    int     `json:"textBytes"`
}

type visualIssue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Region   string `json:"region"`
	Detail   string `json:"detail"`
}

type regionObservation struct {
	Metric regionMetric
	Boxes  []layoutmodel.Box
}

func TestRealSiteCorpusProducesGrowseReferenceDiffAndRegionArtifacts(t *testing.T) {
	manifest := readCorpusManifest(t)
	server := httptest.NewServer(http.FileServer(http.Dir(".")))
	defer server.Close()
	engine := browser.New(network.NewClientWithLimits(server.Client(), 8<<20))
	defer engine.Close()
	artifactDirectory := os.Getenv("GROWSE_REAL_SITE_ARTIFACT_DIR")

	for _, fixture := range manifest.Cases {
		for _, viewport := range manifest.Viewports {
			name := fixture.ID + "-" + viewport.Name
			t.Run(name, func(t *testing.T) {
				_, err := engine.Navigate(context.Background(), server.URL+"/"+fixture.Fixture)
				if err != nil {
					t.Fatal(err)
				}
				engine.UpdateViewport(float32(viewport.Width), float32(viewport.Height))
				page := engine.Page()
				tree := layoutmodel.BuildWithScrollAndResources(page.Document, page.ComputedStyles, page.ImageResources, page.WebFonts, float32(viewport.Width), float32(viewport.Height), 0, 0)
				list := paintmodel.Build(tree)
				growse := rasterDisplayList(list, viewport.Width, viewport.Height)
				reference := readPNG(t, filepath.Join("testdata", "reference", name+".png"))
				difference, ratio := diffImages(growse, reference)
				regions, issues := analyzeRegions(page.Document, tree, fixture.Regions)
				glyphs := 0
				for _, box := range tree.Boxes {
					glyphs += utf8.RuneCountInString(box.Text)
				}
				stylesheets, rules := 0, 0
				if page.Stylesheet != nil {
					stylesheets = 1 + len(page.Stylesheet.Imports)
					rules = len(page.Stylesheet.Rules)
				}
				metric := visualEvidence{
					Case: fixture.ID, Viewport: viewport.Name, Width: viewport.Width, Height: viewport.Height,
					DisplayedBoxes: len(tree.Boxes), PaintCommands: len(list.Commands), DifferenceRatio: ratio,
					DisplayedNodes: len(tree.Decorations), Stylesheets: stylesheets, CSSRules: rules, Glyphs: glyphs,
					Regions: regions, Issues: issues,
				}
				if err := validateEvidenceLimits(metric, manifest); err != nil {
					t.Fatal(err)
				}
				if len(metric.Regions) < len(fixture.Regions) || metric.DisplayedBoxes == 0 || metric.PaintCommands == 0 {
					t.Fatalf("incomplete semantic evidence: %#v", metric)
				}
				writeEvidenceArtifact(t, name+"-growse.png", growse)
				writeEvidenceArtifact(t, name+"-reference.png", reference)
				writeEvidenceArtifact(t, name+"-diff.png", difference)
				writeEvidenceJSON(t, name+"-metrics.json", metric)
			})
		}
	}
	if artifactDirectory != "" {
		if err := validateArtifactDirectory(artifactDirectory, int64(manifest.Limits.MaxArtifactBytes), int64(manifest.Limits.MaxCorpusArtifactBytes)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOneMBClubTableKeepsColumnsAndCellBaselinesAligned(t *testing.T) {
	server := httptest.NewServer(http.FileServer(http.Dir(".")))
	defer server.Close()
	engine := browser.New(network.NewClientWithLimits(server.Client(), 8<<20))
	defer engine.Close()
	if _, err := engine.Navigate(context.Background(), server.URL+"/fixtures/one-mb-club.html"); err != nil {
		t.Fatal(err)
	}
	page := engine.Page()
	tree := layoutmodel.BuildWithViewport(page.Document, page.ComputedStyles, 1280, 900)
	values := make(map[string]layoutmodel.Box)
	for _, box := range tree.Boxes {
		text := strings.TrimSpace(box.Text)
		if text == "0.3" || text == "0.9" || text == "3.5" {
			values[text] = box
		}
	}
	if len(values) != 3 {
		t.Fatalf("size column text boxes = %#v", values)
	}
	first := values["0.3"]
	for _, value := range []string{"0.9", "3.5"} {
		box := values[value]
		if difference := box.X - first.X; difference < -0.01 || difference > 0.01 {
			t.Fatalf("size column is not aligned: 0.3=%#v %s=%#v", first, value, box)
		}
		if box.Baseline <= box.Y || box.Baseline > box.Y+box.Height {
			t.Fatalf("cell baseline is outside %s: %#v", value, box)
		}
	}
}

func TestSchemescapeInlineIconsAndLinksDoNotTriggerStructuralOverlap(t *testing.T) {
	server := httptest.NewServer(http.FileServer(http.Dir(".")))
	defer server.Close()
	engine := browser.New(network.NewClientWithLimits(server.Client(), 8<<20))
	defer engine.Close()
	if _, err := engine.Navigate(context.Background(), server.URL+"/fixtures/schemescape.html"); err != nil {
		t.Fatal(err)
	}
	page := engine.Page()
	tree := layoutmodel.BuildWithScrollAndResources(page.Document, page.ComputedStyles, page.ImageResources, page.WebFonts, 1280, 800, 0, 0)
	_, issues := analyzeRegions(page.Document, tree, []string{"main", "list"})
	if len(issues) != 0 {
		for index, box := range tree.Boxes {
			t.Logf("box[%d]=node:%d tag:%s text:%q rect:%v,%v %vx%v", index, box.NodeID, box.Tag, box.Text, box.X, box.Y, box.Width, box.Height)
		}
		t.Fatalf("Schemescape structural issues = %#v", issues)
	}
}

func validateEvidenceLimits(evidence visualEvidence, manifest corpusManifest) error {
	pixels := int64(evidence.Width) * int64(evidence.Height)
	switch {
	case evidence.Width <= 0 || evidence.Height <= 0 || pixels > int64(manifest.Limits.MaxPixels):
		return fmt.Errorf("%s/%s pixels %d exceed limit %d", evidence.Case, evidence.Viewport, pixels, manifest.Limits.MaxPixels)
	case evidence.DisplayedNodes > manifest.Limits.MaxDisplayedElements:
		return fmt.Errorf("%s/%s displayed nodes %d exceed limit %d", evidence.Case, evidence.Viewport, evidence.DisplayedNodes, manifest.Limits.MaxDisplayedElements)
	case evidence.Stylesheets > manifest.Limits.MaxStylesheets:
		return fmt.Errorf("%s/%s stylesheets %d exceed limit %d", evidence.Case, evidence.Viewport, evidence.Stylesheets, manifest.Limits.MaxStylesheets)
	case evidence.CSSRules > manifest.Limits.MaxCSSRules:
		return fmt.Errorf("%s/%s CSS rules %d exceed limit %d", evidence.Case, evidence.Viewport, evidence.CSSRules, manifest.Limits.MaxCSSRules)
	case evidence.Glyphs > manifest.Limits.MaxGlyphs:
		return fmt.Errorf("%s/%s glyphs %d exceed limit %d", evidence.Case, evidence.Viewport, evidence.Glyphs, manifest.Limits.MaxGlyphs)
	default:
		return nil
	}
}

func validateArtifactDirectory(directory string, maxSingle, maxTotal int64) error {
	var total int64
	err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxSingle {
			return fmt.Errorf("artifact %s is %d bytes, limit %d", filepath.Base(path), info.Size(), maxSingle)
		}
		total += info.Size()
		if total > maxTotal {
			return fmt.Errorf("corpus artifacts are %d bytes, limit %d", total, maxTotal)
		}
		return nil
	})
	return err
}

func analyzeRegions(document *dom.Document, tree *layoutmodel.Tree, required []string) ([]regionMetric, []visualIssue) {
	var metrics []regionMetric
	observations := make(map[string][]regionObservation)
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil {
			return
		}
		if name, ok := node.Attribute("data-region"); ok {
			bounds := tree.Bounds[node.ID]
			metric := regionMetric{Name: name, NodeID: uint64(node.ID), X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: bounds.Height}
			observation := regionObservation{Metric: metric}
			for _, box := range tree.Boxes {
				if nodeContains(document, node, box.NodeID) && !box.Hidden && box.Width > 0 && box.Height > 0 {
					metric.VisibleBoxes++
					metric.TextBytes += len(strings.TrimSpace(box.Text))
					observation.Boxes = append(observation.Boxes, box)
				}
			}
			observation.Metric = metric
			metrics = append(metrics, metric)
			observations[name] = append(observations[name], observation)
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(document.Root)
	return metrics, classifyStructuralIssues(document, required, observations)
}

func classifyStructuralIssues(document *dom.Document, required []string, observations map[string][]regionObservation) []visualIssue {
	var issues []visualIssue
	for _, name := range required {
		candidates := observations[name]
		if len(candidates) == 0 {
			issues = append(issues, visualIssue{Severity: "P0", Code: "region-missing", Region: name, Detail: "required semantic region is absent"})
			continue
		}
		visible := false
		for _, candidate := range candidates {
			metric := candidate.Metric
			if metric.Width > 0 && metric.Height > 0 && metric.VisibleBoxes > 0 {
				visible = true
			}
			region := layoutmodel.Rect{X: metric.X, Y: metric.Y, Width: metric.Width, Height: metric.Height}
			for index, box := range candidate.Boxes {
				if strings.TrimSpace(box.Text) == "" {
					continue
				}
				if !rectContains(region, layoutmodel.Rect{X: box.X, Y: box.Y, Width: box.Width, Height: box.Height}, 1) {
					issues = appendBoundedIssue(issues, visualIssue{Severity: "P1", Code: "text-clipped", Region: name, Detail: fmt.Sprintf("text box %d exceeds region bounds", index)})
				}
				for other := index + 1; other < len(candidate.Boxes); other++ {
					next := candidate.Boxes[other]
					if strings.TrimSpace(next.Text) != "" && !nodesAreAncestorRelated(document, box.NodeID, next.NodeID) && boxesStructurallyOverlap(box, next) {
						issues = appendBoundedIssue(issues, visualIssue{Severity: "P1", Code: "text-overlap", Region: name, Detail: fmt.Sprintf("text boxes %d and %d overlap", index, other)})
					}
				}
			}
		}
		if !visible {
			issues = append(issues, visualIssue{Severity: "P0", Code: "region-empty", Region: name, Detail: "required semantic region has no visible boxes"})
		}
	}
	return issues
}

func nodesAreAncestorRelated(document *dom.Document, leftID, rightID dom.NodeID) bool {
	if document == nil {
		return false
	}
	if leftID == rightID {
		return leftID == rightID
	}
	left, leftOK := document.NodeByID(leftID)
	right, rightOK := document.NodeByID(rightID)
	if !leftOK || !rightOK {
		return false
	}
	for current := left.Parent; current != nil; current = current.Parent {
		if current == right {
			return true
		}
	}
	for current := right.Parent; current != nil; current = current.Parent {
		if current == left {
			return true
		}
	}
	return false
}

func appendBoundedIssue(issues []visualIssue, issue visualIssue) []visualIssue {
	if len(issues) >= 256 {
		return issues
	}
	return append(issues, issue)
}

func rectContains(outer, inner layoutmodel.Rect, tolerance float32) bool {
	return inner.X >= outer.X-tolerance && inner.Y >= outer.Y-tolerance && inner.X+inner.Width <= outer.X+outer.Width+tolerance && inner.Y+inner.Height <= outer.Y+outer.Height+tolerance
}

func boxesStructurallyOverlap(left, right layoutmodel.Box) bool {
	intersectionWidth := min(left.X+left.Width, right.X+right.Width) - max(left.X, right.X)
	intersectionHeight := min(left.Y+left.Height, right.Y+right.Height) - max(left.Y, right.Y)
	if intersectionWidth <= 1 || intersectionHeight <= 1 {
		return false
	}
	// Inline fragments sharing one line may touch in the cross axis. Only a
	// substantial two-dimensional collision is a structural overlap.
	return intersectionWidth > min(left.Width, right.Width)*0.15 && intersectionHeight > min(left.Height, right.Height)*0.25
}

func nodeContains(document *dom.Document, ancestor *dom.Node, id dom.NodeID) bool {
	node, ok := document.NodeByID(id)
	if !ok {
		return false
	}
	for current := node; current != nil; current = current.Parent {
		if current == ancestor {
			return true
		}
	}
	return false
}

func rasterDisplayList(list *paintmodel.DisplayList, width, height int) *image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(rgba(list.Background, 1)), image.Point{}, draw.Src)
	for _, candidate := range list.Commands {
		switch command := candidate.(type) {
		case paintmodel.DrawBox:
			for _, shadow := range command.BoxShadows {
				if shadow.Inset {
					continue
				}
				spread := shadow.Spread + shadow.Blur/2
				drawRoundedRect(canvas, command.X+shadow.OffsetX-spread, command.Y+shadow.OffsetY-spread, command.Width+2*spread, command.Height+2*spread, expandRadii(command.Radius, spread), rgba(shadow.Color, command.Opacity))
			}
			drawRoundedRect(canvas, command.X, command.Y, command.Width, command.Height, command.Radius, rgba(command.Color, command.Opacity))
			drawRasterBorders(canvas, command)
		case paintmodel.DrawText:
			if len(command.Runs) == 0 {
				drawRasterText(canvas, command.Text, command.X, command.Y+command.Baseline, command.FontSize, command.Bold, command.Color, command.Opacity)
				continue
			}
			cursor := command.X
			for _, run := range command.Runs {
				if !run.Atomic && run.Text != "" {
					drawRasterText(canvas, run.Text, cursor+run.OffsetX, command.Y+run.Baseline+run.OffsetY, run.FontSize, run.Bold, run.Color, command.Opacity*run.Opacity)
				}
				cursor += run.Width
			}
		}
	}
	return canvas
}

func drawRasterBorders(canvas draw.Image, command paintmodel.DrawBox) {
	mask := roundedRectMask(command.X, command.Y, command.Width, command.Height, command.Radius)
	drawSide := func(x, y, width, height float32, side stylemodel.BorderSide) {
		if side.Width <= 0 || side.Style == stylemodel.BorderNone || uint8(side.Color) == 0 {
			return
		}
		rectangle := image.Rect(int(x), int(y), int(x+width+.5), int(y+height+.5)).Intersect(canvas.Bounds())
		if !rectangle.Empty() {
			draw.DrawMask(canvas, rectangle, image.NewUniform(rgba(side.Color, command.Opacity)), image.Point{}, mask, rectangle.Min, draw.Over)
		}
	}
	drawSide(command.X, command.Y, command.Width, command.Border.Top.Width, command.Border.Top)
	drawSide(command.X+command.Width-command.Border.Right.Width, command.Y, command.Border.Right.Width, command.Height, command.Border.Right)
	drawSide(command.X, command.Y+command.Height-command.Border.Bottom.Width, command.Width, command.Border.Bottom.Width, command.Border.Bottom)
	drawSide(command.X, command.Y, command.Border.Left.Width, command.Height, command.Border.Left)
}

func drawRoundedRect(destination draw.Image, x, y, width, height float32, radius layoutmodel.BorderRadii, fill color.NRGBA) {
	if width <= 0 || height <= 0 || fill.A == 0 {
		return
	}
	rectangle := image.Rect(int(x), int(y), int(x+width+.5), int(y+height+.5)).Intersect(destination.Bounds())
	if rectangle.Empty() {
		return
	}
	mask := roundedRectMask(x, y, width, height, radius)
	draw.DrawMask(destination, rectangle, image.NewUniform(fill), image.Point{}, mask, rectangle.Min, draw.Over)
}

func roundedRectMask(x, y, width, height float32, radius layoutmodel.BorderRadii) *image.Alpha {
	bounds := image.Rect(int(x), int(y), int(x+width+.5), int(y+height+.5))
	mask := image.NewAlpha(bounds)
	for py := bounds.Min.Y; py < bounds.Max.Y; py++ {
		for px := bounds.Min.X; px < bounds.Max.X; px++ {
			if rasterPointInsideRoundedRect(float32(px)+.5, float32(py)+.5, x, y, width, height, radius) {
				mask.SetAlpha(px, py, color.Alpha{A: 255})
			}
		}
	}
	return mask
}

func rasterPointInsideRoundedRect(px, py, x, y, width, height float32, radius layoutmodel.BorderRadii) bool {
	if px < x || py < y || px >= x+width || py >= y+height {
		return false
	}
	corners := []struct {
		radius layoutmodel.CornerRadius
		cx, cy float32
		corner bool
	}{
		{radius.TopLeft, x + radius.TopLeft.X, y + radius.TopLeft.Y, px < x+radius.TopLeft.X && py < y+radius.TopLeft.Y},
		{radius.TopRight, x + width - radius.TopRight.X, y + radius.TopRight.Y, px > x+width-radius.TopRight.X && py < y+radius.TopRight.Y},
		{radius.BottomRight, x + width - radius.BottomRight.X, y + height - radius.BottomRight.Y, px > x+width-radius.BottomRight.X && py > y+height-radius.BottomRight.Y},
		{radius.BottomLeft, x + radius.BottomLeft.X, y + height - radius.BottomLeft.Y, px < x+radius.BottomLeft.X && py > y+height-radius.BottomLeft.Y},
	}
	for _, corner := range corners {
		if !corner.corner || corner.radius.X <= 0 || corner.radius.Y <= 0 {
			continue
		}
		dx, dy := (px-corner.cx)/corner.radius.X, (py-corner.cy)/corner.radius.Y
		return dx*dx+dy*dy <= 1
	}
	return true
}

func expandRadii(radius layoutmodel.BorderRadii, amount float32) layoutmodel.BorderRadii {
	expand := func(value layoutmodel.CornerRadius) layoutmodel.CornerRadius {
		return layoutmodel.CornerRadius{X: max(value.X+amount, 0), Y: max(value.Y+amount, 0)}
	}
	return layoutmodel.BorderRadii{TopLeft: expand(radius.TopLeft), TopRight: expand(radius.TopRight), BottomRight: expand(radius.BottomRight), BottomLeft: expand(radius.BottomLeft)}
}

var rasterFonts struct {
	sync.Mutex
	once     sync.Once
	regular  *opentype.Font
	bold     *opentype.Font
	fallback *opentype.Font
	symbols  []*opentype.Font
	faces    map[string]font.Face
}

func drawRasterText(canvas draw.Image, text string, x, baseline, size float32, bold bool, color uint32, opacity float32) {
	dot := fixed.P(int(x), int(baseline))
	for text != "" {
		first, runeSize := utf8.DecodeRuneInString(text)
		if first == '\ufe0e' || first == '\ufe0f' {
			text = text[runeSize:]
			continue
		}
		parsed, family := rasterFontForRune(first, bold)
		end := runeSize
		for end < len(text) {
			character, characterSize := utf8.DecodeRuneInString(text[end:])
			_, nextFamily := rasterFontForRune(character, bold)
			if nextFamily != family {
				break
			}
			end += characterSize
		}
		face := rasterFontFace(parsed, family, size)
		drawer := font.Drawer{Dst: canvas, Src: image.NewUniform(rgba(color, opacity)), Face: face, Dot: dot}
		drawer.DrawString(text[:end])
		dot = drawer.Dot
		text = text[end:]
	}
}

func rasterFontForRune(character rune, bold bool) (*opentype.Font, string) {
	rasterFonts.Lock()
	defer rasterFonts.Unlock()
	rasterFonts.once.Do(func() {
		rasterFonts.regular, _ = opentype.Parse(goregular.TTF)
		rasterFonts.bold, _ = opentype.Parse(gobold.TTF)
		rasterFonts.faces = make(map[string]font.Face)
		for _, name := range []string{
			os.Getenv("GROWSE_VISUAL_CJK_FONT"),
			"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
			"/System/Library/Fonts/ヒラギノ角ゴシック W3.ttc",
			"C:/Windows/Fonts/msgothic.ttc",
		} {
			if name == "" {
				continue
			}
			data, err := os.ReadFile(name)
			if err != nil {
				continue
			}
			collection, err := opentype.ParseCollection(data)
			if err != nil || collection.NumFonts() == 0 {
				continue
			}
			rasterFonts.fallback, _ = collection.Font(0)
			if rasterFonts.fallback != nil {
				break
			}
		}
		for _, name := range []string{
			"/usr/share/fonts/truetype/noto/NotoSansSymbols2-Regular.ttf",
			"/usr/share/fonts/opentype/unifont/unifont_upper.otf",
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		} {
			data, err := os.ReadFile(name)
			if err != nil {
				continue
			}
			parsed, err := opentype.Parse(data)
			if err == nil {
				rasterFonts.symbols = append(rasterFonts.symbols, parsed)
			}
		}
	})
	primary, family := rasterFonts.regular, "regular"
	if bold {
		primary, family = rasterFonts.bold, "bold"
	}
	if primary != nil {
		if glyph, err := primary.GlyphIndex(nil, character); err == nil && glyph != 0 {
			return primary, family
		}
	}
	if rasterFonts.fallback != nil {
		if glyph, err := rasterFonts.fallback.GlyphIndex(nil, character); err == nil && glyph != 0 {
			return rasterFonts.fallback, "cjk-fallback"
		}
	}
	for index, candidate := range rasterFonts.symbols {
		if glyph, err := candidate.GlyphIndex(nil, character); err == nil && glyph != 0 {
			return candidate, fmt.Sprintf("symbol-fallback-%d", index)
		}
	}
	return primary, family
}

func rasterFontFace(parsed *opentype.Font, family string, size float32) font.Face {
	rasterFonts.Lock()
	defer rasterFonts.Unlock()
	key := fmt.Sprintf("%s/%.3f", family, size)
	if face := rasterFonts.faces[key]; face != nil {
		return face
	}
	if parsed == nil || size <= 0 {
		return basicfont.Face7x13
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return basicfont.Face7x13
	}
	rasterFonts.faces[key] = face
	return face
}

func TestVisualEvidenceRasterUsesDistinctCJKFallbackGlyphsWhenAvailable(t *testing.T) {
	firstFont, firstFamily := rasterFontForRune('日', false)
	secondFont, secondFamily := rasterFontForRune('語', false)
	if firstFamily != "cjk-fallback" || secondFamily != "cjk-fallback" {
		t.Skip("no CJK fallback font is installed on this platform")
	}
	first, firstErr := firstFont.GlyphIndex(nil, '日')
	second, secondErr := secondFont.GlyphIndex(nil, '語')
	if firstErr != nil || secondErr != nil || first == 0 || second == 0 || first == second {
		t.Fatalf("CJK evidence glyphs are not distinct: 日=%d/%v 語=%d/%v", first, firstErr, second, secondErr)
	}
}

func rgba(value uint32, opacity float32) color.NRGBA {
	return color.NRGBA{R: uint8(value >> 24), G: uint8(value >> 16), B: uint8(value >> 8), A: uint8(float32(uint8(value)) * opacity)}
}

func diffImages(left, right image.Image) (*image.RGBA, float64) {
	bounds := left.Bounds().Intersect(right.Bounds())
	difference := image.NewRGBA(left.Bounds())
	different := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			lr, lg, lb, _ := left.At(x, y).RGBA()
			rr, rg, rb, _ := right.At(x, y).RGBA()
			delta := abs16(lr, rr) + abs16(lg, rg) + abs16(lb, rb)
			if delta > 24*257*3 {
				difference.SetRGBA(x, y, color.RGBA{R: 220, G: 38, B: 38, A: 255})
				different++
			} else {
				difference.SetRGBA(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}
	if bounds.Empty() {
		return difference, 1
	}
	return difference, float64(different) / float64(bounds.Dx()*bounds.Dy())
}

func abs16(left, right uint32) uint32 {
	if left > right {
		return left - right
	}
	return right - left
}

func readPNG(t *testing.T, name string) image.Image {
	t.Helper()
	file, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoded, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func writeEvidenceArtifact(t *testing.T, name string, source image.Image) {
	t.Helper()
	directory := os.Getenv("GROWSE_REAL_SITE_ARTIFACT_DIR")
	if directory == "" {
		return
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(directory, name))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, source); err != nil {
		t.Fatal(err)
	}
}

func writeEvidenceJSON(t *testing.T, name string, value any) {
	t.Helper()
	directory := os.Getenv("GROWSE_REAL_SITE_ARTIFACT_DIR")
	if directory == "" {
		return
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(filepath.Join(directory, name), encoded, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiffMetricDetectsStructuralChange(t *testing.T) {
	left := image.NewRGBA(image.Rect(0, 0, 4, 4))
	right := image.NewRGBA(image.Rect(0, 0, 4, 4))
	draw.Draw(left, left.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(right, right.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	right.Set(1, 1, color.Black)
	encoded := new(bytes.Buffer)
	difference, ratio := diffImages(left, right)
	if ratio != 1.0/16.0 || difference.RGBAAt(1, 1).R == 0 || png.Encode(encoded, difference) != nil || encoded.Len() == 0 {
		t.Fatalf("diff evidence = ratio:%f pixel:%v bytes:%d", ratio, difference.RGBAAt(1, 1), encoded.Len())
	}
}

func TestEvidenceRasterPreservesRoundedBorderShadowAndOpacity(t *testing.T) {
	radius := layoutmodel.BorderRadii{
		TopLeft: layoutmodel.CornerRadius{X: 20, Y: 20}, TopRight: layoutmodel.CornerRadius{X: 20, Y: 20},
		BottomRight: layoutmodel.CornerRadius{X: 20, Y: 20}, BottomLeft: layoutmodel.CornerRadius{X: 20, Y: 20},
	}
	border := stylemodel.BorderSide{Width: 2, Style: stylemodel.BorderSolid, Color: 0x0000ffff}
	list := &paintmodel.DisplayList{Background: 0xffffffff, Commands: []paintmodel.Command{paintmodel.DrawBox{
		X: 10, Y: 10, Width: 40, Height: 40, Color: 0xff0000ff, Radius: radius,
		Border:     stylemodel.Borders{Top: border, Right: border, Bottom: border, Left: border},
		BoxShadows: []stylemodel.Shadow{{OffsetX: 2, OffsetY: 2, Spread: 2, Color: 0x00000080}}, Opacity: .75,
	}}}
	canvas := rasterDisplayList(list, 64, 64)
	if corner := canvas.RGBAAt(10, 10); corner.R > 245 && corner.G < 20 {
		t.Fatalf("rounded corner was painted as a square: %v", corner)
	}
	if center := canvas.RGBAAt(30, 30); center.R < 200 || center.G > 80 {
		t.Fatalf("opaque content disappeared from rounded box: %v", center)
	}
	if edge := canvas.RGBAAt(30, 10); edge.B < 100 {
		t.Fatalf("border was not present in evidence raster: %v", edge)
	}
}

func TestStructuralClassifierDetectsMissingEmptyClippedAndOverlappingContent(t *testing.T) {
	observations := map[string][]regionObservation{
		"empty": {{Metric: regionMetric{Name: "empty", Width: 100, Height: 40}}},
		"broken": {{
			Metric: regionMetric{Name: "broken", Width: 100, Height: 40, VisibleBoxes: 2, TextBytes: 8},
			Boxes: []layoutmodel.Box{
				{Text: "first", X: 0, Y: 0, Width: 80, Height: 24},
				{Text: "second", X: 20, Y: 10, Width: 100, Height: 24},
			},
		}},
	}
	issues := classifyStructuralIssues(nil, []string{"missing", "empty", "broken"}, observations)
	for _, want := range []struct{ severity, code string }{{"P0", "region-missing"}, {"P0", "region-empty"}, {"P1", "text-clipped"}, {"P1", "text-overlap"}} {
		found := false
		for _, issue := range issues {
			if issue.Severity == want.severity && issue.Code == want.code {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing %s/%s in %#v", want.severity, want.code, issues)
		}
	}
}

func TestEvidenceLimitsFailWithNamedFiniteErrors(t *testing.T) {
	manifest := readCorpusManifest(t)
	over := visualEvidence{Case: "fixture", Viewport: "desktop", Width: manifest.Limits.MaxPixels + 1, Height: 1}
	if err := validateEvidenceLimits(over, manifest); err == nil || !strings.Contains(err.Error(), "pixels") {
		t.Fatalf("pixel limit error = %v", err)
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "oversized.png"), make([]byte, 9), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateArtifactDirectory(directory, 8, 32); err == nil || !strings.Contains(err.Error(), "oversized.png") {
		t.Fatalf("artifact limit error = %v", err)
	}
	if err := validateArtifactDirectory(directory, 16, 8); err == nil || !strings.Contains(err.Error(), "corpus artifacts") {
		t.Fatalf("corpus limit error = %v", err)
	}
}

func Example_visualEvidence() {
	fmt.Println("growse, reference, diff, semantic region metrics")
	// Output: growse, reference, diff, semantic region metrics
}
