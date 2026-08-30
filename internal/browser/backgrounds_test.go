package browser

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"net/url"
	"sync"
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	runtimejavascript "github.com/Grove-Computing/Growse/internal/runtime/javascript"
	"github.com/Grove-Computing/Growse/internal/style"
	"github.com/gen2brain/avif"
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

func TestLoadBackgroundImagesDecodesBoundedDataImageWithoutNetwork(t *testing.T) {
	var encoded bytes.Buffer
	source := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	source.SetNRGBA(1, 1, color.NRGBA{B: 255, A: 255})
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	resource := "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())
	computed := style.Map{dom.NodeID(1): {BackgroundImage: style.BackgroundImage{Kind: style.BackgroundImageURL, URL: resource}}}
	images, failures := loadBackgroundImages(context.Background(), nil, computed)
	if len(failures) != 0 || images[resource] == nil || images[resource].Bounds() != image.Rect(0, 0, 2, 2) {
		t.Fatalf("data background images/failures = %#v / %#v", images, failures)
	}
}

func TestPageImageCacheSharesFetchBodyAndDecodeAcrossBackgroundAndElement(t *testing.T) {
	resourceURL := "https://example.com/assets/shared.png"
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	loader := &routeLoader{responses: map[string]*network.Response{
		resourceURL: {URL: mustParseURL(t, resourceURL), ContentType: "image/png", Body: encoded.Bytes()},
	}}
	cache, budget := newImageResourceCache(), newImageDecodeBudget()
	computed := style.Map{dom.NodeID(1): {BackgroundImage: style.BackgroundImage{Kind: style.BackgroundImageURL, URL: resourceURL}}}
	backgrounds, backgroundErrors := loadBackgroundImagesWithCache(context.Background(), loader, computed, budget, cache)
	document := dom.NewDocument()
	imageNode := document.CreateElement("img", map[string]string{"src": resourceURL})
	if err := document.AppendChild(document.Root, imageNode); err != nil {
		t.Fatal(err)
	}
	resources, images, imageErrors := loadReplacedImagesWithCache(context.Background(), loader, mustParseURL(t, "https://example.com/"), document, 1280, 1, nil, budget, cache)
	if len(backgroundErrors) != 0 || len(imageErrors) != 0 || !resources[imageNode.ID].Loaded {
		t.Fatalf("shared load errors = background:%v image:%v resource:%#v", backgroundErrors, imageErrors, resources[imageNode.ID])
	}
	if len(loader.requested) != 1 {
		t.Fatalf("shared resource fetches = %d, want 1: %v", len(loader.requested), loader.requested)
	}
	if backgrounds[resourceURL] != images[resourceURL] {
		t.Fatal("background and replaced image did not share the decoded result")
	}
	entry := cache.entries[resourceURL]
	if entry == nil || !bytes.Equal(entry.body, encoded.Bytes()) || entry.decoded != images[resourceURL] {
		t.Fatalf("cached body/decode = %#v", entry)
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

func TestLoadReplacedImagesResolvesAndDecodesWebP(t *testing.T) {
	pageURL := mustParseURL(t, "https://example.com/app/")
	imageURL := "https://example.com/app/avatar.webp"
	encoded, err := base64.StdEncoding.DecodeString("UklGRrIBAABXRUJQVlA4TKUBAAAvSsAYAA8w//M///MfeJAkbXvaSG7m8Q3GfYSBJekwQztm/IcZlgwnmWImn2BK7aFmBtnVir6q//8VOkFE/xm4baTIu8c48ArEo6+B3zFKYln3pqClSCKX0begFTAXFOLXHSyF8cCNcZEG4OywuA4KVVfJCiArU7GAgJI8+lJP/OKMT/fBAjevg1cYB7YVkFuWga2lyPi5I0HFy5YTpWIHg0RZpkniRVW9odHAKOwosWuOGdxIyn2OvaCDvhg/we6TwadPBPbqBV58MsLmMJ8yZnOWk8SRz4N+QoyPL+MnamzMvcE1rHNEr91F9GKZPVUcS9w7PhhH36suB9qPeYb/oLk6cuTiJ0wOK3m5h1cKjW6EVZCYMK7dxcKCBdgP9HkKr9gkAO2P8GKZGWVdIAatQa+1IDpt6qyorVwdy01xdW8Jkfk6xjEXmVQQ+HQdFr6OKhIN34dXWq0+0qr6EJSCeeVLH9+gvGTLyqM65PQ44ihzlTXxQKjKbAvshXgir7Lil9w4L2bvMycmjQcqXaMCO6BlY28i+FOLzbfI1vEqxAhotocAAA==")
	if err != nil {
		t.Fatal(err)
	}
	document := dom.NewDocument()
	imageNode := document.CreateElement("img", map[string]string{"src": "avatar.webp", "alt": "Avatar"})
	if err := document.AppendChild(document.Root, imageNode); err != nil {
		t.Fatal(err)
	}
	loader := &routeLoader{responses: map[string]*network.Response{
		imageURL: {URL: mustParseURL(t, imageURL), StatusCode: 200, ContentType: "image/webp", Body: encoded},
	}}
	resources, images, failures := loadReplacedImages(context.Background(), loader, pageURL, document, 1280, 1)
	resource := resources[imageNode.ID]
	if len(failures) != 0 || !resource.Loaded || resource.URL != imageURL || resource.IntrinsicWidth <= 0 || resource.IntrinsicHeight <= 0 || resource.Alt != "Avatar" || images[imageURL] == nil {
		t.Fatalf("resource/images/failures = %#v / %#v / %#v", resource, images, failures)
	}
}

func TestAVIFPictureCandidateAndBackgroundDecodeWithinSafetyLimits(t *testing.T) {
	var encoded bytes.Buffer
	source := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	source.SetNRGBA(1, 1, color.NRGBA{R: 32, G: 96, B: 224, A: 255})
	if err := avif.Encode(&encoded, source, avif.Options{Quality: 80, Speed: 10}); err != nil {
		t.Fatal(err)
	}
	rawURL := "https://example.com/hero.avif"
	loader := &routeLoader{responses: map[string]*network.Response{
		rawURL: {URL: mustParseURL(t, rawURL), ContentType: "image/avif", Body: encoded.Bytes()},
	}}
	document := dom.NewDocument()
	picture := document.CreateElement("picture", nil)
	avifSource := document.CreateElement("source", map[string]string{"type": "image/avif", "srcset": "hero.avif 1x"})
	imageNode := document.CreateElement("img", map[string]string{"src": "fallback.png"})
	for _, edge := range [][2]*dom.Node{{document.Root, picture}, {picture, avifSource}, {picture, imageNode}} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	resources, images, failures := loadReplacedImages(context.Background(), loader, mustParseURL(t, "https://example.com/"), document, 800, 1)
	if len(failures) != 0 || !resources[imageNode.ID].Loaded || resources[imageNode.ID].URL != rawURL || images[rawURL] == nil {
		t.Fatalf("AVIF picture resource/images/failures = %#v / %#v / %#v", resources[imageNode.ID], images, failures)
	}
	computed := style.Map{dom.NodeID(99): {BackgroundImage: style.BackgroundImage{Kind: style.BackgroundImageURL, URL: rawURL}}}
	backgrounds, backgroundFailures := loadBackgroundImages(context.Background(), loader, computed)
	if len(backgroundFailures) != 0 || backgrounds[rawURL] == nil || backgrounds[rawURL].Bounds().Dx() != 3 {
		t.Fatalf("AVIF background/failures = %#v / %#v", backgrounds, backgroundFailures)
	}
}

func TestLoadReplacedImagesLocalizesDecodeFailure(t *testing.T) {
	baseURL := mustParseURL(t, "https://example.com/")
	document := dom.NewDocument()
	broken := document.CreateElement("img", map[string]string{"src": "broken.png", "alt": "Fallback"})
	if err := document.AppendChild(document.Root, broken); err != nil {
		t.Fatal(err)
	}
	loader := &routeLoader{responses: map[string]*network.Response{
		"https://example.com/broken.png": {URL: mustParseURL(t, "https://example.com/broken.png"), ContentType: "image/png", Body: []byte("not an image")},
	}}
	resources, images, failures := loadReplacedImages(context.Background(), loader, baseURL, document, 1280, 1)
	if resources[broken.ID].Loaded || resources[broken.ID].Alt != "Fallback" || len(images) != 0 || len(failures) != 1 {
		t.Fatalf("resource/images/failures = %#v / %#v / %#v", resources[broken.ID], images, failures)
	}
}

func TestNavigateLoadsReplacedImagesOnlyForExplicitJavaScriptEngine(t *testing.T) {
	pageURL := "https://example.com/index.html"
	imageURL := "https://example.com/photo.png"
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	newLoader := func() *routeLoader {
		return &routeLoader{responses: map[string]*network.Response{
			pageURL:  {URL: mustParseURL(t, pageURL), StatusCode: 200, ContentType: "text/html", Body: []byte(`<img src="photo.png" alt="Photo">`)},
			imageURL: {URL: mustParseURL(t, imageURL), StatusCode: 200, ContentType: "image/png", Body: encoded.Bytes()},
		}}
	}
	factory := func(runtimemodel.Engine) runtimemodel.Runtime { return &runtimeStub{} }
	goBrowser := NewWithEngineFactory(newLoader(), factory)
	defer goBrowser.Close()
	goPage, err := goBrowser.Navigate(context.Background(), pageURL)
	if err != nil {
		t.Fatal(err)
	}
	if goPage.ImageResources != nil || goPage.Images != nil {
		t.Fatalf("Go page loaded modern images: %#v / %#v", goPage.ImageResources, goPage.Images)
	}
	jsBrowser := NewWithEngineFactory(newLoader(), factory)
	defer jsBrowser.Close()
	if _, err := jsBrowser.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	jsPage, err := jsBrowser.Navigate(context.Background(), pageURL)
	if err != nil {
		t.Fatal(err)
	}
	if len(jsPage.ImageResources) != 1 || jsPage.Images[imageURL] == nil {
		t.Fatalf("JavaScript page images = %#v / %#v", jsPage.ImageResources, jsPage.Images)
	}
}

func TestImageCandidatesSelectPictureSourceByTypeMediaSizesAndScale(t *testing.T) {
	document := dom.NewDocument()
	picture := document.CreateElement("picture", nil)
	avifSource := document.CreateElement("source", map[string]string{"type": "image/avif", "srcset": "hero.avif 1x"})
	wide := document.CreateElement("source", map[string]string{"type": "image/webp", "media": "(min-width: 900px)", "srcset": "wide.webp 1x"})
	matched := document.CreateElement("source", map[string]string{
		"type": "image/png", "media": "(max-width: 899px)",
		"srcset": "small.png 400w, large.png 800w", "sizes": "(max-width: 600px) 100vw, 50vw",
	})
	imageNode := document.CreateElement("img", map[string]string{"src": "fallback.jpg"})
	appendNodes := [][2]*dom.Node{{document.Root, picture}, {picture, avifSource}, {picture, wide}, {picture, matched}, {picture, imageNode}}
	for _, pair := range appendNodes {
		if err := document.AppendChild(pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
	}
	candidates := imageCandidates(imageNode, mustParseURL(t, "https://example.com/assets/"), 800, 1.5)
	want := []string{"https://example.com/assets/hero.avif", "https://example.com/assets/large.png", "https://example.com/assets/small.png", "https://example.com/assets/fallback.jpg"}
	if len(candidates) != len(want) {
		t.Fatalf("candidates = %#v, want %#v", candidates, want)
	}
	for index := range want {
		if candidates[index].String() != want[index] {
			t.Fatalf("candidate[%d] = %q, want %q", index, candidates[index], want[index])
		}
	}
}

func TestLoadReplacedImagesFallsBackAfterPreferredCandidateFailure(t *testing.T) {
	baseURL := mustParseURL(t, "https://example.com/")
	document := dom.NewDocument()
	imageNode := document.CreateElement("img", map[string]string{"srcset": "broken.webp 2x, fallback.png 1x", "src": "last.jpg", "alt": "Hero"})
	if err := document.AppendChild(document.Root, imageNode); err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 4, 2))); err != nil {
		t.Fatal(err)
	}
	loader := &routeLoader{responses: map[string]*network.Response{
		"https://example.com/broken.webp":  {URL: mustParseURL(t, "https://example.com/broken.webp"), ContentType: "image/webp", Body: []byte("broken")},
		"https://example.com/fallback.png": {URL: mustParseURL(t, "https://example.com/fallback.png"), ContentType: "image/png", Body: encoded.Bytes()},
	}}
	resources, images, failures := loadReplacedImages(context.Background(), loader, baseURL, document, 800, 2)
	resource := resources[imageNode.ID]
	if len(failures) != 0 || !resource.Loaded || resource.URL != "https://example.com/fallback.png" || resource.IntrinsicWidth != 4 || images[resource.URL] == nil {
		t.Fatalf("resource/images/failures = %#v / %#v / %#v", resource, images, failures)
	}
}

