package browser

import "net/url"

type history struct {
	entries []*url.URL
	index   int
}

func newHistory() history {
	return history{index: -1}
}

func (h *history) push(entry *url.URL) {
	if entry == nil {
		return
	}
	if h.index+1 < len(h.entries) {
		h.entries = h.entries[:h.index+1]
	}
	h.entries = append(h.entries, cloneURL(entry))
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
	h.entries[h.index] = cloneURL(entry)
}

func (h *history) target(delta int) (*url.URL, int, bool) {
	index := h.index + delta
	if index < 0 || index >= len(h.entries) {
		return nil, h.index, false
	}
	return cloneURL(h.entries[index]), index, true
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
