// Package app starts the Growse desktop application.
package app

import (
	"log"

	gioapp "gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// Run opens the Growse browser window and starts its event loop.
func Run() {
	go func() {
		window := new(gioapp.Window)
		window.Option(
			gioapp.Title("Growse"),
			gioapp.Size(unit.Dp(1280), unit.Dp(800)),
		)

		if err := runWindow(window); err != nil {
			log.Printf("Growse window closed with an error: %v", err)
		}
	}()

	gioapp.Main()
}

func runWindow(window *gioapp.Window) error {
	var ops op.Ops
	theme := material.NewTheme()

	for {
		switch event := window.Event().(type) {
		case gioapp.DestroyEvent:
			return event.Err
		case gioapp.FrameEvent:
			gtx := gioapp.NewContext(&ops, event)
			render(gtx, theme)
			event.Frame(gtx.Ops)
		}
	}
}

func render(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	paint.ColorOp{Color: theme.Bg}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return material.H6(theme, "Growse").Layout(gtx)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, material.Body1(theme, "Browser engine scaffold").Layout)
		}),
	)
}
