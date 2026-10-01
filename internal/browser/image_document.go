package browser

import (
	"context"
	"fmt"
	"image"
	"mime"
	"net/url"
	"path"
	"strings"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/devtools"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	layoutengine "github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/style"
)

type imageNavigation struct {
	page       *Page
	nodeID     dom.NodeID
	generation uint64
}

func imageMediaType(contentType string) (string, bool) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || !isImageContentType(mediaType) {
		return "", false
	}
	return mediaType, true
}

func buildImageDocument(target *url.URL) (*dom.Document, dom.NodeID) {
	document := dom.NewDocument()
	htmlNode := document.CreateElement("html", nil)
	head := document.CreateElement("head", nil)
	title := document.CreateElement("title", nil)
	body := document.CreateElement("body", map[string]string{"style": "margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;background:#202124"})
	name := "Image"
	if target != nil {
		if candidate := path.Base(target.Path); candidate != "" && candidate != "." && candidate != "/" {
			name = candidate
		}
	}
	imageNode := document.CreateElement("img", map[string]string{
		"src": target.String(), "alt": name,
		"style": "display:block;max-width:100%;max-height:100vh;object-fit:contain",
	})
	_ = document.AppendChild(document.Root, htmlNode)
	_ = document.AppendChild(htmlNode, head)
	_ = document.AppendChild(head, title)
	_ = document.AppendChild(title, document.CreateText(name))
	_ = document.AppendChild(htmlNode, body)
	_ = document.AppendChild(body, imageNode)
	document.SetReadyState("interactive")
	return document, imageNode.ID
}

func (b *Browser) commitImageResponseHead(ctx context.Context, head network.ResponseHead, commit historyCommit, historyIndex int, navigationID uint64, engine runtimemodel.Engine, reducedMotion bool, pageStore *devtools.PageStore) (*imageNavigation, error) {
	if _, ok := imageMediaType(head.ContentType); !ok || head.URL == nil {
		return nil, nil
	}
	document, nodeID := buildImageDocument(head.URL)
	stylesheet := &css.Stylesheet{}
	computed, styleErrors := computeStableStylesWithDiagnostics(document, stylesheet, style.InteractionState{}, 1280, 720, reducedMotion, engine == runtimemodel.EngineJavaScript)
	cache := newImageResourceCache()
	resource := layoutengine.ImageResource{URL: head.URL.String(), Alt: path.Base(head.URL.Path), Deferred: true}
	page := &Page{
		URL: cloneURL(head.URL), BaseURL: cloneURL(head.URL), StatusCode: head.StatusCode, ContentType: head.ContentType,
		Document: document, Events: events.NewDispatcher(), Stylesheet: stylesheet, ComputedStyles: computed, StyleErrors: styleErrors,
		Animations: style.NewAnimationRegistry(), Transitions: style.NewTransitionRegistry(), StyleRevision: 1,
		ReducedMotion: reducedMotion, ViewportWidth: 1280, ViewportHeight: 720,
		BackgroundImages: make(map[string]image.Image), ImageResources: map[dom.NodeID]layoutengine.ImageResource{nodeID: resource},
		Images: make(map[string]image.Image), AnimatedImages: make(map[dom.NodeID]*animatedImagePlayer), WebFonts: layoutPageFonts(nil, engine == runtimemodel.EngineJavaScript),
		Engine: engine, Compatibility: compatibilityProfileForEngine(engine), DevTools: pageStore, serviceWorkers: b.serviceWorkers, imageCache: cache,
	}
	loadContext, generation := page.beginImageLoad(context.Background())
	_ = loadContext

	b.mu.Lock()
	if navigationID != b.navigationID {
		b.mu.Unlock()
		page.closeDevTools()
		page.releaseImageResources()
		return nil, context.Canceled
	}
	previousRuntime := b.activeRuntime
	previousPage := b.page
	b.nextPageID++
	page.HistoryID = b.nextPageID
	page.ScrollRevision = 1
	b.activeRuntime = nil
	b.page = page
	var record *NavigationRecord
	observer := b.onNavigation
	switch commit {
	case historyPush:
		b.history.pushEntry(&historyEntry{URL: page.URL, PageID: page.HistoryID})
		if observer != nil {
			value := NavigationRecord{URL: page.URL.String(), Title: document.Title(), Typed: ctx.Value(typedNavigationKey) == true}
			record = &value
		}
	case historyTraverse:
		previousEntry := cloneHistoryEntry(b.history.entries[historyIndex])
		b.history.index = historyIndex
		if previousEntry != nil {
			page.HistoryState, page.ScrollFirst, page.ScrollOffset = previousEntry.State, previousEntry.ScrollFirst, previousEntry.ScrollOffset
			b.history.rebindPage(previousEntry.PageID, page.HistoryID)
			b.history.replaceEntry(&historyEntry{URL: page.URL, State: previousEntry.State, PageID: page.HistoryID, ScrollFirst: previousEntry.ScrollFirst, ScrollOffset: previousEntry.ScrollOffset})
		} else {
			b.history.replaceEntry(&historyEntry{URL: page.URL, PageID: page.HistoryID})
		}
	case historyReplace:
		b.history.index = historyIndex
		state := ""
		if current, ok := b.history.current(); ok && current != nil {
			state = current.State
		}
		b.history.replaceEntry(&historyEntry{URL: page.URL, State: state, PageID: page.HistoryID})
	}
	onMutation := b.onMutation
	b.mu.Unlock()

	if record != nil {
		observer(*record)
	}
	if previousRuntime != nil {
		_ = previousRuntime.Stop()
	}
	if previousPage != nil && previousPage != page {
		if previousPage.Animations != nil {
			previousPage.Animations.Clear()
		}
		if previousPage.Transitions != nil {
			previousPage.Transitions.Clear()
		}
		previousPage.closeDevTools()
		_ = closePageFrames(previousPage)
		previousPage.releaseImageResources()
	}
	if onMutation != nil {
		onMutation()
	}
	return &imageNavigation{page: page, nodeID: nodeID, generation: generation}, nil
}

