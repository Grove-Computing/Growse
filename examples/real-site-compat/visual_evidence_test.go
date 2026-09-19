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
	DifferenceRatio float64        `json:"differenceRatio"`
	Regions         []regionMetric `json:"regions"`
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

func TestRealSiteCorpusProducesGrowseReferenceDiffAndRegionArtifacts(t *testing.T) {
	manifest := readCorpusManifest(t)
	server := httptest.NewServer(http.FileServer(http.Dir(".")))
	defer server.Close()
	engine := browser.New(network.NewClientWithLimits(server.Client(), 8<<20))
	defer engine.Close()

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
				metric := visualEvidence{
					Case: fixture.ID, Viewport: viewport.Name, Width: viewport.Width, Height: viewport.Height,
					DisplayedBoxes: len(tree.Boxes), PaintCommands: len(list.Commands), DifferenceRatio: ratio,
					Regions: collectRegionMetrics(page.Document, tree),
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
}

func collectRegionMetrics(document *dom.Document, tree *layoutmodel.Tree) []regionMetric {
	var metrics []regionMetric
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil {
			return
		}
		if name, ok := node.Attribute("data-region"); ok {
			bounds := tree.Bounds[node.ID]
			metric := regionMetric{Name: name, NodeID: uint64(node.ID), X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: bounds.Height}
			for _, box := range tree.Boxes {
				if nodeContains(document, node, box.NodeID) && !box.Hidden && box.Width > 0 && box.Height > 0 {
					metric.VisibleBoxes++
					metric.TextBytes += len(strings.TrimSpace(box.Text))
				}
			}
			metrics = append(metrics, metric)
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(document.Root)
	return metrics
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

func Example_visualEvidence() {
	fmt.Println("growse, reference, diff, semantic region metrics")
	// Output: growse, reference, diff, semantic region metrics
}
