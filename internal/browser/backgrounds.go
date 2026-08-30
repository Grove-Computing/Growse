package browser

import (
	"bytes"
	"context"
	"encoding/base64"
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
	"sync"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	"github.com/Grove-Computing/Growse/internal/style"
	"github.com/gen2brain/avif"
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
	mu           sync.Mutex
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
	budget.mu.Lock()
	defer budget.mu.Unlock()
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
	budget.mu.Lock()
	defer budget.mu.Unlock()
	bytes := int64(width) * int64(height) * 4
	return bytes > 0 && bytes <= maxPageImageSurfaceBytes-budget.surfaceBytes
}

func (budget *imageDecodeBudget) commitSurface(width, height int) {
	if budget != nil {
		budget.mu.Lock()
		budget.surfaceBytes += int64(width) * int64(height) * 4
		budget.mu.Unlock()
	}
}

func (budget *imageDecodeBudget) reserveSurface(width, height int) bool {
	if budget == nil {
		return true
	}
	bytes := int64(width) * int64(height) * 4
	budget.mu.Lock()
	defer budget.mu.Unlock()
	if bytes <= 0 || bytes > maxPageImageSurfaceBytes-budget.surfaceBytes {
		return false
	}
	budget.surfaceBytes += bytes
	return true
}

func (budget *imageDecodeBudget) releaseSurface(width, height int) {
	if budget == nil {
		return
	}
	budget.mu.Lock()
	budget.surfaceBytes -= int64(width) * int64(height) * 4
	if budget.surfaceBytes < 0 {
		budget.surfaceBytes = 0
	}
	budget.mu.Unlock()
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
			if strings.HasPrefix(strings.ToLower(background.URL), "data:") {
				decoded, err := decodeDataBackground(background.URL, budget)
				if err != nil {
					errors = append(errors, "background data image decode failed")
					continue
				}
				images[background.URL] = decoded
				continue
			}
			resourceURL, err := url.Parse(background.URL)
			if err != nil || resourceURL.Scheme != "http" && resourceURL.Scheme != "https" {
				errors = append(errors, "background image URL is not a supported HTTP(S) URL")
				continue
			}
			if client == nil {
				errors = append(errors, "background image request failed: "+network.RedactedURL(resourceURL))
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
	return images, boundedImageDiagnostics(errors)
}

func decodeDataBackground(resource string, budget *imageDecodeBudget) (image.Image, error) {
	if len(resource) > maxImageBytes*2 || !budget.claim("background:"+resource) {
		return nil, errors.New("data image resource limit exceeded")
	}
	comma := strings.IndexByte(resource, ',')
	if comma < len("data:image/ ") || comma < 0 {
		return nil, errors.New("invalid data image")
	}
	metadata, payload := resource[len("data:"):comma], resource[comma+1:]
	parts := strings.Split(metadata, ";")
	mediaType := strings.ToLower(strings.TrimSpace(parts[0]))
	if !strings.HasPrefix(mediaType, "image/") {
		return nil, errors.New("data resource is not an image")
	}
	base64Encoded := false
	for _, parameter := range parts[1:] {
		if strings.EqualFold(strings.TrimSpace(parameter), "base64") {
			base64Encoded = true
		}
	}
	var body []byte
	var err error
	if base64Encoded {
		body, err = base64.StdEncoding.DecodeString(payload)
	} else {
		var decoded string
		decoded, err = url.PathUnescape(payload)
		body = []byte(decoded)
	}
	if err != nil || len(body) == 0 || len(body) > maxImageBytes {
		return nil, errors.New("invalid data image payload")
	}
	decoded, _, _, err := decodeImageResponseWithBudget(body, mediaType, budget)
	return decoded, err
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
	type imageLoadResult struct {
		nodeID   dom.NodeID
		resource layout.ImageResource
		decoded  image.Image
		failure  string
	}
	var nodes []*dom.Node
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Type == dom.NodeElement && node.TagName == "img" {
			nodes = append(nodes, node)
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(document.Root)
	preloads := imagePreloads(document, baseURL)
	results := make([]imageLoadResult, len(nodes))
	jobs := make([]resourceJob, len(nodes))
	for index, node := range nodes {
		index, node := index, node
		load := eligible == nil || eligible[node.ID]
		var target *url.URL
		if candidates := imageCandidates(node, baseURL, viewportWidth, deviceScale); len(candidates) != 0 {
			target = candidates[0]
		}
		jobs[index] = resourceJob{
			priority: imageResourcePriority(node, target, load, preloads), order: index,
			run: func(jobContext context.Context) {
				resource, decoded, failure := loadReplacedImageNodeWithCache(jobContext, client, baseURL, node, viewportWidth, deviceScale, load, budget, cache)
				results[index] = imageLoadResult{nodeID: node.ID, resource: resource, decoded: decoded, failure: failure}
			},
		}
	}
	rejected := runBoundedResourceJobs(ctx, jobs)
	for _, job := range jobs[len(jobs)-rejected:] {
		node := nodes[job.order]
		alt, _ := node.Attribute("alt")
		results[job.order] = imageLoadResult{
			nodeID:   node.ID,
			resource: layout.ImageResource{Alt: alt, Error: "image resource queue saturated"},
			failure:  "image resource queue saturated",
		}
	}
	for _, result := range results {
		if result.nodeID == 0 {
			continue
		}
		resources[result.nodeID] = result.resource
		if result.decoded != nil {
			images[result.resource.URL] = result.decoded
		}
		if result.failure != "" {
			errors = append(errors, result.failure)
		}
	}
	return resources, images, boundedImageDiagnostics(errors)
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
			if cached.animationErr != "" {
				resource.Error = cached.animationErr
			} else {
				resource.Error = "image dimensions were rejected"
			}
			continue
		case imageLoadResourceLimit:
			resource.Error = "image resource limit exceeded"
			continue
		}
		resource.Loaded, resource.Error = true, ""
		resource.IntrinsicWidth, resource.IntrinsicHeight = float32(cached.width), float32(cached.height)
		resized, err := cache.prepareSurface(ctx, cached, target, node, deviceScale, budget)
		if err != nil {
			resource.Loaded, resource.Error = false, "image resize failed"
			continue
		}
		return resource, resized, ""
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
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/avif", "image/svg+xml":
		return true
	default:
		return false
	}
}

