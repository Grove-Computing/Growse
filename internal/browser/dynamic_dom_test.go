package browser

import (
	"context"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/dom"
	layoutengine "github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	paintmodel "github.com/Grove-Computing/Growse/internal/paint"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/yaegi"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestClassStyleHoverAndWebGoMutationReconcileAnimations(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost/animation-mutation.html")
	loader := stubLoader{response: &network.Response{
		URL: pageURL, StatusCode: 200, ContentType: "text/html",
		Body: []byte(`<!doctype html><html><head><style>
#target { animation: 1s linear infinite base; }
#target:hover { animation-name: hovered; }
#target.active { animation-name: classed; }
</style></head><body><button id="target">Change</button>
<script type="text/go">package main
import "growse/dom"
func main() {
	target := dom.GetElementByID("target")
	target.OnClick(func() {
		target.AddClass("active")
		target.SetAttribute("style", "animation-name: inline")
	})
}</script></body></html>`),
	}}
	current := time.Unix(100, 0)
	browserState := NewWithRuntimeFactory(loader, func() runtimemodel.Runtime { return yaegi.New() })
	clock := &browserFakeClock{current: current}
	browserState.SetAnimationClock(clock)
	var page *Page
	var err error
	var targetID dom.NodeID
	var mutationNames []string
	browserState.SetOnMutation(func() {
		if page == nil {
			return
		}
		samples := page.Animations.Sample(targetID, current)
		if len(samples) == 1 {
			mutationNames = append(mutationNames, samples[0].Name)
		}
	})

	page, err = browserState.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	target, ok := page.Document.GetElementByID("target")
	if !ok {
		t.Fatal("target element was not found")
	}
	targetID = target.ID
	assertAnimationSample(t, page, target.ID, current.Add(250*time.Millisecond), "base", 0.25)

	current = current.Add(250 * time.Millisecond)
	clock.current = current
	if !browserState.UpdateHover(target.ID, 0, 0) {
		t.Fatal("UpdateHover() = false, want true")
	}
	assertAnimationSample(t, page, target.ID, current, "hovered", 0)
	assertAnimationSample(t, page, target.ID, current.Add(250*time.Millisecond), "hovered", 0.25)

	current = current.Add(500 * time.Millisecond)
	clock.current = current
	mutationNames = nil
	if !browserState.DispatchClick(target.ID, 0, 0) {
		t.Fatal("WebGo click mutation was not handled")
	}
	if len(mutationNames) != 2 || mutationNames[0] != "classed" || mutationNames[1] != "inline" {
		t.Fatalf("mutation animation names = %v, want [classed inline]", mutationNames)
	}
	assertAnimationSample(t, page, target.ID, current, "inline", 0)
}

func TestHoverResizeAndScrollDoNotRestartUnchangedAnimation(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost/steady-animation.html")
	loader := stubLoader{response: &network.Response{
		URL: pageURL, StatusCode: 200, ContentType: "text/html",
		Body: []byte(`<style>
#target { animation: 1s linear infinite steady; }
#target:hover { color: blue; }
</style><div id="target">Steady</div>`),
	}}
	start := time.Unix(100, 0)
	current := start
	browserState := New(loader)
	clock := &browserFakeClock{current: current}
	browserState.SetAnimationClock(clock)
	page, err := browserState.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	target, _ := page.Document.GetElementByID("target")

	current = start.Add(250 * time.Millisecond)
	clock.current = current
	if !browserState.UpdateHover(target.ID, 0, 0) {
		t.Fatal("hover state did not change")
	}
	assertAnimationSample(t, page, target.ID, start.Add(500*time.Millisecond), "steady", 0.5)

	current = start.Add(600 * time.Millisecond)
	clock.current = current
	if !browserState.UpdateViewport(640, 480) {
		t.Fatal("viewport did not change")
	}
	assertAnimationSample(t, page, target.ID, start.Add(750*time.Millisecond), "steady", 0.75)

	_ = layoutengine.BuildWithScroll(page.Document, page.ComputedStyles, 640, 480, 0, 100)
	assertAnimationSample(t, page, target.ID, start.Add(900*time.Millisecond), "steady", 0.9)
}

type browserFakeClock struct {
	current time.Time
}

func (clock *browserFakeClock) Now() time.Time {
	return clock.current
}

func assertAnimationSample(t *testing.T, page *Page, nodeID dom.NodeID, current time.Time, name string, progress float64) {
	t.Helper()
	samples := page.Animations.Sample(nodeID, current)
	if len(samples) != 1 || samples[0].Name != name || samples[0].Sample.Progress != progress {
		t.Fatalf("animation sample = %#v, want %s at %v", samples, name, progress)
	}
}

func TestWebGoMutationRecomputesStyles(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost/dynamic.html")
	loader := stubLoader{response: &network.Response{
		URL: pageURL, StatusCode: 200, ContentType: "text/html",
		Body: []byte(`<!doctype html><html><head><style>
.completed { color: #ff0000; }
li.todo { font-size: 24px; animation: 1s linear infinite pulse; }
</style></head><body>
<ul id="list"><li id="existing">Existing</li></ul>
<script type="text/go">package main
import "growse/dom"
func main() {
	existing := dom.GetElementByID("existing")
	existing.AddClass("completed")
	item := dom.CreateElement("li")
	item.AddClass("todo")
	item.SetText("Dynamic")
	dom.GetElementByID("list").AppendChild(item)
	existing.OnClick(func() { item.Remove() })
}</script></body></html>`),
	}}
	browser := NewWithRuntimeFactory(loader, func() runtimemodel.Runtime { return yaegi.New() })

	page, err := browser.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	if !page.RuntimeStarted || page.RuntimeError != "" {
		t.Fatalf("runtime state = started:%v error:%q", page.RuntimeStarted, page.RuntimeError)
	}
	existing, ok := page.Document.GetElementByID("existing")
	if !ok {
		t.Fatal("existing element was not found")
	}
	existingStyle, ok := page.ComputedStyles.For(existing)
	if !ok || existingStyle.Color != 0xff0000ff {
		t.Fatalf("existing style = (%#v, %v), want red", existingStyle, ok)
	}
	dynamic, ok := page.Document.QuerySelector("li.todo")
	if !ok {
		t.Fatal("dynamic element was not found")
	}
	dynamicStyle, ok := page.ComputedStyles.For(dynamic)
	if !ok || dynamicStyle.FontSize != 24 {
		t.Fatalf("dynamic style = (%#v, %v), want 24px", dynamicStyle, ok)
	}
	if page.Animations.Count(dynamic.ID) != 1 {
		t.Fatalf("dynamic animation count = %d, want 1", page.Animations.Count(dynamic.ID))
	}

	tree := layoutengine.Build(page.Document, page.ComputedStyles, 800)
	displayList := paintmodel.Build(tree)
	if !displayListContainsText(displayList, "Dynamic") {
		t.Fatal("dynamic element is missing from the initial display list")
	}
	if !browser.DispatchClick(existing.ID, 0, 0) {
		t.Fatal("remove click was not handled")
	}
	if _, ok := page.Document.QuerySelector("li.todo"); ok {
		t.Fatal("dynamic element remains after WebGo removal")
	}
	if page.Animations.Count(dynamic.ID) != 0 {
		t.Fatalf("removed element animation count = %d, want zero", page.Animations.Count(dynamic.ID))
	}
	updatedTree := layoutengine.Build(page.Document, page.ComputedStyles, 800)
	updatedDisplayList := paintmodel.Build(updatedTree)
	if displayListContainsText(updatedDisplayList, "Dynamic") {
		t.Fatal("removed element remains in the updated display list")
	}
}

func TestHoverRebuildsLayoutAndDisplayList(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost/hover.html")
	loader := stubLoader{response: &network.Response{
		URL: pageURL, StatusCode: 200, ContentType: "text/html",
		Body: []byte(`<!doctype html><html><head><style>
button { color: black; font-size: 16px; }
button:hover { color: red; font-size: 24px; padding: 8px; }
</style></head><body><button id="save">Save</button></body></html>`),
	}}
	browserState := New(loader)
	page, err := browserState.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	button, ok := page.Document.GetElementByID("save")
	if !ok {
		t.Fatal("save button was not found")
	}

	normalTree := layoutengine.Build(page.Document, page.ComputedStyles, 800)
	if !browserState.UpdateHover(button.ID, 12, 34) {
		t.Fatal("UpdateHover() = false, want true")
	}
	hoverTree := layoutengine.Build(page.Document, page.ComputedStyles, 800)
	hoverList := paintmodel.Build(hoverTree)
	if len(normalTree.Boxes) == 0 || len(hoverTree.Boxes) == 0 || len(hoverList.Commands) == 0 {
		t.Fatal("hover layout or display list is empty")
	}
	if normalTree.Boxes[0].Height == hoverTree.Boxes[0].Height {
		t.Fatalf("layout height stayed %v after hover font and padding change", hoverTree.Boxes[0].Height)
	}
	draw, ok := hoverList.Commands[0].(paintmodel.DrawText)
	if !ok || len(draw.Runs) != 1 || draw.Runs[0].Color != 0xff0000ff || draw.Runs[0].FontSize != 24 {
		t.Fatalf("hover display command = %#v, want red 24px text", hoverList.Commands[0])
	}
	if !browserState.ClearHover() {
		t.Fatal("ClearHover() = false, want true")
	}
	restoredList := paintmodel.Build(layoutengine.Build(page.Document, page.ComputedStyles, 800))
	restored, ok := restoredList.Commands[0].(paintmodel.DrawText)
	if !ok || len(restored.Runs) != 1 || restored.Runs[0].Color == 0xff0000ff || restored.Runs[0].FontSize != 16 {
		t.Fatalf("restored display command = %#v, want normal 16px text", restoredList.Commands[0])
	}
}

func TestWebGoMutationAndHoverDiscardGridPositionAndTransformGeometry(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost/grid-dynamic.html")
	loader := stubLoader{response: &network.Response{
		URL: pageURL, StatusCode: 200, ContentType: "text/html",
		Body: []byte(`<!doctype html><html><head><style>
.grid { display:grid; width:200px; grid-template-columns:100px 100px; grid-template-rows:40px; }
.grid.changed { grid-template-columns:150px 50px; }
.item { background-color:#ddd; }
#second:hover { position:relative; left:10px; transform:translateX(25px); }
</style></head><body>
<div id="grid" class="grid"><button id="change" class="item">Change</button><div id="second" class="item">Second</div></div>
<script type="text/go">package main
import "growse/dom"
func main() { dom.GetElementByID("change").OnClick(func() { dom.GetElementByID("grid").AddClass("changed") }) }
</script></body></html>`),
	}}
	browserState := NewWithRuntimeFactory(loader, func() runtimemodel.Runtime { return yaegi.New() })
	page, err := browserState.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	grid, _ := page.Document.GetElementByID("grid")
	change, _ := page.Document.GetElementByID("change")
	second, _ := page.Document.GetElementByID("second")
	initialTree := layoutengine.Build(page.Document, page.ComputedStyles, 400)
	initial := dynamicDecoration(t, initialTree, second.ID)
	if !browserState.DispatchClick(change.ID, 0, 0) {
		t.Fatal("WebGo class mutation was not dispatched")
	}
	mutatedTree := layoutengine.Build(page.Document, page.ComputedStyles, 400)
	mutated := dynamicDecoration(t, mutatedTree, second.ID)
	if mutated.X != initial.X+50 || mutated.Transform != style.IdentityMatrix() {
		t.Fatalf("mutated grid geometry = initial %#v mutated %#v", initial, mutated)
	}
	if !browserState.UpdateHover(second.ID, mutated.X+1, mutated.Y+1) {
		t.Fatal("hover state was not updated")
	}
	hoverStyle, _ := page.ComputedStyles.For(second)
	if hoverStyle.Position != style.PositionRelative || len(hoverStyle.Transform) == 0 {
		t.Fatalf("hover style was not recomputed: %#v", hoverStyle)
	}
	hoverTree := layoutengine.Build(page.Document, page.ComputedStyles, 400)
	hovered := dynamicDecoration(t, hoverTree, second.ID)
	if hovered.X != mutated.X+10 || hovered.Transform == style.IdentityMatrix() {
		t.Fatalf("hover geometry = mutated %#v hovered %#v", mutated, hovered)
	}
	hitX, hitY := hovered.Transform.TransformPoint(hovered.X+1, hovered.Y+1)
	if hit, ok := layoutengine.HitTest(hoverTree, hitX, hitY); !ok || hit != second.ID {
		t.Fatalf("hover transformed hit = (%d, %v)", hit, ok)
	}
	if !browserState.ClearHover() {
		t.Fatal("hover state was not cleared")
	}
	restored := dynamicDecoration(t, layoutengine.Build(page.Document, page.ComputedStyles, 400), second.ID)
	if restored.X != mutated.X || restored.Transform != style.IdentityMatrix() {
		t.Fatalf("stale hover geometry remained = %#v", restored)
	}
	if class, _ := grid.Attribute("class"); class != "grid changed" {
		t.Fatalf("mutation state was lost: %q", class)
	}
}

func dynamicDecoration(t *testing.T, tree *layoutengine.Tree, nodeID dom.NodeID) layoutengine.Decoration {
	t.Helper()
	for _, decoration := range tree.Decorations {
		if decoration.NodeID == nodeID {
			return decoration
		}
	}
	t.Fatalf("decoration for node %d was not found", nodeID)
	return layoutengine.Decoration{}
}

func displayListContainsText(list *paintmodel.DisplayList, text string) bool {
	if list == nil {
		return false
	}
	for _, command := range list.Commands {
		drawText, ok := command.(paintmodel.DrawText)
		if ok && drawText.Text == text {
			return true
		}
	}
	return false
}
