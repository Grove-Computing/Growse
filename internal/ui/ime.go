package ui

import (
	"reflect"

	"gioui.org/widget"
)

// imeInputState keeps application actions in step with Gio's composition
// range. widget.Editor intentionally owns the range, so read it only to decide
// whether application-level search and candidate actions must be deferred.
// The pinned Gio version is covered by editorCompositionActive tests; if its
// internal layout changes, the safe fallback is to report no composition.
type imeInputState struct {
	initialized bool
	composing   bool
	dirty       bool
}

func (state *imeInputState) active(_ *widget.Editor) bool {
	return state.composing
}

// changed reports whether a ChangeEvent contains committed text. Composition
// edits stay private until Gio closes the composition range.
func (state *imeInputState) changed(editor *widget.Editor) bool {
	if !state.initialized {
		state.initialized = true
		return true
	}
	if editorCompositionActive(editor) {
		state.composing = true
		state.dirty = true
		return false
	}
	state.composing = false
	state.dirty = false
	return true
}

// committed reports a composition that ended without its final edit producing
// a separate ChangeEvent in the same frame.
func (state *imeInputState) committed(editor *widget.Editor) bool {
	if !state.initialized {
		state.initialized = true
		return false
	}
	if editorCompositionActive(editor) {
		state.composing = true
		return false
	}
	committed := state.composing && state.dirty
	state.composing = false
	state.dirty = false
	return committed
}

func editorCompositionActive(editor *widget.Editor) bool {
	if editor == nil {
		return false
	}
	value := reflect.ValueOf(editor)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return false
	}
	value = value.Elem()
	ime := value.FieldByName("ime")
	if !ime.IsValid() {
		return false
	}
	composition := ime.FieldByName("composition")
	if !composition.IsValid() {
		return false
	}
	start := composition.FieldByName("Start")
	end := composition.FieldByName("End")
	if !start.IsValid() || !end.IsValid() || !start.CanInt() || !end.CanInt() {
		return false
	}
	return start.Int() >= 0 && end.Int() >= 0
}
