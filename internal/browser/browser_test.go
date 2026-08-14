package browser

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/forms"
	"github.com/Grove-Computing/Growse/internal/network"
	"github.com/Grove-Computing/Growse/internal/style"
)

type stubLoader struct {
	response *network.Response
	err      error
}

type routeLoader struct {
	responses map[string]*network.Response
	requested []string
}

type requestRouteLoader struct {
	routeLoader
	request *network.Request
}

func (loader *requestRouteLoader) Do(_ context.Context, request *network.Request) (*network.Response, error) {
	copy := *request
	copy.Body = append([]byte(nil), request.Body...)
	copy.Header = request.Header.Clone()
	loader.request = &copy
	response, ok := loader.responses[request.URL.String()]
	if !ok {
		return nil, errors.New("missing response")
	}
	return response, nil
}

func (loader *routeLoader) Get(_ context.Context, resourceURL *url.URL) (*network.Response, error) {
	loader.requested = append(loader.requested, resourceURL.String())
	response, ok := loader.responses[resourceURL.String()]
	if !ok {
		return nil, errors.New("unexpected URL: " + resourceURL.String())
	}
	return response, nil
}

func (loader stubLoader) Get(context.Context, *url.URL) (*network.Response, error) {
	return loader.response, loader.err
}

func TestNewHasNoActivePage(t *testing.T) {
	browser := New(nil)

	if browser.Page() != nil {
		t.Fatal("a new browser should not have an active page")
	}
}

func TestSetPageReplacesActivePage(t *testing.T) {
	browser := New(nil)
	first := NewPage(mustParseURL(t, "https://example.com/first"))
	second := NewPage(mustParseURL(t, "https://example.com/second"))

	browser.SetPage(first)
	browser.SetPage(second)

	if got := browser.Page(); got != second {
		t.Fatalf("active page = %p, want %p", got, second)
	}
}

func TestSetPageCanClearActivePage(t *testing.T) {
	browser := New(nil)
	browser.SetPage(NewPage(mustParseURL(t, "https://example.com")))

	browser.SetPage(nil)

	if browser.Page() != nil {
		t.Fatal("active page should be cleared")
	}
}

func TestSetInputValueUpdatesActiveTextInput(t *testing.T) {
	document := dom.NewDocument()
	input := document.CreateElement("input", map[string]string{"type": "text"})
	if err := document.AppendChild(document.Root, input); err != nil {
		t.Fatal(err)
	}
	page := NewPage(mustParseURL(t, "http://localhost"))
	page.Document = document
	page.Events = events.NewDispatcher()
	var inputEvent events.Event
	page.Events.AddEventListener(input.ID, events.Input, func(event events.Event) { inputEvent = event })
	browser := New(nil)
	browser.SetPage(page)
	mutations := 0
	browser.SetOnMutation(func() { mutations++ })

	if !browser.SetInputValue(input.ID, "hello") {
		t.Fatal("SetInputValue() = false, want true")
	}
	if got := forms.CurrentValue(input); got != "hello" {
		t.Fatalf("input value = %q, want hello", got)
	}
	if browser.SetInputValue(input.ID, "hello") {
		t.Fatal("SetInputValue() = true for unchanged value")
	}
	if got, want := mutations, 1; got != want {
		t.Fatalf("mutation count = %d, want %d", got, want)
	}
	if inputEvent.Type != events.Input || inputEvent.Target != input.ID || inputEvent.Value != "hello" {
		t.Fatalf("input event = %#v, want updated value", inputEvent)
	}
}

func TestSetInputValueUpdatesTextareaWithNewlines(t *testing.T) {
	document := dom.NewDocument()
	textarea := document.CreateElement("textarea", nil)
	if err := document.AppendChild(document.Root, textarea); err != nil {
		t.Fatal(err)
	}
	page := NewPage(mustParseURL(t, "http://localhost"))
	page.Document = document
	page.Events = events.NewDispatcher()
	browserState := New(nil)
	browserState.SetPage(page)

	if !browserState.SetInputValue(textarea.ID, "first\nsecond") {
		t.Fatal("SetInputValue(textarea) = false, want true")
	}
	if got := forms.CurrentValue(textarea); got != "first\nsecond" {
		t.Fatalf("textarea value = %q", got)
	}
}

func TestSetInputValueUpdatesSupportedTextInputTypes(t *testing.T) {
	for _, inputType := range []string{"password", "email", "url", "number", "unknown-control"} {
		t.Run(inputType, func(t *testing.T) {
			document := dom.NewDocument()
			input := document.CreateElement("input", map[string]string{"type": inputType})
			if err := document.AppendChild(document.Root, input); err != nil {
				t.Fatal(err)
			}
			page := &Page{Document: document, ComputedStyles: style.Compute(document, nil), Events: events.NewDispatcher()}
			browser := New(nil)
			browser.SetPage(page)
			if !browser.SetInputValue(input.ID, "edited") {
				t.Fatalf("SetInputValue(%s) = false", inputType)
			}
			if value := forms.CurrentValue(input); value != "edited" {
				t.Fatalf("value = %q", value)
			}
		})
	}
}

