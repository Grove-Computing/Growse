package ui

import (
	"encoding/json"
	"fmt"
	"image"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/devtools"
	"github.com/Grove-Computing/Growse/internal/dom"
	layoutmodel "github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

type devToolsVisualSnapshot struct {
	Geometry  []string `json:"geometry"`
	Console   []string `json:"console"`
	Inspector []string `json:"inspector"`
	Network   []string `json:"network"`
}

func TestDevToolsPanelsVisualRegression(t *testing.T) {
	geometry := calculateBrowserChromeGeometryWithDevTools(image.Pt(1280, 800), 224, 92, 280)
	consoleStore := devtools.NewPageStore()
	consoleStore.AddConsole(devtools.ConsoleInfo, "webgo", "ready")
	consoleStore.AddConsole(devtools.ConsoleError, "runtime", "failed")
	consoleStore.AddConsole(devtools.ConsoleWarn, "webgo", strings.Repeat("x", devtools.DefaultMaxMessageBytes+20))
	consoleRecords := consoleStore.Console()

	document := dom.NewDocument()
	selected := document.CreateElement("input", map[string]string{"id": "password", "type": "password", "value": "visual-secret"})
	if err := document.AppendChild(document.Root, selected); err != nil {
		t.Fatal(err)
	}
	styles := stylemodel.Map{selected.ID: {Display: stylemodel.DisplayInlineBlock, FontSize: 16, FontWeight: 400, Opacity: 1}}
	tree := &layoutmodel.Tree{Bounds: map[dom.NodeID]layoutmodel.Rect{selected.ID: {X: 8, Y: 12, Width: 160, Height: 32}}}
	inspector := devtools.SnapshotInspector(document, styles, tree, selected.ID)
	deep := dom.NewDocument()
	parent := deep.Root
	for range devtools.MaxDOMDepth + 2 {
		child := deep.CreateElement("div", nil)
		if err := deep.AppendChild(parent, child); err != nil {
			t.Fatal(err)
		}
		parent = child
	}
	truncatedInspector := devtools.SnapshotInspector(deep, nil, nil, 0)

	networkStore := devtools.NewPageStore()
	target, _ := url.Parse("https://example.test/data?token=visual-secret")
	networkStore.ObserveNetwork(network.Observation{Method: "GET", URL: target, Kind: network.RequestFetch, Duration: 1500 * time.Microsecond, StatusCode: 200, CacheStatus: "hit", ResponseBytes: 42})
	networkStore.ObserveNetwork(network.Observation{Method: "GET", URL: target, Kind: network.RequestFetch, Duration: 20 * time.Millisecond, ErrorCategory: "timeout"})
	networkStore.ObserveNetwork(network.Observation{Method: "GET", URL: target, Kind: network.RequestFetch, ErrorCategory: "response_limit"})
	networkRecords := networkStore.Network()

	actual := devToolsVisualSnapshot{
		Geometry: []string{"viewport=" + geometry.viewport.String(), "devtools=" + geometry.devTools.String()},
		Console: []string{
			"empty=Console has no matching messages",
			fmt.Sprintf("normal=%04d/%s/%s/%s", consoleRecords[0].Sequence, consoleRecords[0].Level, consoleRecords[0].Source, consoleRecords[0].Message),
			fmt.Sprintf("error=%04d/%s/%s", consoleRecords[1].Sequence, consoleRecords[1].Level, consoleRecords[1].Message),
			fmt.Sprintf("truncated=bytes:%d suffix:%t", len(consoleRecords[2].Message), strings.HasSuffix(consoleRecords[2].Message, "…")),
		},
		Inspector: []string{
			"empty=Select a DOM node to inspect attributes, styles, and layout",
			"normal=" + inspectorNodeLabel(*inspector.SelectedNode),
			fmt.Sprintf("details=attrs:%v styles:%d box:%.0fx%.0f", inspector.SelectedNode.Attributes, len(inspector.Styles), inspector.Layout.Width, inspector.Layout.Height),
			fmt.Sprintf("truncated=%t nodes=%d", truncatedInspector.Truncated, len(truncatedInspector.Nodes)),
		},
		Network: []string{
			"empty=Network has no requests for this page",
			networkRecordLabel(networkRecords[0], "200", "hit"),
			networkRecordLabel(networkRecords[1], "error:timeout", "-"),
			networkRecordLabel(networkRecords[2], "error:response_limit", "-"),
		},
	}
	wantBytes, err := os.ReadFile("testdata/devtools-panels.golden.json")
	if err != nil {
		encoded, _ := json.MarshalIndent(actual, "", "  ")
		t.Fatalf("read DevTools golden: %v\n--- actual ---\n%s", err, encoded)
	}
	var want devToolsVisualSnapshot
	if err := json.Unmarshal(wantBytes, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, want) {
		encoded, _ := json.MarshalIndent(actual, "", "  ")
		t.Fatalf("DevTools panel visual snapshot changed; inspect before updating golden\n--- actual ---\n%s", encoded)
	}
}
