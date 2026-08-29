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
			if !budget.claim("background:" + background.URL) {
				errors = append(errors, "background image resource limit exceeded")
				continue
			}
			resourceURL, err := url.Parse(background.URL)
			if err != nil || resourceURL.Scheme != "http" && resourceURL.Scheme != "https" {
				errors = append(errors, "background image URL is not a supported HTTP(S) URL")
				continue
			}
			response, err := client.Get(ctx, resourceURL)
			if err != nil || response == nil {
				errors = append(errors, "background image request failed: "+network.RedactedURL(resourceURL))
				continue
			}
			if len(response.Body) > maxImageBytes || !isImageContentType(response.ContentType) {
				errors = append(errors, "background image response was rejected: "+network.RedactedURL(resourceURL))
				continue
			}
			decoded, _, _, err := decodeImageResponseWithBudget(response.Body, response.ContentType, budget)
			if err != nil {
				errors = append(errors, "background image decode failed: "+network.RedactedURL(resourceURL))
				continue
			}
			images[background.URL] = decoded
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
			alt, _ := node.Attribute("alt")
			resource := layout.ImageResource{Alt: alt}
			candidates := imageCandidates(node, baseURL, viewportWidth, deviceScale)
			if len(candidates) == 0 {
				resource.Error = "image source is missing or invalid"
				resources[node.ID] = resource
			} else {
				if loading, _ := node.Attribute("loading"); strings.EqualFold(strings.TrimSpace(loading), "lazy") && eligible != nil && !eligible[node.ID] {
					resource.URL, resource.Deferred = candidates[0].String(), true
					resources[node.ID] = resource
					for _, child := range node.Children {
						visit(child)
					}
					return
				}
				var lastTarget *url.URL
				for _, target := range candidates {
					if ctx.Err() != nil {
						return
					}
					lastTarget = target
					resource.URL = target.String()
					if !budget.claim("image:" + resource.URL) {
						resource.Error = "image resource limit exceeded"
						continue
					}
					if target.Scheme != "http" && target.Scheme != "https" {
						resource.Error = "image URL is not a supported HTTP(S) URL"
						continue
					}
					response, loadErr := client.Get(ctx, target)
					if loadErr != nil || response == nil {
						resource.Error = "image request failed"
						continue
					}
					if len(response.Body) > maxImageBytes || !isImageContentType(response.ContentType) {
						resource.Error = "image response was rejected"
						continue
					}
					decoded, decodedWidth, decodedHeight, decodeErr := decodeImageResponseWithBudget(response.Body, response.ContentType, budget)
					if decodeErr != nil {
						resource.Error = "image dimensions were rejected"
						continue
					}
					resource.Loaded, resource.Error = true, ""
					resource.IntrinsicWidth, resource.IntrinsicHeight = float32(decodedWidth), float32(decodedHeight)
					images[resource.URL] = decoded
					break
				}
				resources[node.ID] = resource
				if resource.Error != "" && lastTarget != nil {
					errors = append(errors, resource.Error+": "+network.RedactedURL(lastTarget))
				}
			}
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(document.Root)
	return resources, images, errors
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