func TestSetInputValueRejectsUnsupportedOrInactiveNode(t *testing.T) {
	document := dom.NewDocument()
	checkbox := document.CreateElement("input", map[string]string{"type": "checkbox"})
	paragraph := document.CreateElement("p", nil)
	detached := document.CreateElement("input", nil)
	for _, node := range []*dom.Node{checkbox, paragraph} {
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	page := NewPage(mustParseURL(t, "http://localhost"))
	page.Document = document
	browser := New(nil)
	browser.SetPage(page)

	for _, node := range []*dom.Node{checkbox, paragraph, detached} {
		if browser.SetInputValue(node.ID, "value") {
			t.Fatalf("SetInputValue(%s) = true, want false", node.TagName)
		}
	}
}

func TestCommitInputValueDispatchesChangeEvent(t *testing.T) {
	document := dom.NewDocument()
	input := document.CreateElement("input", map[string]string{"id": "query", "value": "hello"})
	if err := document.AppendChild(document.Root, input); err != nil {
		t.Fatal(err)
	}
	page := NewPage(mustParseURL(t, "http://localhost"))
	page.Document = document
	page.Events = events.NewDispatcher()
	var received events.Event
	page.Events.AddEventListener(input.ID, events.Change, func(event events.Event) { received = event })
	browser := New(nil)
	browser.SetPage(page)

	if !browser.CommitInputValue(input.ID, "hello") {
		t.Fatal("CommitInputValue() = false, want handled event")
	}
	if received.Type != events.Change || received.Target != input.ID || received.Value != "hello" {
		t.Fatalf("change event = %#v, want committed value", received)
	}
}

func TestSubmitFormDispatchesToNearestForm(t *testing.T) {
	document := dom.NewDocument()
	form := document.CreateElement("form", map[string]string{"id": "search"})
	input := document.CreateElement("input", map[string]string{"id": "query"})
	if err := document.AppendChild(document.Root, form); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(form, input); err != nil {
		t.Fatal(err)
	}
	page := NewPage(mustParseURL(t, "http://localhost"))
	page.Document = document
	page.Events = events.NewDispatcher()
	var received events.Event
	page.Events.AddEventListener(form.ID, events.Submit, func(event events.Event) { received = event })
	browser := New(nil)
	browser.SetPage(page)

	if !browser.SubmitForm(input.ID) {
		t.Fatal("SubmitForm() = false, want true")
	}
	if received.Type != events.Submit || received.Target != form.ID {
		t.Fatalf("submit event = %#v, want form target", received)
	}
}

func TestDispatchClickSubmitsSubmitButton(t *testing.T) {
	document := dom.NewDocument()
	form := document.CreateElement("form", nil)
	button := document.CreateElement("button", map[string]string{"type": "submit"})
	if err := document.AppendChild(document.Root, form); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(form, button); err != nil {
		t.Fatal(err)
	}
	page := NewPage(mustParseURL(t, "http://localhost"))
	page.Document = document
	page.Events = events.NewDispatcher()
	submitted := false
	page.Events.AddEventListener(form.ID, events.Submit, func(events.Event) { submitted = true })
	browser := New(nil)
	browser.SetPage(page)

	if !browser.DispatchClick(button.ID, 0, 0) || !submitted {
		t.Fatal("submit button click did not submit its form")
	}
}

func TestDispatchClickUsesExplicitFormOwnerAndStoresSubmitter(t *testing.T) {
	document := dom.NewDocument()
	form := document.CreateElement("form", map[string]string{"id": "search"})
	button := document.CreateElement("button", map[string]string{"form": "search", "formaction": "/alternate"})
	if err := document.AppendChild(document.Root, form); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(document.Root, button); err != nil {
		t.Fatal(err)
	}
	dispatcher := events.NewDispatcher()
	submitted := false
	dispatcher.AddEventListener(form.ID, events.Submit, func(events.Event) { submitted = true })
	page := &Page{Document: document, ComputedStyles: style.Compute(document, nil), Events: dispatcher}
	browser := New(nil)
	browser.SetPage(page)

	if !browser.DispatchClick(button.ID, 1, 2) || !submitted || page.Submitter != button.ID {
		t.Fatalf("submitted=%v submitter=%d", submitted, page.Submitter)
	}
	config, ok := forms.ResolveSubmission(document, button)
	if !ok || config.Form != form || config.Action != "/alternate" {
		t.Fatalf("submission config = %#v, ok=%v", config, ok)
	}
}

func TestUpdateHoverTracksAncestorPathAndRecomputesStyles(t *testing.T) {
	document := dom.NewDocument()
	button := document.CreateElement("button", map[string]string{"id": "save"})
	label := document.CreateElement("span", map[string]string{"id": "label"})
	if err := document.AppendChild(document.Root, button); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(button, label); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`button:hover { color: red } span:hover { font-size: 24px }`))
	if err != nil {
		t.Fatal(err)
	}
	page := NewPage(mustParseURL(t, "http://localhost"))
	page.Document = document
	page.Stylesheet = stylesheet
	page.ComputedStyles = style.Compute(document, stylesheet)
	browser := New(nil)
	browser.SetPage(page)
	invalidations := 0
	browser.SetOnMutation(func() { invalidations++ })

	if !browser.UpdateHover(label.ID, 12, 34) {
		t.Fatal("UpdateHover() = false, want changed")
	}
	if got, want := page.HoverPath, []dom.NodeID{button.ID, label.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("hover path = %v, want %v", got, want)
	}
	buttonStyle, _ := page.ComputedStyles.For(button)
	labelStyle, _ := page.ComputedStyles.For(label)
	if buttonStyle.Color != 0xff0000ff || labelStyle.FontSize != 24 {
		t.Fatalf("hover styles = button:%#v label:%#v", buttonStyle, labelStyle)
	}
	if browser.UpdateHover(label.ID, 12, 34) {
		t.Fatal("same hover path requested a recalculation")
	}
	if !browser.ClearHover() || len(page.HoverPath) != 0 || page.HoverTarget != 0 {
		t.Fatalf("ClearHover left state target:%d path:%v", page.HoverTarget, page.HoverPath)
	}
	buttonStyle, _ = page.ComputedStyles.For(button)
	if buttonStyle.Color == 0xff0000ff {
		t.Fatal("hover style remains after ClearHover")
	}
	if got, want := invalidations, 2; got != want {
		t.Fatalf("invalidation count = %d, want %d", got, want)
	}
}

