package browser

import (
	"strings"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestAnimationInvalidationStopsForOffscreenHiddenCanceledAndStalePage(t *testing.T) {
	document := dom.NewDocument()
	target := document.CreateElement("div", nil)
	if err := document.AppendChild(document.Root, target); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`
@keyframes pulse { from { opacity: 0; } to { opacity: 1; } }
div { animation: pulse 1s linear infinite; }
`))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Unix(100, 0)
	computed := style.Compute(document, stylesheet)
	page := &Page{Document: document, Stylesheet: stylesheet, ComputedStyles: computed, Animations: style.NewAnimationRegistry(), ViewportWidth: 320, ViewportHeight: 240}
	page.Animations.Reconcile(computed, start)
	tree := &layout.Tree{Width: 320, Height: 240, Bounds: map[dom.NodeID]layout.Rect{target.ID: {X: 10, Y: 10, Width: 40, Height: 40}}}
	if !page.ActiveAnimationsInViewport(start.Add(500*time.Millisecond), tree) {
		t.Fatal("visible animation did not request a frame")
	}
	tree.Bounds[target.ID] = layout.Rect{X: 10, Y: 400, Width: 40, Height: 40}
	if page.ActiveAnimationsInViewport(start.Add(500*time.Millisecond), tree) {
		t.Fatal("offscreen animation continued invalidating")
	}

	computed[target.ID] = func(value style.ComputedStyle) style.ComputedStyle {
		value.Visibility = style.VisibilityHidden
		return value
	}(computed[target.ID])
	tree.Bounds[target.ID] = layout.Rect{X: 10, Y: 10, Width: 40, Height: 40}
	if page.ActiveAnimationsInViewport(start.Add(500*time.Millisecond), tree) {
		t.Fatal("hidden animation continued invalidating")
	}
	page.Animations.Clear()
	if page.ActiveAnimations(start.Add(500 * time.Millisecond)) {
		t.Fatal("canceled animation remained active")
	}

	browserState := New(nil)
	browserState.SetPage(page)
	if !browserState.IsPageVisible(page) {
		t.Fatal("selected page was not visible")
	}
	browserState.SetTabActive(false)
	if browserState.IsPageVisible(page) {
		t.Fatal("background tab kept page visible")
	}
	browserState.SetTabActive(true)
	replacement := &Page{Document: dom.NewDocument(), ComputedStyles: style.Map{}}
	browserState.SetPage(replacement)
	if browserState.IsPageVisible(page) {
		t.Fatal("stale page generation remained visible")
	}
}
