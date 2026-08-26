package dom

import "testing"

func TestDocumentSnapshotRoundTripPreservesIDsAndControlState(t *testing.T) {
	document := NewDocument()
	html := document.CreateElement("html", nil)
	input := document.CreateElement("input", map[string]string{"id": "name", "class": "field"})
	input.ControlValue = "edited"
	input.ControlValueDirty = true
	if err := document.AppendChild(document.Root, html); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(html, input); err != nil {
		t.Fatal(err)
	}

	clone, err := NewDocumentFromSnapshot(document.Snapshot())
	if err != nil {
		t.Fatalf("NewDocumentFromSnapshot() error = %v", err)
	}
	got, ok := clone.NodeByID(input.ID)
	if !ok || got.TagName != "input" || got.ControlValue != "edited" || !got.ControlValueDirty || got.Parent == nil {
		t.Fatalf("round-trip input = %#v", got)
	}
	if byID, ok := clone.GetElementByID("name"); !ok || byID != got {
		t.Fatalf("round-trip ID index = %#v, %t", byID, ok)
	}
}

func TestApplySnapshotReusesRetainedNodesAndDisconnectsRemovedNodes(t *testing.T) {
	document := NewDocument()
	parent := document.CreateElement("main", nil)
	retained := document.CreateElement("p", map[string]string{"id": "message"})
	removed := document.CreateElement("span", nil)
	_ = document.AppendChild(document.Root, parent)
	_ = document.AppendChild(parent, retained)
	_ = document.AppendChild(parent, removed)

	snapshot := document.Snapshot()
	snapshot.Root.Children[0].Children = snapshot.Root.Children[0].Children[:1]
	snapshot.Root.Children[0].Children[0].Attributes["class"] = "ready"
	if err := document.ApplySnapshot(snapshot); err != nil {
		t.Fatalf("ApplySnapshot() error = %v", err)
	}
	current, ok := document.NodeByID(retained.ID)
	if !ok || current != retained {
		t.Fatal("retained node identity changed")
	}
	if value, _ := retained.Attribute("class"); value != "ready" {
		t.Fatalf("retained class = %q", value)
	}
	if _, ok := document.NodeByID(removed.ID); ok || removed.Parent != nil {
		t.Fatal("removed node remained connected or indexed")
	}
}

func TestApplySnapshotRejectsMalformedTreesWithoutReplacingDocument(t *testing.T) {
	document := NewDocument()
	root := document.Root
	snapshot := document.Snapshot()
	snapshot.Root.Children = []NodeSnapshot{{ID: snapshot.Root.ID, Type: NodeElement}}
	if err := document.ApplySnapshot(snapshot); err == nil {
		t.Fatal("ApplySnapshot() accepted duplicate node ID")
	}
	if document.Root != root {
		t.Fatal("malformed snapshot replaced document root")
	}
}

func TestDocumentSnapshotPreservesAndValidatesReadyState(t *testing.T) {
	document := NewDocument()
	if document.ReadyState() != "loading" || !document.SetReadyState("interactive") {
		t.Fatalf("initial ready state = %q", document.ReadyState())
	}
	clone, err := NewDocumentFromSnapshot(document.Snapshot())
	if err != nil || clone.ReadyState() != "interactive" {
		t.Fatalf("snapshot ready state = %q, %v", clone.ReadyState(), err)
	}
	invalid := document.Snapshot()
	invalid.ReadyState = "stale"
	if err := clone.ApplySnapshot(invalid); err == nil {
		t.Fatal("invalid ready state was accepted")
	}
}
