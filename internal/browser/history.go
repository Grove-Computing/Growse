package browser

import "net/url"

type historyEntry struct {
	URL          *url.URL
	State        string
	SameDocument bool
	PageID       uint64
	ScrollX      float32
	ScrollY      float32
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
	h.entries = append(h.entries, cloneHistoryEntry(entry))
	h.index = len(h.entries) - 1
}

func (h *history) replace(entry *url.URL) {
	if entry == nil {
		return
	}
	if h.index < 0 || h.index >= len(h.entries) {
		h.push(entry)
		return
	}
	h.entries[h.index].URL = cloneURL(entry)
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
	index := h.index + delta
	if index < 0 || index >= len(h.entries) {
		return nil, h.index, false
	}
	return cloneHistoryEntry(h.entries[index]), index, true
}

func (h *history) current() (*historyEntry, bool) {
	if h.index < 0 || h.index >= len(h.entries) {
		return nil, false
	}
	return cloneHistoryEntry(h.entries[h.index]), true
}

func (h *history) canBack() bool {
	return h.index > 0
}

func (h *history) canForward() bool {
	return h.index >= 0 && h.index+1 < len(h.entries)
}

func (h *history) reset(entry *url.URL) {
	h.entries = nil
	h.index = -1
	h.push(entry)
}
