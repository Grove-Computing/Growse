package browser

import "testing"

func TestSessionKeepsOrderedTabsAndOneActiveTab(t *testing.T) {
	session := NewSession()
	session.tabs = []*Tab{
		{id: 11, state: TabBackground},
		{id: 22, state: TabActive},
		{id: 33, state: TabBackground},
	}
	session.activeID = 22

	tabs := session.Tabs()
	if len(tabs) != 3 {
		t.Fatalf("len(Tabs()) = %d, want 3", len(tabs))
	}
	for index, wantID := range []TabID{11, 22, 33} {
		if tabs[index].ID != wantID || tabs[index].Position != index {
			t.Fatalf("Tabs()[%d] = %#v, want id %d at position %d", index, tabs[index], wantID, index)
		}
	}
	if tabs[0].Active || !tabs[1].Active || tabs[2].Active {
		t.Fatalf("active flags = %#v, want only the middle tab active", tabs)
	}

	active, ok := session.ActiveTab()
	if !ok || active.ID != 22 || active.State != TabActive {
		t.Fatalf("ActiveTab() = %#v, %t, want tab 22", active, ok)
	}
}

func TestEmptySessionHasNoActiveTab(t *testing.T) {
	session := NewSession()
	if tabs := session.Tabs(); len(tabs) != 0 {
		t.Fatalf("Tabs() = %#v, want empty", tabs)
	}
	if active, ok := session.ActiveTab(); ok {
		t.Fatalf("ActiveTab() = %#v, true, want no active tab", active)
	}
}
