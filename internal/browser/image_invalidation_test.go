package browser

import (
	"context"
	"image"
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/layout"
)

func TestImageCompletionInvalidatesOnlyTargetAndRequiredAncestors(t *testing.T) {
	document := dom.NewDocument()
	body := document.CreateElement("body", nil)
	container := document.CreateElement("section", nil)
	target := document.CreateElement("img", nil)
	sibling := document.CreateElement("aside", nil)
	for _, edge := range [][2]*dom.Node{{document.Root, body}, {body, container}, {container, target}, {body, sibling}} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	page := &Page{
		Document: document, StyleRevision: 7, imageGeneration: 1,
		ImageResources: map[dom.NodeID]layout.ImageResource{target.ID: {IntrinsicWidth: 10, IntrinsicHeight: 10}},
		Images:         map[string]image.Image{},
	}
	resource := layout.ImageResource{URL: "https://example.com/hero.png", Loaded: true, IntrinsicWidth: 20, IntrinsicHeight: 10}
	if !page.commitImageResourceLoad(1, target.ID, resource, image.NewNRGBA(image.Rect(0, 0, 20, 10)), "") {
		t.Fatal("image completion was not committed")
	}
	invalidation := page.ImageInvalidationSnapshot()
	if invalidation.Target != target.ID || !invalidation.IntrinsicChanged || len(invalidation.PaintNodes) != 1 || invalidation.PaintNodes[0] != target.ID {
		t.Fatalf("targeted invalidation = %+v", invalidation)
	}
	want := []dom.NodeID{target.ID, container.ID, body.ID, document.Root.ID}
	if len(invalidation.LayoutAncestors) != len(want) {
		t.Fatalf("layout ancestors = %v, want %v", invalidation.LayoutAncestors, want)
	}
	for index := range want {
		if invalidation.LayoutAncestors[index] != want[index] || invalidation.LayoutAncestors[index] == sibling.ID {
			t.Fatalf("layout ancestors = %v, want %v", invalidation.LayoutAncestors, want)
		}
	}
	if page.StyleRevision != 8 {
		t.Fatalf("intrinsic layout revision = %d", page.StyleRevision)
	}
}

func TestImageCompletionWithStableIntrinsicSizeIsPaintOnly(t *testing.T) {
	document := dom.NewDocument()
	target := document.CreateElement("img", nil)
	if err := document.AppendChild(document.Root, target); err != nil {
		t.Fatal(err)
	}
	page := &Page{
		Document: document, StyleRevision: 3, imageGeneration: 1,
		ImageResources: map[dom.NodeID]layout.ImageResource{target.ID: {IntrinsicWidth: 10, IntrinsicHeight: 10}},
		Images:         map[string]image.Image{},
	}
	resource := layout.ImageResource{URL: "https://example.com/hero.png", Loaded: true, IntrinsicWidth: 10, IntrinsicHeight: 10}
	if !page.commitImageResourceLoad(1, target.ID, resource, image.NewNRGBA(image.Rect(0, 0, 10, 10)), "") {
		t.Fatal("image completion was not committed")
	}
	invalidation := page.ImageInvalidationSnapshot()
	if invalidation.IntrinsicChanged || len(invalidation.LayoutAncestors) != 0 || page.StyleRevision != 3 {
		t.Fatalf("paint-only completion invalidation/revision = %+v / %d", invalidation, page.StyleRevision)
	}
}

func TestImageCompletionRejectsStaleGeneration(t *testing.T) {
	page := &Page{StyleRevision: 4, ImageResources: map[dom.NodeID]layout.ImageResource{}, Images: map[string]image.Image{}}
	staleContext, stale := page.beginImageLoad(context.Background())
	currentContext, current := page.beginImageLoad(context.Background())
	t.Cleanup(func() { page.cancelImageLoads() })
	if staleContext.Err() == nil || currentContext.Err() != nil {
		t.Fatalf("image generation contexts = stale:%v current:%v", staleContext.Err(), currentContext.Err())
	}
	resource := layout.ImageResource{URL: "https://example.com/current.png", Loaded: true, IntrinsicWidth: 20, IntrinsicHeight: 10}
	if page.commitImageResourceLoad(stale, 1, resource, image.NewNRGBA(image.Rect(0, 0, 20, 10)), "") {
		t.Fatal("stale image completion was committed")
	}
	if len(page.ImageResources) != 0 || page.StyleRevision != 4 {
		t.Fatalf("stale image completion changed page: resources=%v revision=%d", page.ImageResources, page.StyleRevision)
	}
	if !page.commitImageResourceLoad(current, 1, resource, image.NewNRGBA(image.Rect(0, 0, 20, 10)), "") {
		t.Fatal("current image completion was rejected")
	}
}

func TestDynamicImageCompletionDoesNotCancelPageImageBatch(t *testing.T) {
	document := dom.NewDocument()
	target := document.CreateElement("img", nil)
	if err := document.AppendChild(document.Root, target); err != nil {
		t.Fatal(err)
	}
	page := &Page{
		Document:       document,
		ImageResources: map[dom.NodeID]layout.ImageResource{},
		Images:         map[string]image.Image{},
	}
	batchContext, generation := page.beginImageLoad(context.Background())
	t.Cleanup(page.cancelImageLoads)

	resource := layout.ImageResource{URL: "https://example.com/dynamic.png", Loaded: true, IntrinsicWidth: 20, IntrinsicHeight: 10}
	page.commitDynamicImageResourceLoad(target.ID, resource, image.NewNRGBA(image.Rect(0, 0, 20, 10)), "")
	if batchContext.Err() != nil {
		t.Fatalf("dynamic image cancelled page batch: %v", batchContext.Err())
	}
	if !page.stageImageLoad(generation, map[dom.NodeID]layout.ImageResource{target.ID: resource}, map[string]image.Image{}, nil, nil, nil, false) {
		t.Fatal("page batch became stale after dynamic image completion")
	}
}

func TestPageRequestsFrameAfterPublishingStagedImages(t *testing.T) {
	page := &Page{ImageResources: map[dom.NodeID]layout.ImageResource{}, Images: map[string]image.Image{}}
	_, generation := page.beginImageLoad(context.Background())
	t.Cleanup(page.cancelImageLoads)
	if !page.stageImageLoad(generation, map[dom.NodeID]layout.ImageResource{}, map[string]image.Image{}, nil, nil, nil, false) {
		t.Fatal("image batch was not staged")
	}
	invalidations := 0
	browserState := &Browser{page: page, onMutation: func() { invalidations++ }}
	if got := browserState.Page(); got != page {
		t.Fatalf("Page() = %p, want %p", got, page)
	}
	if invalidations != 1 {
		t.Fatalf("post-publication invalidations = %d, want 1", invalidations)
	}
	if browserState.Page() != page || invalidations != 1 {
		t.Fatalf("committed batch requested another frame: invalidations=%d", invalidations)
	}
}
