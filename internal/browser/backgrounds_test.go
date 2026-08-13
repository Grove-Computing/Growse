package browser

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/url"
	"testing"

	"github.com/saku0512/growse/internal/dom"
	"github.com/saku0512/growse/internal/network"
	"github.com/saku0512/growse/internal/style"
)

func TestLoadBackgroundImagesDecodesSafeImage(t *testing.T) {
	resourceURL := "https://example.com/assets/card.png"
	var encoded bytes.Buffer
	source := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	source.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	loader := &routeLoader{responses: map[string]*network.Response{
		resourceURL: {URL: mustParseURL(t, resourceURL), ContentType: "image/png", Body: encoded.Bytes()},
	}}
	computed := style.Map{dom.NodeID(1): {BackgroundImage: style.BackgroundImage{Kind: style.BackgroundImageURL, URL: resourceURL}}}
	images, errors := loadBackgroundImages(context.Background(), loader, computed)
	if len(errors) != 0 || images[resourceURL] == nil || images[resourceURL].Bounds().Dx() != 2 {
		t.Fatalf("images/errors = %#v / %#v", images, errors)
	}
}

func TestLoadBackgroundImagesRejectsUnsafeContentTypeWithoutFailingPage(t *testing.T) {
	resourceURL, _ := url.Parse("https://example.com/not-image")
	loader := &routeLoader{responses: map[string]*network.Response{
		resourceURL.String(): {URL: resourceURL, ContentType: "text/html", Body: []byte("<html>")},
	}}
	computed := style.Map{dom.NodeID(1): {BackgroundImage: style.BackgroundImage{Kind: style.BackgroundImageURL, URL: resourceURL.String()}}}
	images, errors := loadBackgroundImages(context.Background(), loader, computed)
	if len(images) != 0 || len(errors) != 1 {
		t.Fatalf("images/errors = %#v / %#v", images, errors)
	}
}
