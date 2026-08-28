package browser

import (
	"bytes"
	"context"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"mime"
	"net/url"
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	"github.com/Grove-Computing/Growse/internal/style"
	_ "golang.org/x/image/webp"
)

const (
	maxImageBytes  = 4 << 20
	maxImagePixels = 16 << 20
)

func loadBackgroundImages(ctx context.Context, client ResourceLoader, computed style.Map) (map[string]image.Image, []string) {
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
			response, err := client.Get(ctx, resourceURL)
			if err != nil || response == nil {
				errors = append(errors, "background image request failed: "+network.RedactedURL(resourceURL))
				continue
			}
			if len(response.Body) > maxImageBytes || !isImageContentType(response.ContentType) {
				errors = append(errors, "background image response was rejected: "+network.RedactedURL(resourceURL))
				continue
			}
			config, _, err := image.DecodeConfig(bytes.NewReader(response.Body))
			if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > maxImagePixels/config.Height {
				errors = append(errors, "background image dimensions were rejected: "+network.RedactedURL(resourceURL))
				continue
			}
			decoded, _, err := image.Decode(bytes.NewReader(response.Body))
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
					config, _, decodeErr := image.DecodeConfig(bytes.NewReader(response.Body))
					if decodeErr != nil || config.Width <= 0 || config.Height <= 0 || config.Width > maxImagePixels/config.Height {
						resource.Error = "image dimensions were rejected"
						continue
					}
					decoded, _, decodeErr := image.Decode(bytes.NewReader(response.Body))
					if decodeErr != nil {
						resource.Error = "image decode failed"
						continue
					}
					resource.Loaded, resource.Error = true, ""
					resource.IntrinsicWidth, resource.IntrinsicHeight = float32(config.Width), float32(config.Height)
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
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	default:
		return false
	}
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
	if browserState == nil || page == nil || page.Document == nil {
		return
	}
	var pending []events.Event
	page.imageMu.Lock()
	if page.imageEvents == nil {
		page.imageEvents = make(map[dom.NodeID]string)
	}
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Type == dom.NodeElement && node.TagName == "img" {
			resource, exists := page.ImageResources[node.ID]
			if exists && !resource.Deferred {
				signature := resource.URL + "\x00" + resource.Error
				if resource.Loaded {
					signature += "\x00loaded"
				}
				if page.imageEvents[node.ID] != signature {
					page.imageEvents[node.ID] = signature
					eventType := events.Error
					if resource.Loaded {
						eventType = events.Load
					}
					pending = append(pending, events.New(eventType, node.ID, false, false))
				}
			}
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(page.Document.Root)
	page.imageMu.Unlock()
	for _, event := range pending {
		browserState.dispatchPageEvent(page, event)
	}
}
