package browser

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

type svgText struct {
	x, y float64
	fill string
	text string
}

type svgClip struct {
	kind       string
	x, y, w, h float64
}

type svgMetadata struct {
	viewBox [4]float64
	width   float64
	height  float64
	texts   []svgText
	clips   map[string]svgClip
	clipID  string
}

func rasterizeSVG(source []byte) (image.Image, int, int, error) {
	sanitized, metadata, err := prepareSVG(source)
	if err != nil {
		return nil, 0, 0, err
	}
	icon, err := oksvg.ReadIconStream(bytes.NewReader(sanitized), oksvg.StrictErrorMode)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("parse SVG graphics: %w", err)
	}
	viewBox := metadata.viewBox
	if viewBox[2] <= 0 || viewBox[3] <= 0 {
		viewBox = [4]float64{icon.ViewBox.X, icon.ViewBox.Y, icon.ViewBox.W, icon.ViewBox.H}
	}
	width, height := metadata.width, metadata.height
	if width <= 0 {
		width = viewBox[2]
	}
	if height <= 0 {
		height = viewBox[3]
	}
	if width <= 0 || height <= 0 || width > 32768 || height > 32768 {
		return nil, 0, 0, errors.New("SVG has invalid dimensions")
	}
	pixelWidth, pixelHeight := int(math.Ceil(width)), int(math.Ceil(height))
	if pixelWidth <= 0 || pixelHeight <= 0 || pixelWidth > maxImagePixels/pixelHeight {
		return nil, 0, 0, errors.New("SVG raster surface is too large")
	}
	icon.ViewBox.X, icon.ViewBox.Y, icon.ViewBox.W, icon.ViewBox.H = viewBox[0], viewBox[1], viewBox[2], viewBox[3]
	result := image.NewRGBA(image.Rect(0, 0, pixelWidth, pixelHeight))
	icon.SetTarget(0, 0, float64(pixelWidth), float64(pixelHeight))
	scanner := rasterx.NewScannerGV(pixelWidth, pixelHeight, result, result.Bounds())
	icon.Draw(rasterx.NewDasher(pixelWidth, pixelHeight, scanner), 1)
	paintSVGText(result, metadata)
	applySVGClip(result, metadata)
	return result, pixelWidth, pixelHeight, nil
}

func prepareSVG(source []byte) ([]byte, svgMetadata, error) {
	decoder := xml.NewDecoder(bytes.NewReader(source))
	decoder.Strict = true
	var output bytes.Buffer
	encoder := xml.NewEncoder(&output)
	metadata := svgMetadata{clips: make(map[string]svgClip)}
	textDepth := 0
	var currentText svgText
	currentClip := ""
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, metadata, fmt.Errorf("parse SVG XML: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			if textDepth > 0 {
				textDepth++
				continue
			}
			if value.Name.Local == "text" {
				textDepth = 1
				currentText = svgText{fill: "black"}
				for _, attribute := range value.Attr {
					switch attribute.Name.Local {
					case "x":
						currentText.x, _ = parseSVGLength(attribute.Value)
					case "y":
						currentText.y, _ = parseSVGLength(attribute.Value)
					case "fill":
						currentText.fill = attribute.Value
					}
				}
				continue
			}
			if value.Name.Local == "svg" {
				readSVGRootMetadata(value.Attr, &metadata)
			}
			if value.Name.Local == "clipPath" {
				currentClip = svgAttribute(value.Attr, "id")
			}
			if currentClip != "" {
				if clip, ok := svgClipFromElement(value); ok {
					metadata.clips[currentClip] = clip
				}
			}
			filtered := value
			filtered.Attr = filtered.Attr[:0]
			for _, attribute := range value.Attr {
				if attribute.Name.Local == "clip-path" {
					if metadata.clipID == "" {
						metadata.clipID = svgURLFragment(attribute.Value)
					}
					continue
				}
				filtered.Attr = append(filtered.Attr, attribute)
			}
			if err := encoder.EncodeToken(filtered); err != nil {
				return nil, metadata, err
			}
		case xml.EndElement:
			if textDepth > 0 {
				textDepth--
				if textDepth == 0 {
					metadata.texts = append(metadata.texts, currentText)
				}
				continue
			}
			if value.Name.Local == "clipPath" {
				currentClip = ""
			}
			if err := encoder.EncodeToken(value); err != nil {
				return nil, metadata, err
			}
		case xml.CharData:
			if textDepth > 0 {
				currentText.text += string(value)
				continue
			}
			if err := encoder.EncodeToken(value); err != nil {
				return nil, metadata, err
			}
		default:
			if err := encoder.EncodeToken(token); err != nil {
				return nil, metadata, err
			}
		}
	}
	if err := encoder.Flush(); err != nil {
		return nil, metadata, err
	}
	if metadata.viewBox[2] <= 0 || metadata.viewBox[3] <= 0 {
		metadata.viewBox = [4]float64{0, 0, metadata.width, metadata.height}
	}
	return output.Bytes(), metadata, nil
}

