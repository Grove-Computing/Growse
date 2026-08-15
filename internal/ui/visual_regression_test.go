package ui

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"reflect"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/Grove-Computing/Growse/internal/browser"
)

type tabRailVisualSnapshot struct {
	Viewport           string   `json:"viewport"`
	ChromeGeometry     []string `json:"chrome_geometry"`
	NarrowGeometry     []string `json:"narrow_geometry"`
	TabRows            []string `json:"tab_rows"`
	Overflow           string   `json:"overflow"`
	HorizontalTabStrip bool     `json:"horizontal_tab_strip"`
}

func TestVerticalTabRailVisualRegression(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	gtx := layout.Context{
		Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(1280, 800)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	geometry := calculateBrowserChromeGeometry(gtx.Constraints.Max, gtx.Dp(tabRailWidth), gtx.Dp(toolbarHeight))
	narrow := calculateBrowserChromeGeometry(image.Pt(300, 180), gtx.Dp(tabRailWidth), gtx.Dp(toolbarHeight))
	rowContext := gtx
	rowContext.Constraints = layout.Constraints{Max: image.Pt(204, 400)}
	rows := []browser.TabSnapshot{
		{Active: true, Title: "Notes", URL: "https://example.test/notes"},
		{Loading: true, Title: "Tasks", URL: "https://example.test/tasks"},
		{Error: true, PendingUpdate: true, Title: "Activity", URL: "https://example.test/activity"},
	}
	rowSnapshot := make([]string, 0, len(rows))
	for _, row := range rows {
		dimensions := ui.layoutTabRow(rowContext, row)
		rowSnapshot = append(rowSnapshot, fmt.Sprintf("size=%dx%d title=%q state=%q", dimensions.Size.X, dimensions.Size.Y, tabDisplayTitle(row), tabStateLabel(row)))
	}

	session := browser.NewSession()
	defer session.Close()
	for range 12 {
		if _, err := session.NewTab(nil); err != nil {
			t.Fatal(err)
		}
	}
	overflowUI := NewBrowserUIWithTabs(nil, session, nil)
	overflowUI.tabList.Position = layout.Position{First: 5, Offset: 3}
	overflowContext := gtx
	overflowContext.Constraints = layout.Exact(image.Pt(204, 128))
	overflowSize := overflowUI.layoutTabList(overflowContext, session.Tabs()).Size

	snapshot := tabRailVisualSnapshot{
		Viewport: "1280x800",
		ChromeGeometry: []string{
			"rail=" + geometry.tabRail.String(), "toolbar=" + geometry.toolbar.String(), "page=" + geometry.viewport.String(),
		},
		NarrowGeometry: []string{
			"rail=" + narrow.tabRail.String(), "toolbar=" + narrow.toolbar.String(), "page=" + narrow.viewport.String(),
		},
		TabRows:  rowSnapshot,
		Overflow: fmt.Sprintf("tabs=12 viewport=%dx%d scroll=%d,%d", overflowSize.X, overflowSize.Y, overflowUI.tabList.Position.First, overflowUI.tabList.Position.Offset),
	}
	wantBytes, err := os.ReadFile("testdata/vertical-tabs.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var want tabRailVisualSnapshot
	if err := json.Unmarshal(wantBytes, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot, want) {
		actual, _ := json.MarshalIndent(snapshot, "", "  ")
		t.Fatalf("vertical tab visual snapshot changed; inspect before updating golden\n--- actual ---\n%s", actual)
	}
}