func TestUpdateViewportReselectsResponsiveImageCandidate(t *testing.T) {
	pageURL := "https://example.com/index.html"
	desktopURL := "https://example.com/desktop.png"
	mobileURL := "https://example.com/mobile.png"
	encode := func(width int) []byte {
		var output bytes.Buffer
		if err := png.Encode(&output, image.NewNRGBA(image.Rect(0, 0, width, 1))); err != nil {
			t.Fatal(err)
		}
		return output.Bytes()
	}
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL:    {URL: mustParseURL(t, pageURL), StatusCode: 200, ContentType: "text/html", Body: []byte(`<picture><source media="(max-width: 600px)" srcset="mobile.png"><img src="desktop.png"></picture>`)},
		desktopURL: {URL: mustParseURL(t, desktopURL), StatusCode: 200, ContentType: "image/png", Body: encode(4)},
		mobileURL:  {URL: mustParseURL(t, mobileURL), StatusCode: 200, ContentType: "image/png", Body: encode(2)},
	}}
	browserState := NewWithEngineFactory(loader, func(runtimemodel.Engine) runtimemodel.Runtime { return &runtimeStub{} })
	defer browserState.Close()
	if _, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	page, err := browserState.Navigate(context.Background(), pageURL)
	if err != nil {
		t.Fatal(err)
	}
	var imageID dom.NodeID
	for id := range page.ImageResources {
		imageID = id
	}
	if page.ImageResources[imageID].URL != desktopURL {
		t.Fatalf("initial candidate = %q, want desktop", page.ImageResources[imageID].URL)
	}
	if !browserState.UpdateViewport(500, 700) || page.ImageResources[imageID].URL != mobileURL || page.ImageResources[imageID].IntrinsicWidth != 2 {
		t.Fatalf("responsive candidate = %#v", page.ImageResources[imageID])
	}
}

