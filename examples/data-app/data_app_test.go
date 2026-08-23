package dataapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/yaegi"
)

func TestDataAppLoadsSessionDataThroughWebGoFetch(t *testing.T) {
	index := readFixture(t, "index.html")
	stylesheet := readFixture(t, "style.css")
	script := readFixture(t, "_app.go")
	cookieSeen := make(chan struct{}, 1)
	apiStarted := make(chan struct{})
	releaseAPI := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/":
			http.SetCookie(response, &http.Cookie{Name: "session", Value: "data-app", Path: "/", HttpOnly: true})
			response.Header().Set("Content-Type", "text/html")
			_, _ = response.Write(index)
		case "/style.css":
			response.Header().Set("Content-Type", "text/css")
			_, _ = response.Write(stylesheet)
		case "/_app.go":
			response.Header().Set("Content-Type", "text/x-go")
			_, _ = response.Write(script)
		case "/api/items":
			if request.Header.Get("Accept") != "application/json" {
				http.Error(response, "missing Accept", http.StatusBadRequest)
				return
			}
			close(apiStarted)
			<-releaseAPI
			if cookie, err := request.Cookie("session"); err == nil && cookie.Value == "data-app" {
				cookieSeen <- struct{}{}
			}
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`[{"name":"Growse","done":false}]`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	succeeded := make(chan struct{})
	var successOnce sync.Once
	engine := browser.NewWithRuntimeFactory(network.NewClientWithLimits(server.Client(), 1<<20), func() runtimemodel.Runtime {
		return yaegi.New()
	})
	engine.SetOnMutation(func() {
		page := engine.Page()
		if page == nil || page.Document == nil {
			return
		}
		status, ok := page.Document.GetElementByID("status")
		if ok && status.TextContent() == "success" {
			successOnce.Do(func() { close(succeeded) })
		}
	})
	page, err := engine.Navigate(context.Background(), server.URL+"/")
	if err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	select {
	case <-apiStarted:
	case <-time.After(time.Second):
		t.Fatal("Data App Fetch did not start")
	}
	close(releaseAPI)
	select {
	case <-succeeded:
	case <-time.After(time.Second):
		status, _ := page.Document.GetElementByID("status")
		t.Fatalf("Data App state = %q, want success", status.TextContent())
	}
	select {
	case <-cookieSeen:
	default:
		t.Fatal("WebGo Fetch did not receive the Navigation Session Cookie")
	}
	items, _ := page.Document.GetElementByID("items")
	if !strings.Contains(items.TextContent(), "Growse") || !strings.Contains(string(stylesheet), "@keyframes loading-pulse") {
		t.Fatalf("items = %q animation stylesheet missing = %t", items.TextContent(), !strings.Contains(string(stylesheet), "@keyframes loading-pulse"))
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return content
}
