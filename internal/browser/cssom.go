package browser

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/Grove-Computing/Growse/internal/dom"
	layoutengine "github.com/Grove-Computing/Growse/internal/layout"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

func pageRenderSnapshot(ctx context.Context, page *Page, nodeID dom.NodeID) (runtimemodel.RenderSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return runtimemodel.RenderSnapshot{}, err
	}
	if page == nil || page.Document == nil {
		return runtimemodel.RenderSnapshot{}, fmt.Errorf("render snapshot is unavailable")
	}
	node, ok := page.Document.NodeByID(nodeID)
	computed, styled := page.ComputedStyles[nodeID]
	if !ok || !styled || !page.Document.IsConnected(node) {
		return runtimemodel.RenderSnapshot{}, fmt.Errorf("render target is disconnected")
	}
	revision := page.StyleRevision
	tree := layoutengine.BuildWithScrollAtRevision(page.Document, page.ComputedStyles, page.ViewportWidth, page.ViewportHeight, 0, 0, revision)
	rect, laidOut := tree.Bounds[nodeID]
	if !laidOut {
		rect = layoutengine.Rect{}
	}
	clientWidth := maxFloat32(0, rect.Width-computed.Border.Left.Width-computed.Border.Right.Width)
	clientHeight := maxFloat32(0, rect.Height-computed.Border.Top.Width-computed.Border.Bottom.Width)
	scrollWidth, scrollHeight := clientWidth, clientHeight
	if node == page.Document.Root || node.TagName == "html" || node.TagName == "body" {
		scrollWidth = maxFloat32(scrollWidth, tree.ScrollWidth)
		scrollHeight = maxFloat32(scrollHeight, tree.ScrollHeight)
	}
	return runtimemodel.RenderSnapshot{
		Revision: revision, Style: cssomProperties(computed, rect.Width, rect.Height),
		Rect:        runtimemodel.DOMRect{X: rect.X, Y: rect.Y, Width: rect.Width, Height: rect.Height},
		ClientWidth: clientWidth, ClientHeight: clientHeight, ScrollWidth: scrollWidth, ScrollHeight: scrollHeight,
	}, nil
}

func pageMediaEnvironment(page *Page) runtimemodel.MediaEnvironment {
	if page == nil {
		return runtimemodel.MediaEnvironment{}
	}
	return runtimemodel.MediaEnvironment{
		ViewportWidth: page.ViewportWidth, ViewportHeight: page.ViewportHeight,
		ColorScheme: "light", Hover: true, Pointer: "fine", ReducedMotion: page.ReducedMotion,
	}
}

func cssomProperties(computed stylemodel.ComputedStyle, width, height float32) map[string]string {
	computedWidth, computedHeight := width, height
	if computed.BoxSizing == stylemodel.BoxSizingContentBox {
		computedWidth = maxFloat32(0, width-computed.Padding.Left-computed.Padding.Right-computed.Border.Left.Width-computed.Border.Right.Width)
		computedHeight = maxFloat32(0, height-computed.Padding.Top-computed.Padding.Bottom-computed.Border.Top.Width-computed.Border.Bottom.Width)
	}
	result := map[string]string{
		"display": displayCSS(computed.Display), "position": positionCSS(computed.Position),
		"visibility": visibilityCSS(computed.Visibility), "box-sizing": boxSizingCSS(computed.BoxSizing),
		"font-size": px(computed.FontSize), "font-weight": strconv.Itoa(computed.FontWeight), "line-height": px(computed.LineHeight),
		"color": cssColor(computed.Color), "background-color": cssColor(computed.BackgroundColor),
		"opacity": numberCSS(computed.Opacity), "z-index": strconv.Itoa(computed.ZIndex),
		"width": px(computedWidth), "height": px(computedHeight), "overflow-x": overflowCSS(computed.OverflowX), "overflow-y": overflowCSS(computed.OverflowY),
		"margin-top": px(computed.Margin.Top), "margin-right": px(computed.Margin.Right), "margin-bottom": px(computed.Margin.Bottom), "margin-left": px(computed.Margin.Left),
		"padding-top": px(computed.Padding.Top), "padding-right": px(computed.Padding.Right), "padding-bottom": px(computed.Padding.Bottom), "padding-left": px(computed.Padding.Left),
		"border-top-width": px(computed.Border.Top.Width), "border-right-width": px(computed.Border.Right.Width),
		"border-bottom-width": px(computed.Border.Bottom.Width), "border-left-width": px(computed.Border.Left.Width),
		"flex-grow": numberCSS(computed.FlexGrow), "flex-shrink": numberCSS(computed.FlexShrink), "order": strconv.Itoa(computed.Order),
		"row-gap": lengthPercentageCSS(computed.RowGap), "column-gap": lengthPercentageCSS(computed.ColumnGap),
	}
	for name, value := range computed.CustomProperties {
		result[name] = value
	}
	return result
}

func px(value float32) string { return numberCSS(value) + "px" }

func numberCSS(value float32) string {
	if math.Abs(float64(value)) < 0.000001 {
		value = 0
	}
	return strconv.FormatFloat(float64(value), 'f', -1, 32)
}

func cssColor(value uint32) string {
	red, green := value>>24, (value>>16)&0xff
	blue, alpha := (value>>8)&0xff, value&0xff
	if alpha == 255 {
		return fmt.Sprintf("rgb(%d, %d, %d)", red, green, blue)
	}
	return fmt.Sprintf("rgba(%d, %d, %d, %s)", red, green, blue, numberCSS(float32(alpha)/255))
}

func displayCSS(value stylemodel.Display) string {
	values := [...]string{"inline", "block", "inline-block", "none", "flex", "inline-flex", "grid", "inline-grid"}
	if int(value) < len(values) {
		return values[value]
	}
	return ""
}

func positionCSS(value stylemodel.Position) string {
	values := [...]string{"static", "relative", "absolute", "fixed", "sticky"}
	if int(value) < len(values) {
		return values[value]
	}
	return ""
}

func visibilityCSS(value stylemodel.Visibility) string {
	if value == stylemodel.VisibilityHidden {
		return "hidden"
	}
	return "visible"
}

func boxSizingCSS(value stylemodel.BoxSizing) string {
	if value == stylemodel.BoxSizingBorderBox {
		return "border-box"
	}
	return "content-box"
}

func overflowCSS(value stylemodel.Overflow) string {
	values := [...]string{"visible", "hidden", "auto", "scroll"}
	if int(value) < len(values) {
		return values[value]
	}
	return ""
}

func lengthPercentageCSS(value stylemodel.LengthPercentage) string {
	if value.Percentage != 0 && value.Pixels != 0 {
		return fmt.Sprintf("calc(%s%% + %s)", numberCSS(value.Percentage*100), px(value.Pixels))
	}
	if value.Percentage != 0 {
		return numberCSS(value.Percentage*100) + "%"
	}
	return px(value.Pixels)
}

func maxFloat32(left, right float32) float32 {
	if left > right {
		return left
	}
	return right
}