func TestJavaScriptImageLifecycleDispatchesInDocumentOrderAndExposesState(t *testing.T) {
	pageURL := "https://example.com/index.html"
	imageURL := "https://example.com/ok.png"
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL: {URL: mustParseURL(t, pageURL), StatusCode: 200, ContentType: "text/html", Body: []byte(`<body id="result"><img id="ok" src="ok.png"><img id="bad" src="bad.png"><script>
const result = document.getElementById("result");
const ok = document.getElementById("ok");
const bad = document.getElementById("bad");
result.setAttribute("state", [ok.complete, ok.naturalWidth, ok.naturalHeight, ok.currentSrc].join("|"));
ok.addEventListener("load", () => result.setAttribute("events", (result.getAttribute("events") || "") + "load,"));
bad.addEventListener("error", () => result.setAttribute("events", (result.getAttribute("events") || "") + "error,"));
</script></body>`)},
		imageURL:                      {URL: mustParseURL(t, imageURL), StatusCode: 200, ContentType: "image/png", Body: encoded.Bytes()},
		"https://example.com/bad.png": {URL: mustParseURL(t, "https://example.com/bad.png"), StatusCode: 200, ContentType: "image/png", Body: []byte("broken")},
	}}
	browserState := NewWithEngineFactory(loader, func(engine runtimemodel.Engine) runtimemodel.Runtime {
		if engine == runtimemodel.EngineJavaScript {
			return runtimejavascript.New()
		}
		return &runtimeStub{}
	})
	defer browserState.Close()
	if _, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	page, err := browserState.Navigate(context.Background(), pageURL)
	if err != nil {
		t.Fatal(err)
	}
	result, ok := page.Document.GetElementByID("result")
	if !ok {
		t.Fatal("result element is missing")
	}
	if state, _ := result.Attribute("state"); state != "true|3|2|"+imageURL {
		t.Fatalf("HTMLImageElement state = %q", state)
	}
	if order, _ := result.Attribute("events"); order != "load,error," {
		t.Fatalf("image event order = %q", order)
	}
}

