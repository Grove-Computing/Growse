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

	"github.com/Grove-Computing/Growse/internal/dom"
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

func loadReplacedImages(ctx context.Context, client ResourceLoader, baseURL *url.URL, document *dom.Document) (map[dom.NodeID]layout.ImageResource, map[string]image.Image, []string) {
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
			source, exists := node.Attribute("src")
			reference, err := url.Parse(source)
			if !exists || err != nil {
				resource.Error = "image source is missing or invalid"
				resources[node.ID] = resource
			} else {
				target := baseURL.ResolveReference(reference)
				resource.URL = target.String()
				if target.Scheme != "http" && target.Scheme != "https" {
					resource.Error = "image URL is not a supported HTTP(S) URL"
				} else if response, loadErr := client.Get(ctx, target); loadErr != nil || response == nil {
					resource.Error = "image request failed"
				} else if len(response.Body) > maxImageBytes || !isImageContentType(response.ContentType) {
					resource.Error = "image response was rejected"
				} else if config, _, decodeErr := image.DecodeConfig(bytes.NewReader(response.Body)); decodeErr != nil || config.Width <= 0 || config.Height <= 0 || config.Width > maxImagePixels/config.Height {
					resource.Error = "image dimensions were rejected"
				} else if decoded, _, decodeErr := image.Decode(bytes.NewReader(response.Body)); decodeErr != nil {
					resource.Error = "image decode failed"
				} else {
					resource.Loaded = true
					resource.IntrinsicWidth, resource.IntrinsicHeight = float32(config.Width), float32(config.Height)
					images[resource.URL] = decoded
				}
				resources[node.ID] = resource
				if resource.Error != "" {
					errors = append(errors, resource.Error+": "+network.RedactedURL(target))
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