func readSVGRootMetadata(attributes []xml.Attr, metadata *svgMetadata) {
	for _, attribute := range attributes {
		switch attribute.Name.Local {
		case "width":
			metadata.width, _ = parseSVGLength(attribute.Value)
		case "height":
			metadata.height, _ = parseSVGLength(attribute.Value)
		case "viewBox":
			fields := strings.Fields(strings.ReplaceAll(attribute.Value, ",", " "))
			if len(fields) == 4 {
				for index := range fields {
					metadata.viewBox[index], _ = strconv.ParseFloat(fields[index], 64)
				}
			}
		}
	}
}

func svgClipFromElement(element xml.StartElement) (svgClip, bool) {
	clip := svgClip{kind: element.Name.Local}
	switch element.Name.Local {
	case "rect":
		clip.x, _ = parseSVGLength(svgAttribute(element.Attr, "x"))
		clip.y, _ = parseSVGLength(svgAttribute(element.Attr, "y"))
		clip.w, _ = parseSVGLength(svgAttribute(element.Attr, "width"))
		clip.h, _ = parseSVGLength(svgAttribute(element.Attr, "height"))
	case "circle":
		cx, _ := parseSVGLength(svgAttribute(element.Attr, "cx"))
		cy, _ := parseSVGLength(svgAttribute(element.Attr, "cy"))
		radius, _ := parseSVGLength(svgAttribute(element.Attr, "r"))
		clip.x, clip.y, clip.w, clip.h = cx-radius, cy-radius, radius*2, radius*2
	case "ellipse":
		cx, _ := parseSVGLength(svgAttribute(element.Attr, "cx"))
		cy, _ := parseSVGLength(svgAttribute(element.Attr, "cy"))
		rx, _ := parseSVGLength(svgAttribute(element.Attr, "rx"))
		ry, _ := parseSVGLength(svgAttribute(element.Attr, "ry"))
		clip.x, clip.y, clip.w, clip.h = cx-rx, cy-ry, rx*2, ry*2
	default:
		return svgClip{}, false
	}
	return clip, clip.w > 0 && clip.h > 0
}

func paintSVGText(target *image.RGBA, metadata svgMetadata) {
	if target == nil || metadata.viewBox[2] <= 0 || metadata.viewBox[3] <= 0 {
		return
	}
	scaleX := float64(target.Bounds().Dx()) / metadata.viewBox[2]
	scaleY := float64(target.Bounds().Dy()) / metadata.viewBox[3]
	for _, item := range metadata.texts {
		textColor := color.Color(color.Black)
		if parsed, err := oksvg.ParseSVGColor(item.fill); err == nil {
			textColor = parsed
		}
		drawer := font.Drawer{Dst: target, Src: image.NewUniform(textColor), Face: basicfont.Face7x13}
		drawer.Dot = fixed.P(int(math.Round((item.x-metadata.viewBox[0])*scaleX)), int(math.Round((item.y-metadata.viewBox[1])*scaleY)))
		drawer.DrawString(strings.TrimSpace(item.text))
	}
}

func applySVGClip(target *image.RGBA, metadata svgMetadata) {
	clip, ok := metadata.clips[metadata.clipID]
	if !ok || target == nil || metadata.viewBox[2] <= 0 || metadata.viewBox[3] <= 0 {
		return
	}
	scaleX := float64(target.Bounds().Dx()) / metadata.viewBox[2]
	scaleY := float64(target.Bounds().Dy()) / metadata.viewBox[3]
	x := (clip.x - metadata.viewBox[0]) * scaleX
	y := (clip.y - metadata.viewBox[1]) * scaleY
	w, h := clip.w*scaleX, clip.h*scaleY
	for py := 0; py < target.Bounds().Dy(); py++ {
		for px := 0; px < target.Bounds().Dx(); px++ {
			inside := float64(px) >= x && float64(px) < x+w && float64(py) >= y && float64(py) < y+h
			if clip.kind == "circle" || clip.kind == "ellipse" {
				dx, dy := (float64(px)-(x+w/2))/(w/2), (float64(py)-(y+h/2))/(h/2)
				inside = dx*dx+dy*dy <= 1
			}
			if !inside {
				offset := target.PixOffset(px, py)
				target.Pix[offset+3] = 0
			}
		}
	}
}

