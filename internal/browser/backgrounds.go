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

	"github.com/saku0512/growse/internal/style"
)

const (
	maxBackgroundImageBytes  = 4 << 20
	maxBackgroundImagePixels = 16 << 20
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
				errors = append(errors, "background image request failed: "+resourceURL.Redacted())
				continue
			}
			if len(response.Body) > maxBackgroundImageBytes || !isImageContentType(response.ContentType) {
				errors = append(errors, "background image response was rejected: "+resourceURL.Redacted())
				continue
			}
			config, _, err := image.DecodeConfig(bytes.NewReader(response.Body))
			if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > maxBackgroundImagePixels/config.Height {
				errors = append(errors, "background image dimensions were rejected: "+resourceURL.Redacted())
				continue
			}
			decoded, _, err := image.Decode(bytes.NewReader(response.Body))
			if err != nil {
				errors = append(errors, "background image decode failed: "+resourceURL.Redacted())
				continue
			}
			images[background.URL] = decoded
		}
	}
	return images, errors
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
	case "image/png", "image/jpeg", "image/gif":
		return true
	default:
		return false
	}
}