func TestHoverTransitionFlowsThroughPageFrameStylesAndReverses(t *testing.T) {
	document := dom.NewDocument()
	button := document.CreateElement("button", nil)
	if err := document.AppendChild(document.Root, button); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`
button { opacity: 0; transition: opacity 1s linear; }
button:hover { opacity: 1; }
`))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, time.August, 14, 12, 0, 0, 0, time.UTC)
	clock := &browserFakeClock{current: start}
	page := NewPage(mustParseURL(t, "http://localhost"))
	page.Document, page.Stylesheet = document, stylesheet
	page.ComputedStyles = style.Compute(document, stylesheet)
	browserState := New(nil)
	browserState.SetAnimationClock(clock)
	browserState.SetPage(page)

	if !browserState.UpdateHover(button.ID, 0, 0) {
		t.Fatal("hover did not start transition")
	}
	midpoint, _ := page.AnimatedStyles(start.Add(500 * time.Millisecond)).For(button)
	if midpoint.Opacity != 0.5 || !page.ActiveAnimations(start.Add(500*time.Millisecond)) {
		t.Fatalf("hover midpoint = %v, active=%v; want 0.5, true", midpoint.Opacity, page.ActiveAnimations(start.Add(500*time.Millisecond)))
	}

	clock.current = start.Add(500 * time.Millisecond)
	if !browserState.ClearHover() {
		t.Fatal("clear hover did not reverse transition")
	}
	reversing, _ := page.AnimatedStyles(start.Add(750 * time.Millisecond)).For(button)
	if reversing.Opacity != 0.25 {
		t.Fatalf("reversing opacity = %v, want 0.25", reversing.Opacity)
	}
	finished, _ := page.AnimatedStyles(start.Add(time.Second)).For(button)
	if finished.Opacity != 0 || page.ActiveAnimations(start.Add(time.Second)) || page.Transitions.Count(button.ID) != 0 {
		t.Fatalf("finished transition = opacity:%v active:%v count:%d", finished.Opacity, page.ActiveAnimations(start.Add(time.Second)), page.Transitions.Count(button.ID))
	}
}

func TestUpdateFocusRecomputesStyles(t *testing.T) {
	document := dom.NewDocument()
	input := document.CreateElement("input", nil)
	if err := document.AppendChild(document.Root, input); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`input:focus { color: red; font-size: 24px }`))
	if err != nil {
		t.Fatal(err)
	}
	page := NewPage(mustParseURL(t, "http://localhost"))
	page.Document = document
	page.Stylesheet = stylesheet
	page.ComputedStyles = style.Compute(document, stylesheet)
	browser := New(nil)
	browser.SetPage(page)
	invalidations := 0
	browser.SetOnMutation(func() { invalidations++ })

	if !browser.UpdateFocus(input.ID) || page.FocusTarget != input.ID {
		t.Fatalf("focused target = %d, want %d", page.FocusTarget, input.ID)
	}
	focused, _ := page.ComputedStyles.For(input)
	if focused.Color != 0xff0000ff || focused.FontSize != 24 {
		t.Fatalf("focused style = %#v", focused)
	}
	if browser.UpdateFocus(input.ID) {
		t.Fatal("same focus target requested a recalculation")
	}
	if !browser.UpdateFocus(0) || page.FocusTarget != 0 {
		t.Fatalf("cleared focus target = %d", page.FocusTarget)
	}
	normal, _ := page.ComputedStyles.For(input)
	if normal.Color == 0xff0000ff || normal.FontSize == 24 {
		t.Fatalf("focus style remains after clearing: %#v", normal)
	}
	if invalidations != 2 {
		t.Fatalf("invalidation count = %d, want 2", invalidations)
	}
}

func TestUpdateViewportRecomputesRelativeUnits(t *testing.T) {
	document := dom.NewDocument()
	paragraph := document.CreateElement("p", nil)
	if err := document.AppendChild(document.Root, paragraph); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`
p { font-size: 10vw; padding: 5vh }
@media (max-width: 900px) { p { color: red } }
`))
	if err != nil {
		t.Fatal(err)
	}
	page := NewPage(mustParseURL(t, "http://localhost"))
	page.Document, page.Stylesheet = document, stylesheet
	page.ComputedStyles = style.Compute(document, stylesheet)
	browser := New(nil)
	browser.SetPage(page)

	if !browser.UpdateViewport(800, 600) {
		t.Fatal("UpdateViewport() = false, want changed")
	}
	computed, _ := page.ComputedStyles.For(paragraph)
	if computed.FontSize != 80 || computed.Padding.Top != 30 || computed.Color != 0xff0000ff {
		t.Fatalf("viewport-relative style = %#v", computed)
	}
	if browser.UpdateViewport(800, 600) {
		t.Fatal("same viewport requested a recalculation")
	}
}

func TestUpdateHoverRejectsRemovedElement(t *testing.T) {
	document := dom.NewDocument()
	button := document.CreateElement("button", nil)
	if err := document.AppendChild(document.Root, button); err != nil {
		t.Fatal(err)
	}
	page := NewPage(mustParseURL(t, "http://localhost"))
	page.Document = document
	browser := New(nil)
	browser.SetPage(page)
	if _, ok := document.Remove(button.ID); !ok {
		t.Fatal("Remove() = false, want true")
	}
	if browser.UpdateHover(button.ID, 12, 34) || len(page.HoverPath) != 0 {
		t.Fatal("removed element became hovered")
	}
}

func TestUpdateHoverDispatchesPathDifferenceInOrder(t *testing.T) {
	document := dom.NewDocument()
	parent := document.CreateElement("section", map[string]string{"id": "parent"})
	first := document.CreateElement("button", map[string]string{"id": "first"})
	second := document.CreateElement("button", map[string]string{"id": "second"})
	for _, edge := range [][2]*dom.Node{
		{document.Root, parent},
		{parent, first},
		{parent, second},
	} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	dispatcher := events.NewDispatcher()
	var received []string
	for _, node := range []*dom.Node{parent, first, second} {
		node := node
		dispatcher.AddEventListener(node.ID, events.MouseEnter, func(event events.Event) {
			received = append(received, string(event.Type)+":"+node.Attributes["id"])
			if event.X != 12 || event.Y != 34 {
				t.Errorf("event coordinates = (%v, %v), want (12, 34)", event.X, event.Y)
			}
		})
		dispatcher.AddEventListener(node.ID, events.MouseLeave, func(event events.Event) {
			received = append(received, string(event.Type)+":"+node.Attributes["id"])
		})
	}
	page := NewPage(mustParseURL(t, "http://localhost"))
	page.Document = document
	page.Events = dispatcher
	browser := New(nil)
	browser.SetPage(page)

	if !browser.UpdateHover(first.ID, 12, 34) {
		t.Fatal("first UpdateHover() = false, want true")
	}
	if browser.UpdateHover(first.ID, 99, 99) {
		t.Fatal("same path UpdateHover() = true, want false")
	}
	if !browser.UpdateHover(second.ID, 12, 34) {
		t.Fatal("second UpdateHover() = false, want true")
	}
	if got, want := received, []string{
		"mouseenter:parent", "mouseenter:first",
		"mouseleave:first", "mouseenter:second",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("hover events = %v, want %v", got, want)
	}
}