func TestDynamicImageSourceRelayoutsAndDispatchesOnce(t *testing.T) {
	pageURL := "https://example.com/dynamic.html"
	firstURL, secondURL := "https://example.com/first.png", "https://example.com/second.png"
	encode := func(width int) []byte {
		var output bytes.Buffer
		if err := png.Encode(&output, image.NewNRGBA(image.Rect(0, 0, width, 1))); err != nil {
			t.Fatal(err)
		}
		return output.Bytes()
	}
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL: {URL: mustParseURL(t, pageURL), StatusCode: 200, ContentType: "text/html", Body: []byte(`<body id="result"><img id="hero" src="first.png"><script>
const result = document.getElementById("result"); const hero = document.getElementById("hero");
hero.addEventListener("load", () => result.setAttribute("loads", String(Number(result.getAttribute("loads") || "0") + 1)));
hero.src = "second.png"; result.setAttribute("width", String(hero.naturalWidth));
</script></body>`)},
		firstURL:  {URL: mustParseURL(t, firstURL), ContentType: "image/png", Body: encode(2)},
		secondURL: {URL: mustParseURL(t, secondURL), ContentType: "image/png", Body: encode(5)},
	}}
	browserState := NewWithEngineFactory(loader, func(engine runtimemodel.Engine) runtimemodel.Runtime {
		if engine == runtimemodel.EngineJavaScript {
			return runtimejavascript.New()
		}
		return &runtimeStub{}
	})
	defer browserState.Close()
	_, _ = browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript)
	page, err := browserState.Navigate(context.Background(), pageURL)
	if err != nil {
		t.Fatal(err)
	}
	result, _ := page.Document.GetElementByID("result")
	if loads, _ := result.Attribute("loads"); loads != "1" {
		t.Fatalf("dynamic image loads = %q, want once", loads)
	}
	if width, _ := result.Attribute("width"); width != "5" {
		t.Fatalf("late naturalWidth = %q, want 5", width)
	}
	if page.StyleRevision < 2 {
		t.Fatalf("StyleRevision = %d, want late relayout revision", page.StyleRevision)
	}
}