func loadInlineSVGImages(document *dom.Document) (map[dom.NodeID]layout.ImageResource, map[string]image.Image, []string) {
	resources := make(map[dom.NodeID]layout.ImageResource)
	images := make(map[string]image.Image)
	var failures []string
	if document == nil {
		return resources, images, failures
	}
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Type == dom.NodeElement && node.TagName == "svg" {
			source := serializeSVGNode(node)
			decoded, width, height, err := rasterizeSVG(source)
			resourceURL := fmt.Sprintf("growse:inline-svg/%d", node.ID)
			resource := layout.ImageResource{URL: resourceURL, IntrinsicWidth: float32(width), IntrinsicHeight: float32(height), Loaded: err == nil}
			if err != nil {
				resource.Error = "inline SVG decode failed"
				failures = append(failures, resource.Error)
			} else {
				images[resourceURL] = decoded
			}
			resources[node.ID] = resource
			return
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(document.Root)
	return resources, images, failures
}

func mergeImageResources(resources map[dom.NodeID]layout.ImageResource, images map[string]image.Image, incomingResources map[dom.NodeID]layout.ImageResource, incomingImages map[string]image.Image) {
	for nodeID, resource := range incomingResources {
		resources[nodeID] = resource
	}
	for resourceURL, decoded := range incomingImages {
		images[resourceURL] = decoded
	}
}

func serializeSVGNode(node *dom.Node) []byte {
	var output strings.Builder
	var write func(*dom.Node)
	write = func(current *dom.Node) {
		if current == nil {
			return
		}
		if current.Type == dom.NodeText {
			var escaped bytes.Buffer
			_ = xml.EscapeText(&escaped, []byte(current.Text))
			output.Write(escaped.Bytes())
			return
		}
		if current.Type != dom.NodeElement {
			return
		}
		output.WriteByte('<')
		output.WriteString(canonicalSVGName(current.TagName))
		names := make([]string, 0, len(current.Attributes))
		for name := range current.Attributes {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			output.WriteByte(' ')
			output.WriteString(canonicalSVGAttribute(name))
			output.WriteString(`="`)
			var escaped bytes.Buffer
			_ = xml.EscapeText(&escaped, []byte(current.Attributes[name]))
			output.Write(escaped.Bytes())
			output.WriteByte('"')
		}
		output.WriteByte('>')
		for _, child := range current.Children {
			write(child)
		}
		output.WriteString("</")
		output.WriteString(canonicalSVGName(current.TagName))
		output.WriteByte('>')
	}
	write(node)
	return []byte(output.String())
}

func canonicalSVGName(name string) string {
	switch strings.ToLower(name) {
	case "clippath":
		return "clipPath"
	case "lineargradient":
		return "linearGradient"
	case "radialgradient":
		return "radialGradient"
	default:
		return name
	}
}

func canonicalSVGAttribute(name string) string {
	switch strings.ToLower(name) {
	case "viewbox":
		return "viewBox"
	case "gradienttransform":
		return "gradientTransform"
	case "gradientunits":
		return "gradientUnits"
	case "preserveaspectratio":
		return "preserveAspectRatio"
	default:
		return name
	}
}

func parseSVGLength(raw string) (float64, bool) {
	raw = strings.TrimSpace(strings.TrimSuffix(raw, "px"))
	value, err := strconv.ParseFloat(raw, 64)
	return value, err == nil && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func svgAttribute(attributes []xml.Attr, name string) string {
	for _, attribute := range attributes {
		if attribute.Name.Local == name {
			return attribute.Value
		}
	}
	return ""
}

func svgURLFragment(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "url(#") && strings.HasSuffix(raw, ")") {
		return strings.TrimSuffix(strings.TrimPrefix(raw, "url(#"), ")")
	}
	return ""
}
