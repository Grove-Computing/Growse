package browser

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"strings"
	"time"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/style"
	animatedwebp "github.com/gen2brain/webp"
)

const (
	maxAnimatedImageFrames = 120
	maxAnimatedImageBytes  = 64 << 20
)

type animatedImageData struct {
	frames    []image.Image
	delays    []time.Duration
	loopCount int
}

type animatedImagePlayer struct {
	data     *animatedImageData
	started  time.Time
	pausedAt time.Time
	current  int
	closed   bool
}

func decodeAnimatedImage(body []byte, contentType string, budget *imageDecodeBudget) (*animatedImageData, error) {
	switch contentType {
	case "image/gif":
		decoded, err := gif.DecodeAll(bytes.NewReader(body))
		if err != nil || len(decoded.Image) <= 1 {
			return nil, err
		}
		frames, err := composeGIFFrames(decoded, budget)
		if err != nil {
			return nil, err
		}
		delays := make([]time.Duration, len(frames))
		for index := range delays {
			delay := 10
			if index < len(decoded.Delay) && decoded.Delay[index] > 0 {
				delay = decoded.Delay[index]
			}
			delays[index] = time.Duration(delay) * 10 * time.Millisecond
		}
		return &animatedImageData{frames: frames, delays: delays, loopCount: decoded.LoopCount}, nil
	case "image/webp":
		decoded, err := animatedwebp.DecodeAll(bytes.NewReader(body))
		if err != nil || decoded == nil || len(decoded.Image) <= 1 {
			return nil, err
		}
		if len(decoded.Image) > maxAnimatedImageFrames {
			return nil, errors.New("animated WebP frame limit exceeded")
		}
		frames := make([]image.Image, len(decoded.Image))
		delays := make([]time.Duration, len(decoded.Image))
		for index, frame := range decoded.Image {
			normalized := orientNRGBA(frame, 1)
			bounds := normalized.Bounds()
			if !reserveAnimatedFrame(budget, bounds.Dx(), bounds.Dy(), len(frames)) {
				releaseAnimatedFrames(budget, frames[:index])
				return nil, errors.New("animated WebP surface limit exceeded")
			}
			frames[index] = normalized
			delay := 100
			if index < len(decoded.Delay) && decoded.Delay[index] > 0 {
				delay = decoded.Delay[index]
			}
			delays[index] = time.Duration(delay) * time.Millisecond
		}
		return &animatedImageData{frames: frames, delays: delays, loopCount: decoded.LoopCount}, nil
	default:
		return nil, nil
	}
}

func composeGIFFrames(source *gif.GIF, budget *imageDecodeBudget) ([]image.Image, error) {
	if source == nil || len(source.Image) <= 1 || len(source.Image) > maxAnimatedImageFrames || source.Config.Width <= 0 || source.Config.Height <= 0 {
		return nil, errors.New("animated GIF frame limit exceeded")
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, source.Config.Width, source.Config.Height))
	background := color.NRGBA{}
	if int(source.BackgroundIndex) < len(source.Image[0].Palette) {
		background = color.NRGBAModel.Convert(source.Image[0].Palette[source.BackgroundIndex]).(color.NRGBA)
	}
	frames := make([]image.Image, 0, len(source.Image))
	for index, frame := range source.Image {
		if !reserveAnimatedFrame(budget, source.Config.Width, source.Config.Height, len(source.Image)) {
			releaseAnimatedFrames(budget, frames)
			return nil, errors.New("animated GIF surface limit exceeded")
		}
		previous := cloneNRGBA(canvas)
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
		frames = append(frames, cloneNRGBA(canvas))
		disposal := byte(gif.DisposalNone)
		if index < len(source.Disposal) {
			disposal = source.Disposal[index]
		}
		switch disposal {
		case gif.DisposalBackground:
			draw.Draw(canvas, frame.Bounds(), &image.Uniform{C: background}, image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			canvas = previous
		}
	}
	return frames, nil
}

func reserveAnimatedFrame(budget *imageDecodeBudget, width, height, frameCount int) bool {
	bytes := int64(width) * int64(height) * 4
	return bytes > 0 && bytes <= maxAnimatedImageBytes && bytes*int64(frameCount) <= maxAnimatedImageBytes && budget.reserveSurface(width, height)
}

