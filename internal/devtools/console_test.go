package devtools

import (
	"sync"
	"testing"
)

func TestPageStoreConsoleIsBoundedAndOrdered(t *testing.T) {
	store := newPageStore(2, 8)
	store.AddConsole(ConsoleInfo, "webgo", "first")
	store.AddConsole(ConsoleWarn, "runtime", "123456789")
	store.AddConsole(ConsoleError, "runtime", "last")

	records := store.Console()
	if len(records) != 2 || records[0].Sequence != 2 || records[1].Sequence != 3 {
		t.Fatalf("records = %+v", records)
	}
	if records[0].Message != "12345…" {
		t.Fatalf("truncated message = %q", records[0].Message)
	}
}

func TestPageStoreConsoleRetainsRuntimeEngine(t *testing.T) {
	store := NewPageStore()
	store.AddConsoleForEngine(ConsoleWarn, "javascript", "console", "message")
	records := store.Console()
	if len(records) != 1 || records[0].Engine != "javascript" || records[0].Source != "console" {
		t.Fatalf("Console() = %#v, want JavaScript console record", records)
	}
}

func TestPageStoreConsoleConcurrentAccessAndClose(t *testing.T) {
	store := newPageStore(1000, 4096)
	var group sync.WaitGroup
	for index := 0; index < 50; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			store.AddConsole(ConsoleLog, "webgo", "message")
			_ = store.Console()
		}()
	}
	group.Wait()
	if got := len(store.Console()); got != 50 {
		t.Fatalf("record count = %d, want 50", got)
	}
	store.Close()
	store.AddConsole(ConsoleError, "runtime", "ignored")
	if got := len(store.Console()); got != 0 {
		t.Fatalf("record count after close = %d, want 0", got)
	}
}
