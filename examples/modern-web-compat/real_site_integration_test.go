package main

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/javascript"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestRealSiteFixtureLoadsGeneratedCSSImagesAnimationAndHydration(t *testing.T) {
	requests := &fixtureRequestLog{}
	server := httptest.NewServer(requests.middleware(modernWebCompatibilityHandler()))
	defer server.Close()

	engine := browser.NewWithEngineFactory(network.NewClientWithLimits(server.Client(), 8<<20), func(selected runtimemodel.Engine) runtimemodel.Runtime {
		if selected == runtimemodel.EngineJavaScript {
			return javascript.New()
		}
		return nil
	})
	defer engine.Close()
	if _, err := engine.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	mutations := make(chan struct{}, 32)
	engine.SetOnMutation(func() {
		select {
		case mutations <- struct{}{}:
		default:
		}
	})

	page, err := engine.Navigate(context.Background(), server.URL+"/real-site/")
	if err != nil {
		t.Fatal(err)
	}
	engine.UpdateViewport(1024, 720)
	waitForFixtureText(t, engine, mutations, "real-site-hydration", "hydrated")
	page = engine.Page()
	if page.Engine != runtimemodel.EngineJavaScript || page.Compatibility != browser.CompatibilityProfileModernWeb {
		t.Fatalf("real-site compatibility profile = engine:%s profile:%s", page.Engine, page.Compatibility)
	}
	gridStyle, _ := page.ComputedStyles.For(fixtureNode(t, page, "real-site-grid"))
	cardStyle, _ := page.ComputedStyles.For(fixtureNode(t, page, "real-site-medium"))
	if gridStyle.Display != style.DisplayGrid || len(gridStyle.GridTemplateColumns) != 2 {
		t.Fatalf("generated Tailwind responsive grid = %#v", gridStyle)
	}
	if cardStyle.Padding.Top <= 0 || cardStyle.BorderRadius.TopLeft.X.Pixels <= 0 || cardStyle.BackgroundColor != 0xffffffff {
		t.Fatalf("generated Tailwind card tokens = %#v", cardStyle)
	}
	for _, id := range []string{"real-site-medium", "real-site-high-card"} {
		image := fixtureNode(t, page, id).Children[1]
		resource := page.ImageResources[image.ID]
		if !resource.Loaded || resource.Error != "" {
			t.Fatalf("%s image resource = %+v", id, resource)
		}
	}
	if requests.count("/assets/pixel.png") != 1 {
		t.Fatalf("duplicate image fetch count = %d, want 1", requests.count("/assets/pixel.png"))
	}
	if requests.count("/real-site/app.css") != 1 || requests.count("/real-site/app.mjs") != 1 {
		t.Fatalf("real-site artifact requests = %#v", requests.paths)
	}
	if !page.ActiveAnimations(time.Now()) {
		t.Fatal("real-site CSS animation was not registered")
	}
	if !engine.DispatchClick(fixtureNode(t, page, "real-site-high").ID, 0, 0) || fixtureNode(t, page, "real-site-state").TextContent() != "1件" {
		t.Fatal("real-site hydrated filter interaction failed")
	}
}

func TestRealSiteFixtureKeepsGoEngineOnSSRPath(t *testing.T) {
	requests := &fixtureRequestLog{}
	server := httptest.NewServer(requests.middleware(modernWebCompatibilityHandler()))
	defer server.Close()
	engine := browser.New(network.NewClientWithLimits(server.Client(), 8<<20))
	defer engine.Close()

	page, err := engine.Navigate(context.Background(), server.URL+"/real-site/")
	if err != nil {
		t.Fatal(err)
	}
	if page.Engine != runtimemodel.EngineGo || page.Compatibility != browser.CompatibilityProfileGo || len(page.Scripts) != 0 || page.RuntimeStarted {
		t.Fatalf("Go SSR path = engine:%s profile:%s scripts:%d runtime-started:%t", page.Engine, page.Compatibility, len(page.Scripts), page.RuntimeStarted)
	}
	if got := fixtureNode(t, page, "real-site-hydration").TextContent(); got != "SSR" {
		t.Fatalf("Go SSR marker = %q", got)
	}
	if requests.count("/real-site/app.mjs") != 0 {
		t.Fatalf("Go Engine requested JavaScript: %#v", requests.paths)
	}
}