func releaseAnimatedFrames(budget *imageDecodeBudget, frames []image.Image) {
	for _, frame := range frames {
		if frame != nil {
			bounds := frame.Bounds()
			budget.releaseSurface(bounds.Dx(), bounds.Dy())
		}
	}
}

func cloneNRGBA(source *image.NRGBA) *image.NRGBA {
	target := image.NewNRGBA(source.Bounds())
	copy(target.Pix, source.Pix)
	return target
}

func animatedImagesForResources(resources map[dom.NodeID]layout.ImageResource, cache *imageResourceCache) map[dom.NodeID]*animatedImagePlayer {
	players := make(map[dom.NodeID]*animatedImagePlayer)
	if cache == nil {
		return players
	}
	for nodeID, resource := range resources {
		if data := cache.animation(resource.URL); data != nil {
			players[nodeID] = &animatedImagePlayer{data: data}
		}
	}
	return players
}

// AdvanceAnimatedImages advances only visible image candidates and returns the
// nodes requiring paint. Hidden/offscreen/reduced-motion players retain their
// current frame and resume from the same clock position.
func (p *Page) AdvanceAnimatedImages(current time.Time, tree *layout.Tree) []dom.NodeID {
	if p == nil {
		return nil
	}
	p.imageMu.Lock()
	defer p.imageMu.Unlock()
	var dirty []dom.NodeID
	for nodeID, player := range p.AnimatedImages {
		if player == nil || player.closed || player.data == nil {
			continue
		}
		eligible := !p.ReducedMotion && animatedImageVisible(p, tree, nodeID)
		if !eligible {
			if player.pausedAt.IsZero() {
				player.pausedAt = current
			}
			continue
		}
		if !player.pausedAt.IsZero() {
			if !player.started.IsZero() {
				player.started = player.started.Add(current.Sub(player.pausedAt))
			}
			player.pausedAt = time.Time{}
		}
		if player.started.IsZero() {
			player.started = current
		}
		index := player.frameIndex(current)
		if index == player.current {
			continue
		}
		player.current = index
		resource := p.ImageResources[nodeID]
		p.Images[resource.URL] = player.data.frames[index]
		dirty = append(dirty, nodeID)
	}
	return dirty
}

func (player *animatedImagePlayer) frameIndex(current time.Time) int {
	if player == nil || player.data == nil || len(player.data.frames) == 0 {
		return 0
	}
	total := time.Duration(0)
	for _, delay := range player.data.delays {
		total += delay
	}
	if total <= 0 {
		return 0
	}
	elapsed := current.Sub(player.started)
	if elapsed < 0 {
		elapsed = 0
	}
	if player.data.loopCount > 0 && elapsed >= total*time.Duration(player.data.loopCount) {
		return len(player.data.frames) - 1
	}
	elapsed %= total
	for index, delay := range player.data.delays {
		if elapsed < delay {
			return index
		}
		elapsed -= delay
	}
	return len(player.data.frames) - 1
}

func animatedImageVisible(page *Page, tree *layout.Tree, nodeID dom.NodeID) bool {
	if page.Document != nil {
		if node, exists := page.Document.NodeByID(nodeID); !exists {
			return false
		} else if _, hidden := node.Attribute("hidden"); hidden {
			return false
		} else if value, _ := node.Attribute("style"); strings.Contains(strings.ToLower(value), "display:none") {
			return false
		}
	}
	if computed, ok := page.ComputedStyles[nodeID]; ok && computed.Visibility == style.VisibilityHidden {
		return false
	}
	if tree == nil {
		return true
	}
	bounds, ok := tree.Bounds[nodeID]
	if !ok {
		return false
	}
	width, height := page.ViewportWidth, page.ViewportHeight
	if width <= 0 {
		width = tree.Width
	}
	if height <= 0 {
		height = tree.Height
	}
	return bounds.X < width && bounds.Y < height && bounds.X+bounds.Width > 0 && bounds.Y+bounds.Height > 0
}