func (b *Browser) finishImageNavigation(navigation *imageNavigation, response *network.Response, navigationID uint64, onMutation func()) (*Page, error) {
	if navigation == nil || navigation.page == nil || response == nil {
		return nil, fmt.Errorf("image navigation is unavailable")
	}
	page := navigation.page
	decoded, width, height, err := decodeImageResponseWithBudget(response.Body, response.ContentType, newImageDecodeBudget())
	resource := layoutengine.ImageResource{URL: response.URL.String(), Alt: path.Base(response.URL.Path)}
	failure := ""
	if err != nil {
		resource.Error = "image decode failed"
		failure = resource.Error + ": " + network.RedactedDiagnosticURL(response.URL)
	} else {
		resource.Loaded = true
		resource.IntrinsicWidth, resource.IntrinsicHeight = float32(width), float32(height)
	}
	b.mu.RLock()
	active := navigationID == b.navigationID && b.page == page
	b.mu.RUnlock()
	if !active {
		return nil, context.Canceled
	}
	page.Source = append([]byte(nil), response.Body...)
	page.StatusCode, page.ContentType = response.StatusCode, response.ContentType
	page.commitImageResourceLoad(navigation.generation, navigation.nodeID, resource, decoded, failure)
	page.Document.SetReadyState("complete")
	if onMutation != nil {
		onMutation()
	}
	return page, nil
}

func finishImageNavigationError(navigation *imageNavigation, err error, onMutation func()) *Page {
	if navigation == nil || navigation.page == nil {
		return nil
	}
	page := navigation.page
	message := strings.TrimSpace(err.Error())
	if message != "" {
		page.ImageErrors = boundedImageDiagnostics(append(page.ImageErrors, message))
	}
	page.Document.SetReadyState("complete")
	if onMutation != nil {
		onMutation()
	}
	return page
}
