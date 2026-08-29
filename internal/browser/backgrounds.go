package browser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"mime"
	"net/url"
	"sort"
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	"github.com/Grove-Computing/Growse/internal/style"
	_ "golang.org/x/image/webp"
)

const (
	maxImageBytes            = 16 << 20
	maxImageDimension        = 16_384
	maxImagePixels           = 100_000_000
	maxPageImageSurfaceBytes = 256 << 20
	maxPageImageResources    = 512
)

type imageDecodeBudget struct {
	resources    map[string]struct{}
	surfaceBytes int64
}

func newImageDecodeBudget() *imageDecodeBudget {
	return &imageDecodeBudget{resources: make(map[string]struct{})}
}

func newImageDecodeBudgetWithImages(images map[string]image.Image) *imageDecodeBudget {
	budget := newImageDecodeBudget()
	for resource, decoded := range images {
		if decoded == nil || !budget.claim("background:"+resource) {
			continue
		}
		bounds := decoded.Bounds()
		budget.commitSurface(bounds.Dx(), bounds.Dy())
	}
	return budget
}

func (budget *imageDecodeBudget) claim(resource string) bool {
	if budget == nil {
		return true
	}
	if _, exists := budget.resources[resource]; exists {
		return true
	}
	if len(budget.resources) >= maxPageImageResources {
		return false
	}
	budget.resources[resource] = struct{}{}
	return true
}

func (budget *imageDecodeBudget) allowsSurface(width, height int) bool {
	if budget == nil {
		return true
	}
	bytes := int64(width) * int64(height) * 4
	return bytes > 0 && bytes <= maxPageImageSurfaceBytes-budget.surfaceBytes
}

func (budget *imageDecodeBudget) commitSurface(width, height int) {
	if budget != nil {
		budget.surfaceBytes += int64(width) * int64(height) * 4
	}
}

func loadBackgroundImages(ctx context.Context, client ResourceLoader, computed style.Map) (map[string]image.Image, []string) {
	return loadBackgroundImagesWithBudget(ctx, client, computed, nil)
}

func loadBackgroundImagesWithBudget(ctx context.Context, client ResourceLoader, computed style.Map, budget *imageDecodeBudget) (map[string]image.Image, []string) {
	return loadBackgroundImagesWithCache(ctx, client, computed, budget, newImageResourceCache())
}

func loadBackgroundImagesWithCache(ctx context.Context, client ResourceLoader, computed style.Map, budget *imageDecodeBudget, cache *imageResourceCache) (map[string]image.Image, []string) {
	images := make(map[string]image.Image)
	var errors []string
	if client == nil {
		return images, errors
	}
	seen := make(map[string]bool)
	for _, computedStyle := range computed {
		backgrounds := []style.BackgroundImage{computedStyle.BackgroundImage}
		for _, layer := range computedStyle.BackgroundLayers {
			backgrounds = append(backgrounds, layer.Image)
		}
		for _, background := range backgrounds {
			if background.Kind != style.BackgroundImageURL || seen[background.URL] {
				continue
			}
			seen[background.URL] = true
			resourceURL, err := url.Parse(background.URL)
			if err != nil || resourceURL.Scheme != "http" && resourceURL.Scheme != "https" {
				errors = append(errors, "background image URL is not a supported HTTP(S) URL")
				continue
			}
			resource := cache.load(ctx, client, resourceURL, budget)
			switch resource.failure {
			case imageLoadRequestFailure:
				errors = append(errors, "background image request failed: "+network.RedactedURL(resourceURL))
				continue
			case imageLoadResponseFailure:
				errors = append(errors, "background image response was rejected: "+network.RedactedURL(resourceURL))
				continue
			case imageLoadDecodeFailure:
				errors = append(errors, "background image decode failed: "+network.RedactedURL(resourceURL))
				continue
			case imageLoadResourceLimit:
				errors = append(errors, "background image resource limit exceeded")
				continue
			}
			images[background.URL] = resource.decoded
		}
	}
	return images, errors
}

func loadReplacedImages(ctx context.Context, client ResourceLoader, baseURL *url.URL, document *dom.Document, viewportWidth, deviceScale float32) (map[dom.NodeID]layout.ImageResource, map[string]image.Image, []string) {
	return loadReplacedImagesWithPolicy(ctx, client, baseURL, document, viewportWidth, deviceScale, nil)
}