func TestTargetedImageMutationDoesNotLoadSiblingResources(t *testing.T) {
	baseURL := mustParseURL(t, "https://example.com/")
	targetURL := "https://example.com/target.png"
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 5, 3))); err != nil {
		t.Fatal(err)
	}
	loader := &routeLoader{responses: map[string]*network.Response{
		targetURL: {URL: mustParseURL(t, targetURL), ContentType: "image/png", Body: encoded.Bytes()},
	}}
	document := dom.NewDocument()
	sibling := document.CreateElement("img", map[string]string{"src": "sibling.png"})
	target := document.CreateElement("img", map[string]string{"src": "target.png"})
	if err := document.AppendChild(document.Root, sibling); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(document.Root, target); err != nil {
		t.Fatal(err)
	}
	cache := newImageResourceCache()
	resource, decoded, failure := loadReplacedImageNodeWithCache(context.Background(), loader, baseURL, target, 1280, 1, true, newImageDecodeBudget(), cache)
	if failure != "" || !resource.Loaded || decoded == nil || resource.URL != targetURL {
		t.Fatalf("targeted resource = %#v decoded:%v failure:%q", resource, decoded != nil, failure)
	}
	if len(loader.requested) != 1 || loader.requested[0] != targetURL || cache.entries["https://example.com/sibling.png"] != nil {
		t.Fatalf("targeted requests/cache = %v / %#v", loader.requested, cache.entries)
	}
}

func TestLazyImagePolicyDefersOutsideViewport(t *testing.T) {
	baseURL := mustParseURL(t, "https://example.com/")
	document := dom.NewDocument()
	imageNode := document.CreateElement("img", map[string]string{"src": "lazy.png", "loading": "lazy"})
	if err := document.AppendChild(document.Root, imageNode); err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 2, 1))); err != nil {
		t.Fatal(err)
	}
	loader := &routeLoader{responses: map[string]*network.Response{
		"https://example.com/lazy.png": {URL: mustParseURL(t, "https://example.com/lazy.png"), ContentType: "image/png", Body: encoded.Bytes()},
	}}
	resources, _, failures := loadReplacedImagesWithPolicy(context.Background(), loader, baseURL, document, 800, 1, map[dom.NodeID]bool{imageNode.ID: false})
	if !resources[imageNode.ID].Deferred || len(loader.requested) != 0 || len(failures) != 0 {
		t.Fatalf("deferred resource/requests/failures = %#v / %#v / %#v", resources[imageNode.ID], loader.requested, failures)
	}
	resources, _, failures = loadReplacedImagesWithPolicy(context.Background(), loader, baseURL, document, 800, 1, map[dom.NodeID]bool{imageNode.ID: true})
	if !resources[imageNode.ID].Loaded || len(loader.requested) != 1 || len(failures) != 0 {
		t.Fatalf("eligible resource/requests/failures = %#v / %#v / %#v", resources[imageNode.ID], loader.requested, failures)
	}
}

type cancelAwareImageLoader struct {
	responses map[string]*network.Response
	blocked   string
	started   chan struct{}
	once      sync.Once
}

