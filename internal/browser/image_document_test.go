package browser

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/network"
)

func TestImageNavigationPublishesDocumentBeforeBodyCompletes(t *testing.T) {
	var encoded bytes.Buffer
	source := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	source.SetNRGBA(1, 1, color.NRGBA{R: 255, A: 255})
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	headersSent := make(chan struct{})
	releaseBody := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "image/png")
		response.WriteHeader(http.StatusOK)
		response.(http.Flusher).Flush()
		close(headersSent)
		<-releaseBody
		_, _ = response.Write(encoded.Bytes())
	}))
	defer server.Close()

	browser := New(network.NewClientWithLimits(server.Client(), 1<<20))
	done := make(chan error, 1)
	go func() {
		_, err := browser.Navigate(context.Background(), server.URL+"/photo.png")
		done <- err
	}()
	select {
	case <-headersSent:
	case <-time.After(time.Second):
		t.Fatal("server did not send image headers")
	}
	deadline := time.Now().Add(time.Second)
	for browser.Page() == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	page := browser.Page()
	if page == nil || page.ContentType != "image/png" {
		t.Fatalf("page before body = %#v", page)
	}
	imageNode, ok := page.Document.QuerySelector("img")
	if !ok || !page.ImageResources[imageNode.ID].Deferred {
		t.Fatal("image placeholder was not published from response headers")
	}
	close(releaseBody)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	resource := page.ImageResources[imageNode.ID]
	if !resource.Loaded || resource.IntrinsicWidth != 3 || resource.IntrinsicHeight != 2 || page.Images[resource.URL] == nil {
		t.Fatalf("completed image resource = %#v", resource)
	}
	if page.Document.ReadyState() != "complete" {
		t.Fatalf("readyState = %q", page.Document.ReadyState())
	}
}
