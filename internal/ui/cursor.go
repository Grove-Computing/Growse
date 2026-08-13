package ui

import (
	"bytes"
	_ "embed"
	"encoding/xml"
	"errors"
	"image"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

//go:embed assets/blue.svg
var gopherCursorSVG []byte

var (
	gopherCursorOnce  sync.Once
	gopherCursorImage image.Image
	gopherCursorError error
)

const (
	gopherCursorHeight   = unit.Dp(32)
	gopherCursorHotspotX = unit.Dp(2)
	gopherCursorHotspotY = unit.Dp(2)
)

type pointerState struct {
	position f32.Point
	inside   bool
}

type pointerTag struct{}

func loadGopherCursor() (image.Image, error) {
	gopherCursorOnce.Do(func() {
		gopherCursorImage, gopherCursorError = rasterizeGopherCursor(gopherCursorSVG)
	})
	return gopherCursorImage, gopherCursorError
}

func rasterizeGopherCursor(source []byte) (image.Image, error) {
	if len(source) == 0 {
		return nil, errors.New("gopher cursor SVG is empty")
	}
	icon, err := oksvg.ReadIconStream(bytes.NewReader(source), oksvg.StrictErrorMode)
	if err != nil {
		return nil, err
	}
	viewBox, err := svgViewBox(source)
	if err != nil {
		return nil, err
	}
	icon.ViewBox.X = viewBox[0]
	icon.ViewBox.Y = viewBox[1]
	icon.ViewBox.W = viewBox[2]
	icon.ViewBox.H = viewBox[3]
	width := int(math.Ceil(icon.ViewBox.W))
	height := int(math.Ceil(icon.ViewBox.H))
	if width <= 0 || height <= 0 {
		return nil, errors.New("gopher cursor SVG has an invalid viewBox")
	}

	result := image.NewRGBA(image.Rect(0, 0, width, height))
	icon.SetTarget(0, 0, float64(width), float64(height))
	scanner := rasterx.NewScannerGV(width, height, result, result.Bounds())
	icon.Draw(rasterx.NewDasher(width, height, scanner), 1)
	return result, nil
}

func svgViewBox(source []byte) ([4]float64, error) {
	decoder := xml.NewDecoder(bytes.NewReader(source))
	for {
		token, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return [4]float64{}, errors.New("gopher cursor SVG has no viewBox")
			}
			return [4]float64{}, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "svg" {
			continue
		}
		for _, attribute := range start.Attr {
			if attribute.Name.Local != "viewBox" {
				continue
			}
			fields := strings.Fields(strings.ReplaceAll(attribute.Value, ",", " "))
			if len(fields) != 4 {
				return [4]float64{}, errors.New("gopher cursor SVG has an invalid viewBox")
			}
			var result [4]float64
			for index, field := range fields {
				result[index], err = strconv.ParseFloat(field, 64)
				if err != nil {
					return [4]float64{}, errors.New("gopher cursor SVG has an invalid viewBox")
				}
			}
			if result[2] <= 0 || result[3] <= 0 {
				return [4]float64{}, errors.New("gopher cursor SVG has an invalid viewBox")
			}
			return result, nil
		}
		return [4]float64{}, errors.New("gopher cursor SVG has no viewBox")
	}
}

func cursorGeometry(sourceSize image.Point, metric unit.Metric, position f32.Point) (image.Point, image.Point) {
	height := metric.Dp(gopherCursorHeight)
	if height < 1 || sourceSize.X <= 0 || sourceSize.Y <= 0 {
		return image.Point{}, image.Point{}
	}
	width := int(math.Round(float64(height) * float64(sourceSize.X) / float64(sourceSize.Y)))
	if width < 1 {
		width = 1
	}
	hotspot := image.Pt(metric.Dp(gopherCursorHotspotX), metric.Dp(gopherCursorHotspotY))
	origin := image.Pt(
		int(math.Round(float64(position.X)))-hotspot.X,
		int(math.Round(float64(position.Y)))-hotspot.Y,
	)
	return image.Pt(width, height), origin
}

func (ui *BrowserUI) handlePointerEvents(gtx layout.Context) {
	if ui == nil {
		return
	}
	for {
		raw, ok := gtx.Event(pointer.Filter{
			Target: &ui.pointerTag,
			Kinds:  pointer.Enter | pointer.Move | pointer.Drag | pointer.Press | pointer.Release | pointer.Leave | pointer.Cancel,
		})
		if !ok {
			return
		}
		event, ok := raw.(pointer.Event)
		if !ok || event.Source != pointer.Mouse {
			continue
		}
		switch event.Kind {
		case pointer.Leave, pointer.Cancel:
			ui.pointer.inside = false
			if ui.navigator != nil {
				ui.navigator.ClearHover()
			}
		default:
			ui.pointer.position = event.Position
			ui.pointer.inside = true
		}
		ui.invalidate()
	}
}

func (ui *BrowserUI) registerPointerTracker(gtx layout.Context) {
	if ui == nil {
		return
	}
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	event.Op(gtx.Ops, &ui.pointerTag)
	if ui.gopherCursorReady {
		pointer.CursorNone.Add(gtx.Ops)
	}
	pass.Pop()
	area.Pop()
}

func (ui *BrowserUI) layoutGopherCursor(gtx layout.Context) {
	if ui == nil || !ui.gopherCursorReady || !ui.pointer.inside {
		return
	}
	size, origin := cursorGeometry(ui.gopherCursor.Size(), gtx.Metric, ui.pointer.position)
	if size.X == 0 || size.Y == 0 {
		return
	}

	offset := op.Offset(origin).Push(gtx.Ops)
	defer offset.Pop()
	cursorContext := gtx
	cursorContext.Constraints = layout.Exact(size)
	widget.Image{
		Src:      ui.gopherCursor,
		Fit:      widget.Contain,
		Position: layout.NW,
		Scale:    float32(gopherCursorHeight) / float32(ui.gopherCursor.Size().Y),
	}.Layout(cursorContext)
}
