package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/javascript"
	"github.com/Grove-Computing/Growse/internal/style"
)

type fixtureVisualClock struct{ current time.Time }

func (clock *fixtureVisualClock) Now() time.Time { return clock.current }

func TestRealSiteVisualRegression(t *testing.T) {
	server := httptest.NewServer(modernWebCompatibilityHandler())
	defer server.Close()
	engine := browser.NewWithEngineFactory(network.NewClientWithLimits(server.Client(), 8<<20), func(selected runtimemodel.Engine) runtimemodel.Runtime {
		if selected == runtimemodel.EngineJavaScript {
			return javascript.New()
		}
		return nil
	})
	defer engine.Close()
	clock := &fixtureVisualClock{current: time.Date(2026, time.August, 29, 12, 0, 0, 0, time.UTC)}
	engine.SetAnimationClock(clock)
	mutations := make(chan struct{}, 64)
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
	actual := frameworkVisualGolden{Environment: "viewport=1024x720/640x720 scale=1 font=goregular clock=2026-08-29T12:00:00Z"}
	actual.States = append(actual.States, visualFixtureState("real-initial-ssr", page, "real-site-root", "real-site-hydration", "real-site-state"))
	actual.States = append(actual.States, visualFixtureState("real-japanese", page, "real-site-root", "real-site-medium", "real-site-high-card"))
	engine.UpdateViewport(640, 720)
	actual.States = append(actual.States, visualFixtureState("real-responsive-narrow", page, "real-site-root", "real-site-grid"))

	engine.UpdateViewport(1024, 720)
	if _, err := engine.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	engine.UpdateViewport(1024, 720)
	waitForFixtureText(t, engine, mutations, "real-site-hydration", "hydrated")
	page = engine.Page()
	actual.States = append(actual.States, visualFixtureState("real-image-loaded", page, "real-site-root", "real-site-medium", "real-site-high-card"))
	if !engine.DispatchClick(fixtureNode(t, page, "real-site-high").ID, 0, 0) {
		t.Fatal("real-site hydration interaction was not handled")
	}
	actual.States = append(actual.States, visualFixtureState("real-hydrated-interaction", page, "real-site-root", "real-site-hydration", "real-site-state"))

	clock.current = clock.current.Add(450 * time.Millisecond)
	styles, damage := page.AnimationFrame(clock.current)
	coin := fixtureNode(t, page, "real-site-coin")
	coinStyle := styles[coin.ID]
	transform := style.ResolveTransform(coinStyle.Transform, 32, 32)
	actual.States = append(actual.States, fmt.Sprintf("real-animation-sample damage=%d opacity=%.3f translate=%.3f,%.3f active=%t",
		damage, coinStyle.Opacity, transform.E, transform.F, page.ActiveAnimations(clock.current)))

	wantBytes, err := os.ReadFile("testdata/real-site-visual.golden.json")
	if err != nil {
		encoded, _ := json.MarshalIndent(actual, "", "  ")
		t.Fatalf("read real-site visual golden: %v\n--- actual ---\n%s", err, encoded)
	}
	var want frameworkVisualGolden
	if err := json.Unmarshal(wantBytes, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, want) {
		encoded, _ := json.MarshalIndent(actual, "", "  ")
		t.Fatalf("real-site visual snapshot changed; inspect before updating golden\n--- actual ---\n%s", encoded)
	}
}