func loadReplacedImagesWithPolicy(ctx context.Context, client ResourceLoader, baseURL *url.URL, document *dom.Document, viewportWidth, deviceScale float32, eligible map[dom.NodeID]bool) (map[dom.NodeID]layout.ImageResource, map[string]image.Image, []string) {
	return loadReplacedImagesWithPolicyAndBudget(ctx, client, baseURL, document, viewportWidth, deviceScale, eligible, nil)
}

func loadReplacedImagesWithPolicyAndBudget(ctx context.Context, client ResourceLoader, baseURL *url.URL, document *dom.Document, viewportWidth, deviceScale float32, eligible map[dom.NodeID]bool, budget *imageDecodeBudget) (map[dom.NodeID]layout.ImageResource, map[string]image.Image, []string) {
	return loadReplacedImagesWithCache(ctx, client, baseURL, document, viewportWidth, deviceScale, eligible, budget, newImageResourceCache())
}

func loadReplacedImagesWithCache(ctx context.Context, client ResourceLoader, baseURL *url.URL, document *dom.Document, viewportWidth, deviceScale float32, eligible map[dom.NodeID]bool, budget *imageDecodeBudget, cache *imageResourceCache) (map[dom.NodeID]layout.ImageResource, map[string]image.Image, []string) {
	resources := make(map[dom.NodeID]layout.ImageResource)
	images := make(map[string]image.Image)
	var errors []string
	if client == nil || baseURL == nil || document == nil {
		return resources, images, errors
	}
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Type == dom.NodeElement && node.TagName == "img" {
			load := eligible == nil || eligible[node.ID]
			resource, decoded, failure := loadReplacedImageNodeWithCache(ctx, client, baseURL, node, viewportWidth, deviceScale, load, budget, cache)
			resources[node.ID] = resource
			if decoded != nil {
				images[resource.URL] = decoded
			}
			if failure != "" {
				errors = append(errors, failure)
			}
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(document.Root)
	return resources, images, errors
}

func loadReplacedImageNodeWithCache(ctx context.Context, client ResourceLoader, baseURL *url.URL, node *dom.Node, viewportWidth, deviceScale float32, eligible bool, budget *imageDecodeBudget, cache *imageResourceCache) (layout.ImageResource, image.Image, string) {
	resource := layout.ImageResource{}
	if node == nil || node.Type != dom.NodeElement || node.TagName != "img" {
		resource.Error = "image element is invalid"
		return resource, nil, resource.Error
	}
	resource.Alt, _ = node.Attribute("alt")
	candidates := imageCandidates(node, baseURL, viewportWidth, deviceScale)
	if len(candidates) == 0 {
		resource.Error = "image source is missing or invalid"
		return resource, nil, ""
	}
	if loading, _ := node.Attribute("loading"); strings.EqualFold(strings.TrimSpace(loading), "lazy") && !eligible {
		resource.URL, resource.Deferred = candidates[0].String(), true
		return resource, nil, ""
	}
	var lastTarget *url.URL
	for _, target := range candidates {
		if ctx.Err() != nil {
			return resource, nil, ""
		}
		lastTarget = target
		resource.URL = target.String()
		if target.Scheme != "http" && target.Scheme != "https" {
			resource.Error = "image URL is not a supported HTTP(S) URL"
			continue
		}
		cached := cache.load(ctx, client, target, budget)
		switch cached.failure {
		case imageLoadRequestFailure:
			resource.Error = "image request failed"
			continue
		case imageLoadResponseFailure:
			resource.Error = "image response was rejected"
			continue
		case imageLoadDecodeFailure:
			resource.Error = "image dimensions were rejected"
			continue
		case imageLoadResourceLimit:
			resource.Error = "image resource limit exceeded"
			continue
		}
		resource.Loaded, resource.Error = true, ""
		resource.IntrinsicWidth, resource.IntrinsicHeight = float32(cached.width), float32(cached.height)
		return resource, cached.decoded, ""
	}
	if resource.Error != "" && lastTarget != nil {
		return resource, nil, resource.Error + ": " + network.RedactedURL(lastTarget)
	}
	return resource, nil, ""
}

func isImageContentType(contentType string) bool {
	if contentType == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	switch mediaType {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/svg+xml":
		return true
	default:
		return false
	}
}

func decodeImageResponse(body []byte, contentType string) (image.Image, int, int, error) {
	return decodeImageResponseWithBudget(body, contentType, nil)
}

func decodeImageResponseWithBudget(body []byte, contentType string, budget *imageDecodeBudget) (decoded image.Image, width, height int, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			decoded, width, height, err = nil, 0, 0, fmt.Errorf("image decoder panic: %v", recovered)
		}
	}()
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, 0, 0, err
	}
	if mediaType == "image/svg+xml" {
		return rasterizeSVGWithBudget(body, budget)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("image dimensions are invalid: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxImageDimension || config.Height > maxImageDimension || config.Width > maxImagePixels/config.Height {
		return nil, 0, 0, errors.New("image dimensions are invalid")
	}
	if !budget.allowsSurface(config.Width, config.Height) {
		return nil, 0, 0, errors.New("page image decode surface limit exceeded")
	}
	decoded, _, err = image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, 0, 0, err
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != config.Width || bounds.Dy() != config.Height {
		return nil, 0, 0, errors.New("decoded image dimensions changed")
	}
	budget.commitSurface(config.Width, config.Height)
	return decoded, config.Width, config.Height, nil
}

