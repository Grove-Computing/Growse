package ui

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/devtools"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

func TestDevToolsRuntimeLabelShowsEngineAndLifecycle(t *testing.T) {
	tests := []struct {
		page *browser.Page
		want string
	}{
		{page: nil, want: "Engine: - · Runtime: no page"},
		{page: &browser.Page{Engine: runtimemodel.EngineGo}, want: "Engine: go · Runtime: idle"},
		{page: &browser.Page{Engine: runtimemodel.EngineJavaScript, RuntimeStarted: true}, want: "Engine: javascript · Runtime: running"},
		{page: &browser.Page{Engine: runtimemodel.EngineJavaScript, RuntimeError: "failed"}, want: "Engine: javascript · Runtime: error"},
		{page: &browser.Page{Engine: runtimemodel.EngineGo, Scripts: []browser.Script{{Source: "page"}}}, want: "Engine: go · Runtime: stopped"},
	}
	for _, test := range tests {
		if got := devToolsRuntimeLabel(test.page); got != test.want {
			t.Errorf("devToolsRuntimeLabel(%#v) = %q, want %q", test.page, got, test.want)
		}
	}
}

func TestNetworkRecordLabelIncludesScriptEngine(t *testing.T) {
	record := devtools.NetworkRecord{Sequence: 1, Kind: "script", Engine: "javascript", Method: "GET", URL: "http://localhost/app.js"}
	if got := networkRecordLabel(record, "200", "-"); !strings.Contains(got, "script/javascript") {
		t.Fatalf("networkRecordLabel() = %q, want script/javascript", got)
	}
}
