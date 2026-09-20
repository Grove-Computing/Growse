package ui

import (
	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
)

type pointerState struct {
	position f32.Point
	inside   bool
}

type pointerTag struct{}

func (ui *BrowserUI) handlePointerEvents(gtx layout.Context) {
	if ui == nil {
		return
	}
	for {
		raw, ok := gtx.Event(pointer.Filter{
			Target: &ui.pointerTag,
			Kinds:  pointer.Enter | pointer.Move | pointer.Drag | pointer.Press | pointer.Release | pointer.Leave | pointer.Cancel,
		})
		if !ok {
			return
		}
		event, ok := raw.(pointer.Event)
		if !ok || event.Source != pointer.Mouse {
			continue
		}
		switch event.Kind {
		case pointer.Leave, pointer.Cancel:
			ui.pointer.inside = false
			if ui.navigator != nil {
				ui.navigator.ClearHover()
			}
		default:
			ui.pointer.position = event.Position
			ui.pointer.inside = true
		}
		ui.invalidate()
	}
}

func (ui *BrowserUI) registerPointerTracker(gtx layout.Context) {
	if ui == nil {
		return
	}
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	event.Op(gtx.Ops, &ui.pointerTag)
	pass.Pop()
	area.Pop()
}