func decodeImageResponse(body []byte, contentType string) (image.Image, int, int, error) {
	return decodeImageResponseWithBudget(body, contentType, nil)
}

func decodeImageResponseWithBudget(body []byte, contentType string, budget *imageDecodeBudget) (decoded image.Image, width, height int, err error) {
	reservedWidth, reservedHeight := 0, 0
	defer func() {
		if recovered := recover(); recovered != nil {
			decoded, width, height, err = nil, 0, 0, fmt.Errorf("image decoder panic: %v", recovered)
		}
		if err != nil && reservedWidth > 0 {
			budget.releaseSurface(reservedWidth, reservedHeight)
		}
	}()
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, 0, 0, err
	}
	if mediaType == "image/svg+xml" {
		return rasterizeSVGWithBudget(body, budget)
	}
	var config image.Config
	if mediaType == "image/avif" {
		config, err = avif.DecodeConfig(bytes.NewReader(body))
	} else {
		config, _, err = image.DecodeConfig(bytes.NewReader(body))
	}
	if err != nil {
		return nil, 0, 0, fmt.Errorf("image dimensions are invalid: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxImageDimension || config.Height > maxImageDimension || config.Width > maxImagePixels/config.Height {
		return nil, 0, 0, errors.New("image dimensions are invalid")
	}
	if !budget.reserveSurface(config.Width, config.Height) {
		return nil, 0, 0, errors.New("page image decode surface limit exceeded")
	}
	reservedWidth, reservedHeight = config.Width, config.Height
	if mediaType == "image/avif" {
		decoded, err = avif.Decode(bytes.NewReader(body))
	} else {
		decoded, _, err = image.Decode(bytes.NewReader(body))
	}
	if err != nil {
		return nil, 0, 0, err
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != config.Width || bounds.Dy() != config.Height {
		return nil, 0, 0, errors.New("decoded image dimensions changed")
	}
	decoded = normalizeDecodedImage(decoded, body, mediaType)
	bounds = decoded.Bounds()
	return decoded, bounds.Dx(), bounds.Dy(), nil
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