func (loader *cancelAwareImageLoader) Get(ctx context.Context, target *url.URL) (*network.Response, error) {
	if target.String() == loader.blocked {
		loader.once.Do(func() { close(loader.started) })
		<-ctx.Done()
		return nil, ctx.Err()
	}
	response := loader.responses[target.String()]
	if response == nil {
		return nil, context.Canceled
	}
	copy := *response
	copy.Body = append([]byte(nil), response.Body...)
	return &copy, nil
}

func TestNavigationCancelsStaleResponsiveImageCompletion(t *testing.T) {
	pageURL, nextURL := "https://example.com/page.html", "https://example.com/next.html"
	desktopURL, mobileURL := "https://example.com/desktop.png", "https://example.com/mobile.png"
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 2, 1))); err != nil {
		t.Fatal(err)
	}
	loader := &cancelAwareImageLoader{
		blocked: mobileURL, started: make(chan struct{}), responses: map[string]*network.Response{
			pageURL:    {URL: mustParseURL(t, pageURL), StatusCode: 200, ContentType: "text/html", Body: []byte(`<picture><source media="(max-width:600px)" srcset="mobile.png"><img src="desktop.png"></picture>`)},
			nextURL:    {URL: mustParseURL(t, nextURL), StatusCode: 200, ContentType: "text/html", Body: []byte(`<p>next</p>`)},
			desktopURL: {URL: mustParseURL(t, desktopURL), ContentType: "image/png", Body: encoded.Bytes()},
		},
	}
	browserState := NewWithEngineFactory(loader, func(runtimemodel.Engine) runtimemodel.Runtime { return &runtimeStub{} })
	defer browserState.Close()
	_, _ = browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript)
	oldPage, err := browserState.Navigate(context.Background(), pageURL)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		browserState.UpdateViewport(500, 700)
		close(done)
	}()
	<-loader.started
	if _, err := browserState.Navigate(context.Background(), nextURL); err != nil {
		t.Fatal(err)
	}
	<-done
	for _, resource := range oldPage.ImageResources {
		if resource.URL == mobileURL {
			t.Fatalf("stale image completion committed after navigation: %#v", resource)
		}
	}
}

func TestPageCloseCancelsPendingResponsiveImage(t *testing.T) {
	pageURL := "https://example.com/page.html"
	desktopURL, mobileURL := "https://example.com/desktop.png", "https://example.com/mobile.png"
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 2, 1))); err != nil {
		t.Fatal(err)
	}
	loader := &cancelAwareImageLoader{
		blocked: mobileURL, started: make(chan struct{}), responses: map[string]*network.Response{
			pageURL:    {URL: mustParseURL(t, pageURL), StatusCode: 200, ContentType: "text/html", Body: []byte(`<picture><source media="(max-width:600px)" srcset="mobile.png"><img src="desktop.png"></picture>`)},
			desktopURL: {URL: mustParseURL(t, desktopURL), ContentType: "image/png", Body: encoded.Bytes()},
		},
	}
	browserState := NewWithEngineFactory(loader, func(runtimemodel.Engine) runtimemodel.Runtime { return &runtimeStub{} })
	_, _ = browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript)
	if _, err := browserState.Navigate(context.Background(), pageURL); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		browserState.UpdateViewport(500, 700)
		close(done)
	}()
	<-loader.started
	if err := browserState.Close(); err != nil {
		t.Fatal(err)
	}
	<-done
	if browserState.Page() != nil {
		t.Fatal("closed Page retained after pending image cancellation")
	}
}

func TestNavigateResolvesAndLoadsBackgroundImageFromExternalStylesheet(t *testing.T) {
	pageURL := "https://example.com/index.html"
	stylesheetURL := "https://example.com/styles/main.css"
	imageURL := "https://example.com/images/card.png"
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL:       {URL: mustParseURL(t, pageURL), StatusCode: 200, ContentType: "text/html", Body: []byte(`<link rel="stylesheet" href="styles/main.css"><div class="card">card</div>`)},
		stylesheetURL: {URL: mustParseURL(t, stylesheetURL), StatusCode: 200, ContentType: "text/css", Body: []byte(`.card { background-image: url('../images/card.png'); }`)},
		imageURL:      {URL: mustParseURL(t, imageURL), StatusCode: 200, ContentType: "image/png", Body: encoded.Bytes()},
	}}
	page, err := New(loader).Navigate(context.Background(), pageURL)
	if err != nil {
		t.Fatal(err)
	}
	if page.BackgroundImages[imageURL] == nil || len(page.BackgroundErrors) != 0 {
		t.Fatalf("background images/errors = %#v / %#v", page.BackgroundImages, page.BackgroundErrors)
	}
}