func TestNavigateLoadsHTMLAndUpdatesPage(t *testing.T) {
	finalURL := mustParseURL(t, "https://example.com/final")
	browser := New(stubLoader{response: &network.Response{
		URL:         finalURL,
		StatusCode:  200,
		ContentType: "text/html; charset=utf-8",
		Body:        []byte("<h1>Hello</h1>"),
	}})

	page, err := browser.Navigate(context.Background(), "example.com/start#section")
	if err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	if browser.Page() != page {
		t.Fatal("successful navigation did not activate the loaded page")
	}
	if got, want := page.URL.String(), finalURL.String(); got != want {
		t.Fatalf("page URL = %q, want %q", got, want)
	}
	if got, want := string(page.Source), "<h1>Hello</h1>"; got != want {
		t.Fatalf("page source = %q, want %q", got, want)
	}
	if page.Document == nil || page.Document.ElementCount() == 0 {
		t.Fatal("successful navigation did not build a DOM")
	}
}

func TestNavigatePreservesPageOnFailure(t *testing.T) {
	loadErr := errors.New("network unavailable")
	browser := New(stubLoader{err: loadErr})
	current := NewPage(mustParseURL(t, "https://example.com/current"))
	browser.SetPage(current)

	_, err := browser.Navigate(context.Background(), "https://example.com/next")
	if !errors.Is(err, loadErr) {
		t.Fatalf("Navigate() error = %v, want %v", err, loadErr)
	}
	if browser.Page() != current {
		t.Fatal("failed navigation replaced the active page")
	}
}

func TestBackAndForwardLoadHistoryEntries(t *testing.T) {
	firstURL := mustParseURL(t, "https://example.com/first")
	secondURL := mustParseURL(t, "https://example.com/second")
	loader := &routeLoader{responses: map[string]*network.Response{
		firstURL.String():  {URL: firstURL, StatusCode: 200, ContentType: "text/html", Body: []byte("<p>First</p>")},
		secondURL.String(): {URL: secondURL, StatusCode: 200, ContentType: "text/html", Body: []byte("<p>Second</p>")},
	}}
	browser := New(loader)
	if _, err := browser.Navigate(context.Background(), firstURL.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := browser.Navigate(context.Background(), secondURL.String()); err != nil {
		t.Fatal(err)
	}
	if !browser.CanBack() || browser.CanForward() {
		t.Fatalf("history state after navigation = back:%v forward:%v", browser.CanBack(), browser.CanForward())
	}

	page, err := browser.Back(context.Background())
	if err != nil || page.URL.String() != firstURL.String() {
		t.Fatalf("Back() = (%v, %v), want first page", page, err)
	}
	if browser.CanBack() || !browser.CanForward() {
		t.Fatalf("history state after Back = back:%v forward:%v", browser.CanBack(), browser.CanForward())
	}

	page, err = browser.Forward(context.Background())
	if err != nil || page.URL.String() != secondURL.String() {
		t.Fatalf("Forward() = (%v, %v), want second page", page, err)
	}
}

func TestFailedBackPreservesPageAndHistoryIndex(t *testing.T) {
	firstURL := mustParseURL(t, "https://example.com/first")
	secondURL := mustParseURL(t, "https://example.com/second")
	loader := &routeLoader{responses: map[string]*network.Response{
		firstURL.String():  {URL: firstURL, StatusCode: 200, ContentType: "text/html", Body: []byte("<p>First</p>")},
		secondURL.String(): {URL: secondURL, StatusCode: 200, ContentType: "text/html", Body: []byte("<p>Second</p>")},
	}}
	browser := New(loader)
	if _, err := browser.Navigate(context.Background(), firstURL.String()); err != nil {
		t.Fatal(err)
	}
	current, err := browser.Navigate(context.Background(), secondURL.String())
	if err != nil {
		t.Fatal(err)
	}
	delete(loader.responses, firstURL.String())

	if _, err := browser.Back(context.Background()); err == nil {
		t.Fatal("Back() error = nil, want load failure")
	}
	if browser.Page() != current || !browser.CanBack() || browser.CanForward() {
		t.Fatal("failed Back changed the active page or history position")
	}
}

func TestReloadDoesNotAddHistoryEntry(t *testing.T) {
	pageURL := mustParseURL(t, "https://example.com/page")
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {URL: pageURL, StatusCode: 200, ContentType: "text/html", Body: []byte("<p>Page</p>")},
	}}
	browser := New(loader)
	if _, err := browser.Navigate(context.Background(), pageURL.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := browser.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, want := len(browser.history.entries), 1; got != want {
		t.Fatalf("history entries after Reload = %d, want %d", got, want)
	}
}

