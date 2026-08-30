package browser

import (
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
