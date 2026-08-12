// Package app starts the Growse desktop application.
package app

import (
	"log"

	gioapp "gioui.org/app"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/saku0512/growse/internal/browser"
	"github.com/saku0512/growse/internal/network"
	runtimemodel "github.com/saku0512/growse/internal/runtime"
	"github.com/saku0512/growse/internal/runtime/yaegi"
	"github.com/saku0512/growse/internal/ui"
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
	browserState := browser.NewWithRuntimeFactory(network.NewClient(), func() runtimemodel.Runtime {
		return yaegi.New()
	})
	browserState.SetOnMutation(window.Invalidate)
	defer browserState.Close()
	browserUI := ui.NewBrowserUI(browserState, window.Invalidate)
	defer browserUI.Close()

	for {
		switch event := window.Event().(type) {
		case gioapp.DestroyEvent:
			return event.Err
		case gioapp.FrameEvent:
			gtx := gioapp.NewContext(&ops, event)
			browserUI.Layout(gtx)
			event.Frame(gtx.Ops)
		}
	}
}