func TestNavigationAndReloadDiscardPreviousPageAnimations(t *testing.T) {
	firstURL := mustParseURL(t, "https://example.com/first")
	secondURL := mustParseURL(t, "https://example.com/second")
	body := []byte(`<style>.running { animation: 1s linear infinite pulse; }</style><div class="running"></div>`)
	loader := &routeLoader{responses: map[string]*network.Response{
		firstURL.String():  {URL: firstURL, StatusCode: 200, ContentType: "text/html", Body: body},
		secondURL.String(): {URL: secondURL, StatusCode: 200, ContentType: "text/html", Body: body},
	}}
	browser := New(loader)
	first, err := browser.Navigate(context.Background(), firstURL.String())
	if err != nil {
		t.Fatal(err)
	}
	firstNode, _ := first.Document.QuerySelector(".running")
	if first.Animations.Count(firstNode.ID) != 1 {
		t.Fatal("first page animation was not registered")
	}

	second, err := browser.Navigate(context.Background(), secondURL.String())
	if err != nil {
		t.Fatal(err)
	}
	if first.Animations.Count(firstNode.ID) != 0 {
		t.Fatalf("first page animation survived navigation: %d", first.Animations.Count(firstNode.ID))
	}
	secondNode, _ := second.Document.QuerySelector(".running")
	if second.Animations.Count(secondNode.ID) != 1 {
		t.Fatal("second page animation was not registered")
	}

	reloaded, err := browser.Reload(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.Animations.Count(secondNode.ID) != 0 {
		t.Fatalf("second page animation survived reload: %d", second.Animations.Count(secondNode.ID))
	}
	reloadedNode, _ := reloaded.Document.QuerySelector(".running")
	if reloaded.Animations.Count(reloadedNode.ID) != 1 {
		t.Fatal("reloaded page animation was not registered")
	}
}

func TestBrowserReducedMotionSettingRecomputesAuthorMediaQuery(t *testing.T) {
	pageURL := mustParseURL(t, "https://example.com/motion")
	browser := New(stubLoader{response: &network.Response{
		URL: pageURL, StatusCode: 200, ContentType: "text/html",
		Body: []byte(`<style>
#target { animation: 1s linear infinite moving; }
@media (prefers-reduced-motion: reduce) { #target { animation-name: none; } }
</style><div id="target"></div>`),
	}})
	page, err := browser.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	target, _ := page.Document.GetElementByID("target")
	if page.Animations.Count(target.ID) != 1 {
		t.Fatal("default no-preference animation is missing")
	}
	if !browser.SetReducedMotion(true) || !page.ReducedMotion {
		t.Fatal("reduced-motion setting did not change")
	}
	if page.Animations.Count(target.ID) != 0 {
		t.Fatalf("author reduce rule was not applied: %d animations", page.Animations.Count(target.ID))
	}
	if browser.SetReducedMotion(true) {
		t.Fatal("unchanged reduced-motion setting reported a change")
	}
}

func TestNavigateRejectsUnsupportedContentType(t *testing.T) {
	browser := New(stubLoader{response: &network.Response{
		URL:         mustParseURL(t, "https://example.com/image.png"),
		StatusCode:  200,
		ContentType: "image/png",
	}})

	if _, err := browser.Navigate(context.Background(), "https://example.com/image.png"); err == nil {
		t.Fatal("Navigate() error = nil, want unsupported Content-Type error")
	}
}

func TestNavigateLoadsInlineAndSameOriginStylesheets(t *testing.T) {
	pageURL := mustParseURL(t, "https://example.com/index.html")
	cssURL := mustParseURL(t, "https://example.com/site.css")
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {
			URL: pageURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<html><head>
<link rel="stylesheet" href="/site.css">
<link rel="stylesheet" href="https://cdn.example.org/ignored.css">
<style>#title { font-size: 30px; }</style>
</head><body><h1 id="title" class="hero">Hello</h1></body></html>`),
		},
		cssURL.String(): {
			URL: cssURL, StatusCode: 200, ContentType: "text/css",
			Body: []byte(`.hero { color: #123456; }`),
		},
	}}
	browser := New(loader)

	page, err := browser.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	if got, want := len(loader.requested), 2; got != want {
		t.Fatalf("request count = %d, want %d (%v)", got, want, loader.requested)
	}
	title, ok := page.Document.GetElementByID("title")
	if !ok {
		t.Fatal("title element was not indexed")
	}
	computed, ok := page.ComputedStyles.For(title)
	if !ok {
		t.Fatal("title element has no computed style")
	}
	if got, want := computed.Color, uint32(0x123456ff); got != want {
		t.Fatalf("title color = %#x, want %#x", got, want)
	}
	if got, want := computed.FontSize, float32(30); got != want {
		t.Fatalf("title font size = %v, want %v", got, want)
	}
}

