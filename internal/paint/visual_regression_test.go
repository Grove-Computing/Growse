package paint

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"strings"
	"testing"

	"github.com/saku0512/growse/internal/css"
	"github.com/saku0512/growse/internal/dom"
	"github.com/saku0512/growse/internal/layout"
	"github.com/saku0512/growse/internal/style"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	visualViewportWidth  = 320
	visualViewportHeight = 240
	visualScale          = 1
)

type visualSnapshot struct {
	Viewport   string   `json:"viewport"`
	Scale      int      `json:"scale"`
	Font       string   `json:"font"`
	PixelSHA   string   `json:"pixel_sha256"`
	Geometry   []string `json:"geometry"`
	Display    []string `json:"display_list"`
	HitTesting []string `json:"hit_testing"`
}

// TestDashboardVisualRegression protects pixels, layout geometry, display-list
// ordering, and hit-testing with one deterministic fixture. The raster uses the
// embedded Go Regular font with hinting disabled, a fixed viewport, and scale 1.
func TestDashboardVisualRegression(t *testing.T) {
	document, nodes := visualFixtureDocument(t)
	stylesheet, err := css.Parse(strings.NewReader(`
.dashboard { position:relative; display:grid; width:240px; height:160px; grid-template-columns:80px 1fr; grid-template-rows:48px 1fr; gap:8px; background-color:#20283a }
.header { grid-column:1 / 3; background-color:#4263eb }
.side { background-color:#334155; border-radius:8px }
.card { position:relative; background-color:#f8fafc; border-radius:10px; overflow:hidden }
.badge { position:absolute; right:8px; top:8px; width:36px; height:20px; background-color:#f59f00; transform:rotate(8deg); transform-origin:center; z-index:2 }
.label { color:#ffffff; font-size:14px }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := layout.BuildWithViewport(document, style.Compute(document, stylesheet), visualViewportWidth, visualViewportHeight)
	list := Build(tree)
	imageValue := rasterVisualFixture(t, list, visualViewportWidth, visualViewportHeight, visualScale)
	hash := sha256.Sum256(imageValue.Pix)
	snapshot := visualSnapshot{
		Viewport: fmt.Sprintf("%dx%d", visualViewportWidth, visualViewportHeight), Scale: visualScale,
		Font: "gofont/goregular@72dpi-hinting-none", PixelSHA: hex.EncodeToString(hash[:]),
		Geometry: geometrySnapshot(tree), Display: displaySnapshot(list),
		HitTesting: hitSnapshot(tree, nodes),
	}
	actual, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	actual = append(actual, '\n')
	want, err := os.ReadFile("testdata/dashboard.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != string(want) {
		t.Fatalf("visual snapshot changed; inspect the rendering difference before updating testdata/dashboard.golden.json\n--- actual ---\n%s", actual)
	}
}

func visualFixtureDocument(t *testing.T) (*dom.Document, map[string]dom.NodeID) {
	t.Helper()
	document := dom.NewDocument()
	nodes := make(map[string]dom.NodeID)
	appendElement := func(parent *dom.Node, class string) *dom.Node {
		node := document.CreateElement("div", map[string]string{"class": class})
		if err := document.AppendChild(parent, node); err != nil {
			t.Fatal(err)
		}
		nodes[class] = node.ID
		return node
	}
	dashboard := appendElement(document.Root, "dashboard")
	header := appendElement(dashboard, "header")
	label := appendElement(header, "label")
	if err := document.AppendChild(label, document.CreateText("Growse 0.6")); err != nil {
		t.Fatal(err)
	}
	appendElement(dashboard, "side")
	card := appendElement(dashboard, "card")
	appendElement(card, "badge")
	return document, nodes
}

func geometrySnapshot(tree *layout.Tree) []string {
	result := make([]string, 0, len(tree.Decorations)+len(tree.Boxes))
	for _, item := range tree.Decorations {
		result = append(result, fmt.Sprintf("box node=%d rect=%.1f,%.1f,%.1f,%.1f stack=%d", item.NodeID, item.X, item.Y, item.Width, item.Height, item.StackingID))
	}
	for _, item := range tree.Boxes {
		result = append(result, fmt.Sprintf("text node=%d rect=%.1f,%.1f,%.1f,%.1f text=%q", item.NodeID, item.X, item.Y, item.Width, item.Height, item.Text))
	}
	return result
}

func displaySnapshot(list *DisplayList) []string {
	result := make([]string, 0, len(list.Commands))
	for index, candidate := range list.Commands {
		switch command := candidate.(type) {
		case DrawBox:
			result = append(result, fmt.Sprintf("%d box node=%d rect=%.1f,%.1f,%.1f,%.1f color=%08x opacity=%.2f transform=%.3f,%.3f,%.3f,%.3f,%.3f,%.3f", index, command.NodeID, command.X, command.Y, command.Width, command.Height, command.Color, command.Opacity, command.Transform.A, command.Transform.B, command.Transform.C, command.Transform.D, command.Transform.E, command.Transform.F))
		case DrawText:
			result = append(result, fmt.Sprintf("%d text rect=%.1f,%.1f,%.1f,%.1f color=%08x text=%q", index, command.X, command.Y, command.Width, command.Height, command.Color, command.Text))
		}
	}
	return result
}

func hitSnapshot(tree *layout.Tree, nodes map[string]dom.NodeID) []string {
	result := make([]string, 0, 3)
	for _, point := range [][2]float32{{40, 40}, {130, 105}, {230, 137}} {
		node, ok := layout.HitTest(tree, point[0], point[1])
		name := "none"
		for candidate, id := range nodes {
			if ok && node == id {
				name = candidate
				break
			}
		}
		result = append(result, fmt.Sprintf("%.0f,%.0f=%s(%d)", point[0], point[1], name, node))
	}
	return result
}

func rasterVisualFixture(t *testing.T, list *DisplayList, width, height, scale int) *image.RGBA {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, width*scale, height*scale))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.RGBA{R: 255, G: 255, B: 255, A: 255}), image.Point{}, draw.Src)
	parsedFont, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range list.Commands {
		switch command := candidate.(type) {
		case DrawBox:
			rasterBox(canvas, command, scale)
		case DrawText:
			face, err := opentype.NewFace(parsedFont, &opentype.FaceOptions{Size: float64(command.FontSize * float32(scale)), DPI: 72, Hinting: font.HintingNone})
			if err != nil {
				t.Fatal(err)
			}
			drawer := font.Drawer{Dst: canvas, Src: image.NewUniform(colorFromRGBA(command.Color, command.Opacity)), Face: face, Dot: fixed.P(int(command.X*float32(scale)), int((command.Y+command.Baseline)*float32(scale)))}
			drawer.DrawString(command.Text)
			_ = face.Close()
		}
	}
	return canvas
}

func rasterBox(canvas *image.RGBA, command DrawBox, scale int) {
	if command.Color == 0 || command.Width <= 0 || command.Height <= 0 {
		return
	}
	matrix := command.Transform
	if matrix == (style.Matrix{}) {
		matrix = style.IdentityMatrix()
	}
	inverse, ok := matrix.Inverse()
	if !ok {
		return
	}
	fill := colorFromRGBA(command.Color, command.Opacity)
	for y := 0; y < canvas.Bounds().Dy(); y++ {
		for x := 0; x < canvas.Bounds().Dx(); x++ {
			localX, localY := inverse.TransformPoint((float32(x)+.5)/float32(scale), (float32(y)+.5)/float32(scale))
			if localX < command.X || localX >= command.X+command.Width || localY < command.Y || localY >= command.Y+command.Height {
				continue
			}
			draw.Draw(canvas, image.Rect(x, y, x+1, y+1), image.NewUniform(fill), image.Point{}, draw.Over)
		}
	}
}

func colorFromRGBA(value uint32, opacity float32) color.RGBA {
	return color.RGBA{R: uint8(value >> 24), G: uint8(value >> 16), B: uint8(value >> 8), A: uint8(float32(uint8(value)) * opacity)}
}
