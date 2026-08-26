package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/yaegi"
	storagecore "github.com/Grove-Computing/Growse/internal/storage"
)

func TestWorkspaceServesThreeTabsSharedAssetsAndCookieFixture(t *testing.T) {
	server := httptest.NewServer(workspaceHandler())
	defer server.Close()
	client := server.Client()
	for _, page := range []string{"notes.html", "tasks.html", "activity.html"} {
		response, err := client.Get(server.URL + "/" + page)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		content := string(body)
		if response.StatusCode != http.StatusOK || !strings.Contains(content, "target=\"_blank\"") || !strings.Contains(content, "/style.css") {
			t.Fatalf("%s fixture is incomplete: status=%d", page, response.StatusCode)
		}
	}
	login, err := client.Get(server.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	login.Body.Close()
	if len(login.Cookies()) != 1 || login.Cookies()[0].Name != "workspace_session" || !login.Cookies()[0].HttpOnly {
		t.Fatalf("login Cookies = %+v", login.Cookies())
	}
	style, err := client.Get(server.URL + "/style.css")
	if err != nil {
		t.Fatal(err)
	}
	style.Body.Close()
	if style.Header.Get("Cache-Control") != "max-age=3600" {
		t.Fatalf("shared asset Cache-Control = %q", style.Header.Get("Cache-Control"))
	}
	for _, script := range []string{"_notes.go", "_tasks.go", "_activity.go"} {
		content, err := workspaceAssets.ReadFile(script)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), "storage.") {
			t.Fatalf("%s does not use shared storage", script)
		}
	}
}

func TestWorkspaceWebGoStartsInThreeIsolatedTabs(t *testing.T) {
	server := httptest.NewServer(workspaceHandler())
	defer server.Close()
	client := network.NewClientWithLimits(server.Client(), 1<<20)
	profile := storagecore.NewManager()
	session := browser.NewSession(func() *browser.Browser {
		return browser.NewWithRuntimeFactoryAndStorage(client, func() runtimemodel.Runtime { return yaegi.New() }, profile.NewPageSession())
	})
	defer session.Close()

	for _, pageName := range []string{"notes.html", "tasks.html", "activity.html"} {
		tab, err := session.NewTab(nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := session.SelectTab(tab.ID); err != nil {
			t.Fatal(err)
		}
		_, state, ok := session.ActiveBrowserTarget()
		if !ok {
			t.Fatal("active Browser missing")
		}
		mutations := make(chan struct{}, 32)
		state.SetOnMutation(func() {
			select {
			case mutations <- struct{}{}:
			default:
			}
		})
		page, err := state.Navigate(context.Background(), server.URL+"/"+pageName)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.ScriptErrors) != 0 || !page.RuntimeStarted || page.RuntimeError != "" {
			t.Fatalf("%s WebGo state = started:%t load:%v runtime:%q", pageName, page.RuntimeStarted, page.ScriptErrors, page.RuntimeError)
		}
		if pageName == "activity.html" {
			waitForWorkspaceActivity(t, state, mutations)
		}
	}
}

func waitForWorkspaceActivity(t *testing.T, engine *browser.Browser, mutations <-chan struct{}) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		ready := false
		engine.InspectPage(func(page *browser.Page) bool {
			status, statusOK := page.Document.GetElementByID("status")
			feed, feedOK := page.Document.GetElementByID("activity-feed")
			ready = statusOK && feedOK && status.TextContent() == "online" && feed.TextContent() == "workspace profile synchronized"
			return true
		})
		if ready {
			return
		}
		select {
		case <-mutations:
		case <-deadline.C:
			page := engine.Page()
			status, _ := page.Document.GetElementByID("status")
			feed, _ := page.Document.GetElementByID("activity-feed")
			t.Fatalf("activity callback did not finish: status=%q feed=%q errors=%v runtime=%q", status.TextContent(), feed.TextContent(), page.ScriptErrors, page.RuntimeError)
		}
	}
}