func TestNavigateLoadsSameOriginImportsWithMediaAndStopsCycles(t *testing.T) {
	pageURL := mustParseURL(t, "https://example.com/index.html")
	baseURL := mustParseURL(t, "https://example.com/css/base.css")
	colorsURL := mustParseURL(t, "https://example.com/css/colors.css")
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {
			URL: pageURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<style>
@import "/css/base.css";
.hero { background-color: blue; }
</style><h1 class="hero">Hello</h1>`),
		},
		baseURL.String(): {
			URL: baseURL, StatusCode: 200, ContentType: "text/css",
			Body: []byte(`
@import "colors.css" screen and (min-width: 1000px);
@import "https://evil.example/ignored.css";
.hero { font-size: 20px; }
`),
		},
		colorsURL.String(): {
			URL: colorsURL, StatusCode: 200, ContentType: "text/css",
			Body: []byte(`@import "base.css"; .hero { color: red; }`),
		},
	}}
	browser := New(loader)
	page, err := browser.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := loader.requested, []string{pageURL.String(), baseURL.String(), colorsURL.String()}; !reflect.DeepEqual(got, want) {
		t.Fatalf("stylesheet requests = %v, want %v", got, want)
	}
	heading, ok := page.Document.QuerySelector("h1")
	if !ok {
		t.Fatal("heading was not found")
	}
	computed, _ := page.ComputedStyles.For(heading)
	if computed.Color != 0xff0000ff || computed.FontSize != 20 || computed.BackgroundColor != 0x0000ffff {
		t.Fatalf("imported style = %#v", computed)
	}
}

func TestStylesheetLoadLimits(t *testing.T) {
	state := &stylesheetLoadState{totalBytes: maxCSSTotalBytes - 1}
	if !state.consumeBytes(1) || state.consumeBytes(1) {
		t.Fatal("stylesheet total byte limit was not enforced")
	}
	loader := &routeLoader{responses: map[string]*network.Response{}}
	state = &stylesheetLoadState{
		client: loader, origin: mustParseURL(t, "https://example.com/"),
		activeURLs: make(map[string]bool), fetches: maxCSSStylesheetCount,
	}
	stylesheet, err := state.loadExternal(context.Background(), mustParseURL(t, "https://example.com/extra.css"), 0)
	if err != nil || len(stylesheet.Rules) != 0 || len(loader.requested) != 0 {
		t.Fatalf("fetch limit result = sheet:%#v err:%v requests:%v", stylesheet, err, loader.requested)
	}
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		want    string
		wantErr bool
	}{
		{name: "HTTPS default", rawURL: "example.com/path", want: "https://example.com/path"},
		{name: "localhost HTTP", rawURL: "localhost:8080", want: "http://localhost:8080"},
		{name: "trim and remove fragment", rawURL: " https://example.com/#top ", want: "https://example.com/"},
		{name: "empty", rawURL: " ", wantErr: true},
		{name: "unsupported scheme", rawURL: "file:///tmp/index.html", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeURL(test.rawURL)
			if test.wantErr {
				if err == nil {
					t.Fatalf("normalizeURL(%q) error = nil", test.rawURL)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeURL(%q) error = %v", test.rawURL, err)
			}
			if got.String() != test.want {
				t.Fatalf("normalizeURL(%q) = %q, want %q", test.rawURL, got, test.want)
			}
		})
	}
}

func TestSetSelectValueChangesEnabledOptionAndDispatchesEvents(t *testing.T) {
	document := dom.NewDocument()
	selectNode := document.CreateElement("select", nil)
	first := document.CreateElement("option", map[string]string{"value": "one"})
	second := document.CreateElement("option", map[string]string{"value": "two"})
	for _, edge := range [][2]*dom.Node{{document.Root, selectNode}, {selectNode, first}, {first, document.CreateText("One")}, {selectNode, second}, {second, document.CreateText("Two")}} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	page := &Page{Document: document, ComputedStyles: style.Compute(document, nil), Events: events.NewDispatcher()}
	var received []events.Type
	page.Events.AddEventListener(selectNode.ID, events.Input, func(event events.Event) { received = append(received, event.Type) })
	page.Events.AddEventListener(selectNode.ID, events.Change, func(event events.Event) { received = append(received, event.Type) })
	browser := New(nil)
	browser.SetPage(page)

	if !browser.SetSelectValue(selectNode.ID, "two") {
		t.Fatal("SetSelectValue returned false")
	}
	if got := forms.CurrentValue(selectNode); got != "two" {
		t.Fatalf("select value = %q", got)
	}
	if !reflect.DeepEqual(received, []events.Type{events.Input, events.Change}) {
		t.Fatalf("events = %#v", received)
	}
}

func TestActivateCheckableUpdatesRadioGroupAndDispatchesEvents(t *testing.T) {
	document := dom.NewDocument()
	form := document.CreateElement("form", nil)
	first := document.CreateElement("input", map[string]string{"type": "radio", "name": "plan", "checked": ""})
	second := document.CreateElement("input", map[string]string{"type": "radio", "name": "plan"})
	for _, edge := range [][2]*dom.Node{{document.Root, form}, {form, first}, {form, second}} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	page := &Page{Document: document, ComputedStyles: style.Compute(document, nil), Events: events.NewDispatcher()}
	var received []events.Type
	page.Events.AddEventListener(second.ID, events.Input, func(event events.Event) { received = append(received, event.Type) })
	page.Events.AddEventListener(second.ID, events.Change, func(event events.Event) { received = append(received, event.Type) })
	browser := New(nil)
	browser.SetPage(page)

	if !browser.ActivateCheckable(second.ID) {
		t.Fatal("ActivateCheckable returned false")
	}
	if forms.CurrentChecked(first) {
		t.Fatal("first radio remained checked")
	}
	if !forms.CurrentChecked(second) {
		t.Fatal("second radio was not checked")
	}
	if !reflect.DeepEqual(received, []events.Type{events.Input, events.Change}) {
		t.Fatalf("events = %#v", received)
	}
}

func TestDisabledReadonlyLabelAndResetBehavior(t *testing.T) {
	document := dom.NewDocument()
	form := document.CreateElement("form", nil)
	readonly := document.CreateElement("input", map[string]string{"id": "readonly", "value": "default", "readonly": ""})
	disabled := document.CreateElement("input", map[string]string{"id": "disabled", "disabled": ""})
	checkbox := document.CreateElement("input", map[string]string{"id": "accept", "type": "checkbox"})
	label := document.CreateElement("label", map[string]string{"for": "accept"})
	labelText := document.CreateText("Accept")
	for _, edge := range [][2]*dom.Node{{document.Root, form}, {form, readonly}, {form, disabled}, {form, checkbox}, {form, label}, {label, labelText}} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	page := &Page{Document: document, ComputedStyles: style.Compute(document, nil), Events: events.NewDispatcher()}
	resetEvents := 0
	page.Events.AddEventListener(form.ID, events.Reset, func(events.Event) { resetEvents++ })
	browser := New(nil)
	browser.SetPage(page)

	if browser.SetInputValue(readonly.ID, "changed") || browser.SetInputValue(disabled.ID, "changed") {
		t.Fatal("readonly or disabled input accepted editing")
	}
	if !browser.DispatchClick(labelText.ID, 10, 10) || !forms.CurrentChecked(checkbox) || page.FocusTarget != checkbox.ID {
		t.Fatalf("label activation: checked=%v focus=%d", forms.CurrentChecked(checkbox), page.FocusTarget)
	}
	if !browser.ResetForm(form.ID) || forms.CurrentChecked(checkbox) || forms.CurrentValue(readonly) != "default" || resetEvents != 1 {
		t.Fatalf("reset: checked=%v value=%q events=%d", forms.CurrentChecked(checkbox), forms.CurrentValue(readonly), resetEvents)
	}
}

func TestMoveFormFocusTraversesAndWrapsBothDirections(t *testing.T) {
	document := dom.NewDocument()
	first := document.CreateElement("input", nil)
	disabled := document.CreateElement("input", map[string]string{"disabled": ""})
	last := document.CreateElement("button", nil)
	for _, node := range []*dom.Node{first, disabled, last} {
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	page := &Page{Document: document, ComputedStyles: style.Compute(document, nil)}
	browser := New(nil)
	browser.SetPage(page)

	if !browser.MoveFormFocus(false) || page.FocusTarget != first.ID {
		t.Fatalf("first forward focus = %d", page.FocusTarget)
	}
	if !browser.MoveFormFocus(false) || page.FocusTarget != last.ID {
		t.Fatalf("second forward focus = %d", page.FocusTarget)
	}
	if !browser.MoveFormFocus(true) || page.FocusTarget != first.ID {
		t.Fatalf("reverse focus = %d", page.FocusTarget)
	}
}

func TestUpdateFocusDispatchesBlurBeforeFocus(t *testing.T) {
	document := dom.NewDocument()
	first := document.CreateElement("input", nil)
	second := document.CreateElement("input", nil)
	if err := document.AppendChild(document.Root, first); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(document.Root, second); err != nil {
		t.Fatal(err)
	}
	dispatcher := events.NewDispatcher()
	var order []string
	dispatcher.AddEventListener(first.ID, events.Focus, func(events.Event) { order = append(order, "focus:first") })
	dispatcher.AddEventListener(first.ID, events.Blur, func(events.Event) { order = append(order, "blur:first") })
	dispatcher.AddEventListener(second.ID, events.Focus, func(events.Event) { order = append(order, "focus:second") })
	page := &Page{Document: document, ComputedStyles: style.Compute(document, nil), Events: dispatcher}
	browser := New(nil)
	browser.SetPage(page)

	if !browser.UpdateFocus(first.ID) || !browser.UpdateFocus(second.ID) {
		t.Fatal("focus transition was not applied")
	}
	want := []string{"focus:first", "blur:first", "focus:second"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("event order = %v, want %v", order, want)
	}
}

func TestValidateFormFocusesFirstInvalidControlAndUpdatesPseudoClass(t *testing.T) {
	document := dom.NewDocument()
	form := document.CreateElement("form", nil)
	first := document.CreateElement("input", map[string]string{"required": "", "class": "field"})
	second := document.CreateElement("input", map[string]string{"type": "email", "class": "field"})
	for _, edge := range [][2]*dom.Node{{document.Root, form}, {form, first}, {form, second}} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	stylesheet, err := css.Parse(strings.NewReader(`.field:valid { color: green } .field:invalid { color: red }`))
	if err != nil {
		t.Fatal(err)
	}
	page := &Page{Document: document, Stylesheet: stylesheet, ComputedStyles: style.Compute(document, stylesheet), Events: events.NewDispatcher()}
	browser := New(nil)
	browser.SetPage(page)

	if browser.ValidateForm(form.ID) || page.FocusTarget != first.ID {
		t.Fatalf("validation result=true or focus=%d, want %d", page.FocusTarget, first.ID)
	}
	firstStyle, _ := page.ComputedStyles.For(first)
	if firstStyle.Color != 0xff0000ff {
		t.Fatalf("invalid color = %#x, want red", firstStyle.Color)
	}
	forms.SetCurrentValue(first, "ok")
	forms.SetCurrentValue(second, "user@example.com")
	recomputePageStyles(page, time.Now())
	if !browser.ValidateForm(form.ID) {
		t.Fatal("valid form was rejected")
	}
	firstStyle, _ = page.ComputedStyles.For(first)
	if firstStyle.Color != 0x008000ff {
		t.Fatalf("valid color = %#x, want green", firstStyle.Color)
	}
}

func TestSubmitGETNavigatesWithEncodedEntriesAndPushesHistory(t *testing.T) {
	document := dom.NewDocument()
	form := document.CreateElement("form", map[string]string{"action": "/search?old=1#fragment", "method": "get"})
	query := document.CreateElement("input", map[string]string{"name": "q", "value": "hello world"})
	tag := document.CreateElement("input", map[string]string{"name": "tag", "value": "go"})
	submit := document.CreateElement("button", nil)
	for _, edge := range [][2]*dom.Node{{document.Root, form}, {form, query}, {form, tag}, {form, submit}} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	baseURL := mustParseURL(t, "https://example.com/form")
	targetURL := mustParseURL(t, "https://example.com/search?q=hello+world&tag=go")
	loader := &routeLoader{responses: map[string]*network.Response{
		targetURL.String(): {URL: targetURL, StatusCode: 200, ContentType: "text/html", Body: []byte(`<!doctype html><title>Results</title>`)},
	}}
	browser := New(loader)
	browser.SetPage(&Page{URL: baseURL, Document: document, ComputedStyles: style.Compute(document, nil), Events: events.NewDispatcher()})

	page, err := browser.SubmitGET(context.Background(), form.ID, submit.ID)
	if err != nil {
		t.Fatal(err)
	}
	if page.URL.String() != targetURL.String() || !browser.CanBack() {
		t.Fatalf("page=%s canBack=%v", page.URL, browser.CanBack())
	}
	if got := loader.requested; !reflect.DeepEqual(got, []string{targetURL.String()}) {
		t.Fatalf("requested = %v", got)
	}
	if len(browser.history.entries) != 2 || browser.history.entries[0].String() != baseURL.String() {
		t.Fatalf("history = %#v", browser.history.entries)
	}
}

func TestSubmitPOSTSendsEncodedBodyAndNavigatesToResponse(t *testing.T) {
	document := dom.NewDocument()
	form := document.CreateElement("form", map[string]string{"action": "/save?mode=fast#fragment", "method": "post"})
	input := document.CreateElement("input", map[string]string{"name": "message", "value": "hello world"})
	for _, edge := range [][2]*dom.Node{{document.Root, form}, {form, input}} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	baseURL := mustParseURL(t, "https://example.com/form")
	targetURL := mustParseURL(t, "https://example.com/save?mode=fast")
	finalURL := mustParseURL(t, "https://example.com/saved")
	loader := &requestRouteLoader{routeLoader: routeLoader{responses: map[string]*network.Response{
		targetURL.String(): {URL: finalURL, StatusCode: 200, ContentType: "text/html", Body: []byte(`<!doctype html><title>Saved</title>`)},
	}}}
	browser := New(loader)
	browser.SetPage(&Page{URL: baseURL, Document: document, ComputedStyles: style.Compute(document, nil), Events: events.NewDispatcher()})

	page, err := browser.SubmitPOST(context.Background(), form.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if loader.request == nil || loader.request.Method != http.MethodPost || string(loader.request.Body) != "message=hello+world" ||
		loader.request.Header.Get("Content-Type") != forms.URLEncoded {
		t.Fatalf("request = %#v", loader.request)
	}
	if page.URL.String() != finalURL.String() || !browser.CanBack() {
		t.Fatalf("page=%s canBack=%v", page.URL, browser.CanBack())
	}
}

func TestSubmitHonorsValidationPreventDefaultAndNoValidate(t *testing.T) {
	newBrowser := func(t *testing.T, formAttributes, buttonAttributes map[string]string) (*Browser, *routeLoader, *Page, *dom.Node, *dom.Node) {
		t.Helper()
		document := dom.NewDocument()
		formAttributes["action"] = "/result"
		form := document.CreateElement("form", formAttributes)
		input := document.CreateElement("input", map[string]string{"name": "name", "required": ""})
		button := document.CreateElement("button", buttonAttributes)
		for _, edge := range [][2]*dom.Node{{document.Root, form}, {form, input}, {form, button}} {
			if err := document.AppendChild(edge[0], edge[1]); err != nil {
				t.Fatal(err)
			}
		}
		baseURL := mustParseURL(t, "https://example.com/form")
		targetURL := mustParseURL(t, "https://example.com/result?name=")
		loader := &routeLoader{responses: map[string]*network.Response{
			targetURL.String(): {URL: targetURL, StatusCode: 200, ContentType: "text/html", Body: []byte(`<!doctype html><title>Result</title>`)},
		}}
		page := &Page{URL: baseURL, Document: document, ComputedStyles: style.Compute(document, nil), Events: events.NewDispatcher()}
		browser := New(loader)
		browser.SetPage(page)
		return browser, loader, page, form, button
	}

	browser, loader, page, form, button := newBrowser(t, map[string]string{}, map[string]string{})
	if _, err := browser.Submit(context.Background(), form.ID, button.ID); !errors.Is(err, ErrFormValidation) || page.FocusTarget == 0 || len(loader.requested) != 0 {
		t.Fatalf("validation err=%v focus=%d requests=%v", err, page.FocusTarget, loader.requested)
	}

	browser, loader, page, form, button = newBrowser(t, map[string]string{"novalidate": ""}, map[string]string{})
	page.Events.AddEventListener(form.ID, events.Submit, func(event events.Event) { event.PreventDefault() })
	if _, err := browser.Submit(context.Background(), form.ID, button.ID); !errors.Is(err, ErrSubmissionPrevented) || len(loader.requested) != 0 {
		t.Fatalf("prevent err=%v requests=%v", err, loader.requested)
	}

	for name, attributes := range map[string]struct {
		form   map[string]string
		button map[string]string
	}{
		"novalidate":     {form: map[string]string{"novalidate": ""}, button: map[string]string{}},
		"formnovalidate": {form: map[string]string{}, button: map[string]string{"formnovalidate": ""}},
	} {
		t.Run(name, func(t *testing.T) {
			browser, loader, _, form, button := newBrowser(t, attributes.form, attributes.button)
			if _, err := browser.Submit(context.Background(), form.ID, button.ID); err != nil || len(loader.requested) != 1 {
				t.Fatalf("submit err=%v requests=%v", err, loader.requested)
			}
		})
	}
}

func TestNewPageCopiesURL(t *testing.T) {
	pageURL := mustParseURL(t, "https://example.com/original")
	page := NewPage(pageURL)
	pageURL.Path = "/changed"

	if got, want := page.URL.String(), "https://example.com/original"; got != want {
		t.Fatalf("page URL = %q, want %q", got, want)
	}
}

func TestPageLinkURLResolvesNearestAnchor(t *testing.T) {
	document := dom.NewDocument()
	anchor := document.CreateElement("a", map[string]string{"href": "../next?q=1"})
	span := document.CreateElement("span", nil)
	text := document.CreateText("Next")
	for _, edge := range [][2]*dom.Node{{document.Root, anchor}, {anchor, span}, {span, text}} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	page := &Page{URL: mustParseURL(t, "https://example.com/docs/current"), Document: document}

	resolved, ok := page.LinkURL(span.ID)
	if !ok || resolved.String() != "https://example.com/next?q=1" {
		t.Fatalf("LinkURL() = (%v, %v), want resolved relative URL", resolved, ok)
	}
}

func TestPageLinkURLRejectsUnsupportedScheme(t *testing.T) {
	document := dom.NewDocument()
	anchor := document.CreateElement("a", map[string]string{"href": "javascript:alert(1)"})
	if err := document.AppendChild(document.Root, anchor); err != nil {
		t.Fatal(err)
	}
	page := &Page{URL: mustParseURL(t, "https://example.com"), Document: document}
	if resolved, ok := page.LinkURL(anchor.ID); ok || resolved != nil {
		t.Fatalf("LinkURL() = (%v, %v), want unsupported scheme rejected", resolved, ok)
	}
}

func mustParseURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", rawURL, err)
	}
	return parsed
}
