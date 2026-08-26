package browser

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/html"
)

// WPT source: html/semantics/embedded-content/the-iframe-element/sandbox_005.htm.
func TestWPTIframeEmptySandboxBlocksScriptExecution(t *testing.T) {
	document, err := html.Parse(strings.NewReader(`<iframe id="blocked" sandbox></iframe><iframe id="allowed" sandbox="allow-scripts allow-same-origin unknown-token"></iframe>`))
	if err != nil {
		t.Fatal(err)
	}
	blocked, ok := document.GetElementByID("blocked")
	if !ok {
		t.Fatal("blocked iframe was not parsed")
	}
	blockedPolicy := parseFramePolicy(blocked)
	if !blockedPolicy.Sandboxed || blockedPolicy.AllowsScripts() || !blockedPolicy.HasOpaqueOrigin() {
		t.Fatalf("empty sandbox policy = %+v", blockedPolicy)
	}
	allowed, ok := document.GetElementByID("allowed")
	if !ok {
		t.Fatal("allowed iframe was not parsed")
	}
	allowedPolicy := parseFramePolicy(allowed)
	if !allowedPolicy.AllowsScripts() || allowedPolicy.HasOpaqueOrigin() || allowedPolicy.AllowForms || allowedPolicy.AllowPopups {
		t.Fatalf("token sandbox policy = %+v", allowedPolicy)
	}
}
