// Package app starts the Growse desktop application.
package app

import (
	"log"

	gioapp "gioui.org/app"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/yaegi"
	storagecore "github.com/Grove-Computing/Growse/internal/storage"
	"github.com/Grove-Computing/Growse/internal/ui"
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
	storageManager := storagecore.NewManager()
	if dataRoot, err := storagecore.DefaultDataRoot(); err == nil {
		if persistent, persistentErr := storagecore.NewPersistentManager(dataRoot); persistentErr == nil {
			storageManager = persistent
		} else {
			log.Printf("Local Storage profileを初期化できませんでした: %v", persistentErr)
		}
	} else {
		log.Printf("Local Storage data directoryを解決できませんでした: %v", err)
	}
	browserState := browser.NewWithRuntimeFactoryAndStorage(network.NewClient(), func() runtimemodel.Runtime {
		return yaegi.New()
	}, storageManager)
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