func imageViewportPolicy(document *dom.Document, computed style.Map, baseURL *url.URL, viewportWidth, viewportHeight float32) map[dom.NodeID]bool {
	eligible := make(map[dom.NodeID]bool)
	placeholders := make(map[dom.NodeID]layout.ImageResource)
	if document == nil {
		return eligible
	}
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Type == dom.NodeElement && node.TagName == "img" {
			alt, _ := node.Attribute("alt")
			placeholder := layout.ImageResource{Alt: alt, Deferred: true}
			if candidates := imageCandidates(node, baseURL, viewportWidth, 1); len(candidates) != 0 {
				placeholder.URL = candidates[0].String()
			}
			placeholders[node.ID] = placeholder
			loading, _ := node.Attribute("loading")
			eligible[node.ID] = !strings.EqualFold(strings.TrimSpace(loading), "lazy")
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(document.Root)
	tree := layout.BuildWithScrollAndImages(document, computed, placeholders, viewportWidth, viewportHeight, 0, 0)
	nearBottom := viewportHeight * 2
	for nodeID := range placeholders {
		if bounds, ok := tree.Bounds[nodeID]; ok && bounds.Y <= nearBottom && bounds.Y+bounds.Height >= -viewportHeight {
			eligible[nodeID] = true
		}
	}
	return eligible
}

func dispatchImageResourceEvents(browserState *Browser, page *Page) {
	if browserState == nil || page == nil {
		return
	}
	var pending []events.Event
	page.imageMu.Lock()
	if page.imageEvents == nil {
		page.imageEvents = make(map[dom.NodeID]string)
	}
	// Resource IDs preserve document order without walking the live DOM while
	// the JavaScript runtime may be replacing subtrees. Inline SVG resources do
	// not dispatch HTMLImageElement load/error events.
	nodeIDs := make([]dom.NodeID, 0, len(page.ImageResources))
	for nodeID, resource := range page.ImageResources {
		if !strings.HasPrefix(resource.URL, "growse:inline-svg/") {
			nodeIDs = append(nodeIDs, nodeID)
		}
	}
	sort.Slice(nodeIDs, func(left, right int) bool { return nodeIDs[left] < nodeIDs[right] })
	for _, nodeID := range nodeIDs {
		resource := page.ImageResources[nodeID]
		if resource.Deferred {
			continue
		}
		signature := resource.URL + "\x00" + resource.Error
		if resource.Loaded {
			signature += "\x00loaded"
		}
		if page.imageEvents[nodeID] == signature {
			continue
		}
		page.imageEvents[nodeID] = signature
		eventType := events.Error
		if resource.Loaded {
			eventType = events.Load
		}
		pending = append(pending, events.New(eventType, nodeID, false, false))
	}
	page.imageMu.Unlock()
	for _, event := range pending {
		browserState.dispatchPageEvent(page, event)
	}
}
