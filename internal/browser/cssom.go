package browser

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

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
		"font-size": px(computed.FontSize), "font-weight": strconv.Itoa(computed.FontWeight), "font-family": strings.Join(computed.FontFamilies, ", "),
		"font-style": computed.FontStyle, "font-stretch": computed.FontStretch, "line-height": px(computed.LineHeight),
		"text-align": textAlignCSS(computed.TextAlign), "text-transform": textTransformCSS(computed.TextTransform), "text-indent": lengthPercentageCSS(computed.TextIndent),
		"letter-spacing": spacingCSS(computed.LetterSpacing), "word-spacing": spacingCSS(computed.WordSpacing),
		"word-break": wordBreakCSS(computed.WordBreak), "overflow-wrap": overflowWrapCSS(computed.OverflowWrap),
		"vertical-align": verticalAlignCSS(computed.VerticalAlign), "text-overflow": textOverflowCSS(computed.TextOverflow),
		"object-fit": objectFitCSS(computed.ObjectFit), "object-position": lengthPercentageCSS(computed.ObjectPosition.X) + " " + lengthPercentageCSS(computed.ObjectPosition.Y),
		"list-style-type": listStyleTypeCSS(computed.ListStyleType), "list-style-position": listStylePositionCSS(computed.ListStylePosition), "list-style-image": listStyleImageCSS(computed.ListStyleImage),
		"appearance": appearanceCSS(computed.Appearance), "accent-color": accentColorCSS(computed), "cursor": cursorCSS(computed.Cursor),
		"filter": filterListCSS(computed.Filters), "backdrop-filter": filterListCSS(computed.BackdropFilters), "mix-blend-mode": blendModeCSS(computed.MixBlendMode),
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

func textAlignCSS(value stylemodel.TextAlign) string {
	values := [...]string{"start", "end", "left", "right", "center", "justify"}
	if int(value) < len(values) {
		return values[value]
	}
	return "start"
}

func textTransformCSS(value stylemodel.TextTransform) string {
	values := [...]string{"none", "uppercase", "lowercase", "capitalize"}
	if int(value) < len(values) {
		return values[value]
	}
	return "none"
}

func wordBreakCSS(value stylemodel.WordBreak) string {
	values := [...]string{"normal", "break-all", "keep-all"}
	if int(value) < len(values) {
		return values[value]
	}
	return "normal"
}

func overflowWrapCSS(value stylemodel.OverflowWrap) string {
	values := [...]string{"normal", "break-word", "anywhere"}
	if int(value) < len(values) {
		return values[value]
	}
	return "normal"
}

func spacingCSS(value float32) string {
	if value == 0 {
		return "normal"
	}
	return px(value)
}

func verticalAlignCSS(value stylemodel.VerticalAlign) string {
	values := [...]string{"baseline", "sub", "super", "middle", "text-top", "text-bottom", "top", "bottom", ""}
	if value.Kind == stylemodel.VerticalAlignLength {
		return px(value.Value)
	}
	if int(value.Kind) < len(values) {
		return values[value.Kind]
	}
	return "baseline"
}

func textOverflowCSS(value stylemodel.TextOverflow) string {
	if value == stylemodel.TextOverflowEllipsis {
		return "ellipsis"
	}
	return "clip"
}

func objectFitCSS(value stylemodel.ObjectFit) string {
	values := [...]string{"fill", "contain", "cover", "none", "scale-down"}
	if int(value) < len(values) {
		return values[value]
	}
	return "fill"
}

func listStyleTypeCSS(value stylemodel.ListStyleType) string {
	values := [...]string{"disc", "circle", "square", "decimal", "none"}
	if int(value) < len(values) {
		return values[value]
	}
	return "disc"
}

func listStylePositionCSS(value stylemodel.ListStylePosition) string {
	if value == stylemodel.ListStyleInside {
		return "inside"
	}
	return "outside"
}
func listStyleImageCSS(value string) string {
	if value == "" {
		return "none"
	}
	return `url("` + value + `")`
}
func appearanceCSS(value stylemodel.Appearance) string {
	if value == stylemodel.AppearanceNone {
		return "none"
	}
	return "auto"
}
func accentColorCSS(value stylemodel.ComputedStyle) string {
	if value.AccentColorAuto {
		return "auto"
	}
	return cssColor(value.AccentColor)
}

func cursorCSS(value stylemodel.Cursor) string {
	values := [...]string{"auto", "default", "pointer", "text", "crosshair", "move", "grab", "grabbing", "not-allowed", "wait", "progress", "col-resize", "row-resize"}
	if int(value) < len(values) {
		return values[value]
	}
	return "auto"
}

func blendModeCSS(value stylemodel.BlendMode) string {
	values := [...]string{"normal", "multiply", "screen", "overlay", "darken", "lighten"}
	if int(value) < len(values) {
		return values[value]
	}
	return "normal"
}

func filterListCSS(filters []stylemodel.Filter) string {
	if len(filters) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(filters))
	for _, filter := range filters {
		switch filter.Kind {
		case stylemodel.FilterBlur:
			parts = append(parts, "blur("+px(filter.Radius)+")")
		case stylemodel.FilterBrightness:
			parts = append(parts, "brightness("+numberCSS(filter.Amount)+")")
		case stylemodel.FilterContrast:
			parts = append(parts, "contrast("+numberCSS(filter.Amount)+")")
		case stylemodel.FilterGrayscale:
			parts = append(parts, "grayscale("+numberCSS(filter.Amount)+")")
		case stylemodel.FilterHueRotate:
			parts = append(parts, "hue-rotate("+numberCSS(filter.Angle)+"deg)")
		case stylemodel.FilterInvert:
			parts = append(parts, "invert("+numberCSS(filter.Amount)+")")
		case stylemodel.FilterOpacity:
			parts = append(parts, "opacity("+numberCSS(filter.Amount)+")")
		case stylemodel.FilterSaturate:
			parts = append(parts, "saturate("+numberCSS(filter.Amount)+")")
		case stylemodel.FilterSepia:
			parts = append(parts, "sepia("+numberCSS(filter.Amount)+")")
		case stylemodel.FilterDropShadow:
			parts = append(parts, fmt.Sprintf("drop-shadow(%s %s %s %s)", px(filter.Shadow.OffsetX), px(filter.Shadow.OffsetY), px(filter.Shadow.Blur), cssColor(filter.Shadow.Color)))
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " ")
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
		return fmt.Sprintf("calc(%s%% + %s)", numberCSS(value.Percentage), px(value.Pixels))
	}
	if value.Percentage != 0 {
		return numberCSS(value.Percentage) + "%"
	}
	return px(value.Pixels)
}

func maxFloat32(left, right float32) float32 {
	if left > right {
		return left
	}
	return right
}
