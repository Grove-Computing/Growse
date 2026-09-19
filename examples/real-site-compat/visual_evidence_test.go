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
	"testing"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/dom"
	layoutmodel "github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	paintmodel "github.com/Grove-Computing/Growse/internal/paint"
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
				page, err := engine.Navigate(context.Background(), server.URL+"/"+fixture.Fixture)
				if err != nil {
					t.Fatal(err)
				}
				engine.UpdateViewport(float32(viewport.Width), float32(viewport.Height))
				page = engine.Page()
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
	return metrics, classifyStructuralIssues(required, observations)
}

func classifyStructuralIssues(required []string, observations map[string][]regionObservation) []visualIssue {
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
					if strings.TrimSpace(next.Text) != "" && boxesStructurallyOverlap(box, next) {
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
			drawRect(canvas, command.X, command.Y, command.Width, command.Height, rgba(command.Color, command.Opacity))
		case paintmodel.DrawText:
			face := font.Face(basicfont.Face7x13)
			drawer := font.Drawer{Dst: canvas, Src: image.NewUniform(rgba(command.Color, command.Opacity)), Face: face, Dot: fixed.P(int(command.X), int(command.Y+command.Baseline))}
			drawer.DrawString(command.Text)
		}
	}
	return canvas
}

func drawRect(destination draw.Image, x, y, width, height float32, fill color.RGBA) {
	if width <= 0 || height <= 0 || fill.A == 0 {
		return
	}
	rectangle := image.Rect(int(x), int(y), int(x+width+.5), int(y+height+.5)).Intersect(destination.Bounds())
	if !rectangle.Empty() {
		draw.Draw(destination, rectangle, image.NewUniform(fill), image.Point{}, draw.Over)
	}
}

func rgba(value uint32, opacity float32) color.RGBA {
	return color.RGBA{R: uint8(value >> 24), G: uint8(value >> 16), B: uint8(value >> 8), A: uint8(float32(uint8(value)) * opacity)}
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
	issues := classifyStructuralIssues([]string{"missing", "empty", "broken"}, observations)
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
