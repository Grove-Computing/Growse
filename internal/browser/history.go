package browser

import "net/url"

type historyEntry struct {
	URL          *url.URL
	State        string
	SameDocument bool
	PageID       uint64
	ScrollFirst  int
	ScrollOffset int
}

func (entry *historyEntry) String() string {
	if entry == nil || entry.URL == nil {
		return ""
	}
	return entry.URL.String()
}

func cloneHistoryEntry(source *historyEntry) *historyEntry {
	if source == nil {
		return nil
	}
	copy := *source
	copy.URL = cloneURL(source.URL)
	return &copy
}

type history struct {
	entries []*historyEntry
	index   int
}

func newHistory() history {
	return history{index: -1}
}

func (h *history) push(entry *url.URL) {
	h.pushEntry(&historyEntry{URL: entry})
}

func (h *history) pushEntry(entry *historyEntry) {
	if entry == nil || entry.URL == nil {
		return
	}
	if h.index+1 < len(h.entries) {
		h.entries = h.entries[:h.index+1]
	}
	if len(h.entries) >= maxHistoryEntries {
		h.entries = h.entries[1:]
		h.index--
	}
	h.entries = append(h.entries, cloneHistoryEntry(entry))
	h.index = len(h.entries) - 1
}

func (h *history) replaceEntry(entry *historyEntry) {
	if entry == nil || entry.URL == nil {
		return
	}
	if h.index < 0 || h.index >= len(h.entries) {
		h.pushEntry(entry)
		return
	}
	h.entries[h.index] = cloneHistoryEntry(entry)
}

func (h *history) target(delta int) (*url.URL, int, bool) {
	entry, index, ok := h.targetEntry(delta)
	if !ok {
		return nil, h.index, false
	}
	return cloneURL(entry.URL), index, true
}

func (h *history) targetEntry(delta int) (*historyEntry, int, bool) {
	if delta > len(h.entries) || delta < -len(h.entries) {
		return nil, h.index, false
	}
	index := h.index + delta
	if index < 0 || index >= len(h.entries) {
		return nil, h.index, false
	}
	return cloneHistoryEntry(h.entries[index]), index, true
}

func (h *history) stateBytesAfterPush(state string) int {
	total := len(state)
	last := min(h.index, len(h.entries)-1)
	for index := 0; index <= last; index++ {
		if h.entries[index] != nil {
			total += len(h.entries[index].State)
		}
	}
	return total
}

func (h *history) stateBytesAfterReplace(state string) int {
	total := len(state)
	for index, entry := range h.entries {
		if index != h.index && entry != nil {
			total += len(entry.State)
		}
	}
	return total
}

func (h *history) current() (*historyEntry, bool) {
	if h.index < 0 || h.index >= len(h.entries) {
		return nil, false
	}
	return cloneHistoryEntry(h.entries[h.index]), true
}

func (h *history) rebindPage(previousID, currentID uint64) {
	if previousID == 0 || currentID == 0 || previousID == currentID {
		return
	}
	for _, entry := range h.entries {
		if entry != nil && entry.PageID == previousID {
			entry.PageID = currentID
		}
	}
}

func (h *history) canBack() bool {
	return h.index > 0
}

func (h *history) canForward() bool {
	return h.index >= 0 && h.index+1 < len(h.entries)
}
