package browser

import (
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"mime"
	"strconv"
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
)

func normalizeDecodedImage(source image.Image, encoded []byte, contentType string) *image.NRGBA {
	orientation := 1
	if mediaType, _, err := mime.ParseMediaType(contentType); err == nil && mediaType == "image/jpeg" {
		orientation = jpegEXIFOrientation(encoded)
	}
	return orientNRGBA(source, orientation)
}

func orientNRGBA(source image.Image, orientation int) *image.NRGBA {
	if source == nil {
		return nil
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	targetWidth, targetHeight := width, height
	if orientation >= 5 && orientation <= 8 {
		targetWidth, targetHeight = height, width
	}
	target := image.NewNRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	for y := range height {
		for x := range width {
			destinationX, destinationY := orientedCoordinate(x, y, width, height, orientation)
			target.SetNRGBA(destinationX, destinationY, color.NRGBAModel.Convert(source.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA))
		}
	}
	return target
}

func orientedCoordinate(x, y, width, height, orientation int) (int, int) {
	switch orientation {
	case 2:
		return width - 1 - x, y
	case 3:
		return width - 1 - x, height - 1 - y
	case 4:
		return x, height - 1 - y
	case 5:
		return y, x
	case 6:
		return height - 1 - y, x
	case 7:
		return height - 1 - y, width - 1 - x
	case 8:
		return y, width - 1 - x
	default:
		return x, y
	}
}

func jpegEXIFOrientation(source []byte) int {
	if len(source) < 4 || source[0] != 0xff || source[1] != 0xd8 {
		return 1
	}
	for offset := 2; offset+4 <= len(source); {
		if source[offset] != 0xff {
			return 1
		}
		marker := source[offset+1]
		offset += 2
		if marker == 0xd9 || marker == 0xda {
			break
		}
		length := int(binary.BigEndian.Uint16(source[offset : offset+2]))
		if length < 2 || offset > len(source)-length {
			return 1
		}
		payload := source[offset+2 : offset+length]
		if marker == 0xe1 && len(payload) >= 14 && string(payload[:6]) == "Exif\x00\x00" {
			return tiffOrientation(payload[6:])
		}
		offset += length
	}
	return 1
}

func tiffOrientation(source []byte) int {
	if len(source) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(source[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	if order.Uint16(source[2:4]) != 42 {
		return 1
	}
	offset := int(order.Uint32(source[4:8]))
	if offset < 0 || offset > len(source)-2 {
		return 1
	}
	entries := int(order.Uint16(source[offset : offset+2]))
	offset += 2
	if entries > 256 || offset > len(source)-entries*12 {
		return 1
	}
	for index := range entries {
		entry := source[offset+index*12 : offset+(index+1)*12]
		if order.Uint16(entry[:2]) != 0x0112 || order.Uint16(entry[2:4]) != 3 || order.Uint32(entry[4:8]) != 1 {
			continue
		}
		orientation := int(order.Uint16(entry[8:10]))
		if orientation >= 1 && orientation <= 8 {
			return orientation
		}
	}
	return 1
}

func resizeImageForNode(ctx context.Context, source image.Image, node *dom.Node, deviceScale float32, budget *imageDecodeBudget) (image.Image, error) {
	if source == nil {
		return nil, errors.New("image source is unavailable")
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if deviceScale <= 0 {
		deviceScale = 1
	}
	targetWidth, hasWidth := imageDimensionAttribute(node, "width")
	targetHeight, hasHeight := imageDimensionAttribute(node, "height")
	if !hasWidth && !hasHeight {
		return source, nil
	}
	if hasWidth {
		targetWidth = int(float32(targetWidth)*deviceScale + 0.5)
	} else {
		targetWidth = max(1, targetHeight*width/height)
	}
	if hasHeight {
		targetHeight = int(float32(targetHeight)*deviceScale + 0.5)
	} else {
		targetHeight = max(1, targetWidth*height/width)
	}
	if targetWidth == width && targetHeight == height {
		return source, nil
	}
	if targetWidth <= 0 || targetHeight <= 0 || targetWidth > maxImageDimension || targetHeight > maxImageDimension || targetWidth > maxImagePixels/targetHeight {
		return nil, errors.New("image resize dimensions are invalid")
	}
	if !budget.reserveSurface(targetWidth, targetHeight) {
		return nil, errors.New("page image resize surface limit exceeded")
	}
	target := image.NewNRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	for y := range targetHeight {
		if y&31 == 0 {
			select {
			case <-ctx.Done():
				budget.releaseSurface(targetWidth, targetHeight)
				return nil, ctx.Err()
			default:
			}
		}
		sourceY := bounds.Min.Y + min(height-1, y*height/targetHeight)
		for x := range targetWidth {
			sourceX := bounds.Min.X + min(width-1, x*width/targetWidth)
			target.SetNRGBA(x, y, color.NRGBAModel.Convert(source.At(sourceX, sourceY)).(color.NRGBA))
		}
	}
	return target, nil
}

func imageDimensionAttribute(node *dom.Node, name string) (int, bool) {
	if node == nil {
		return 0, false
	}
	value, exists := node.Attribute(name)
	if !exists {
		return 0, false
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	return parsed, err == nil && parsed > 0
}
