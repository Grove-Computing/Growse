// Package app starts the Growse desktop application.
package app

import (
	"log"

	gioapp "gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/javascript"
	"github.com/Grove-Computing/Growse/internal/runtime/yaegi"
	storagecore "github.com/Grove-Computing/Growse/internal/storage"
	"github.com/Grove-Computing/Growse/internal/ui"
	"github.com/Grove-Computing/Growse/internal/updater"
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
	networkClient := network.NewClient()
	if cacheRoot, err := network.DefaultCacheRoot(); err == nil {
		if persistent, persistentErr := network.NewClientWithCacheRoot(nil, 0, cacheRoot); persistentErr == nil {
			networkClient = persistent
		} else {
			log.Printf("HTTP Cache profileを初期化できませんでした: %v", persistentErr)
		}
	} else {
		log.Printf("HTTP Cache directoryを解決できませんでした: %v", err)
	}
	session := browser.NewSession(func() *browser.Browser {
		state := browser.NewWithEngineFactoryAndStorage(networkClient, func(engine runtimemodel.Engine) runtimemodel.Runtime {
			switch runtimemodel.NormalizeEngine(engine) {
			case runtimemodel.EngineGo:
				return yaegi.New()
			case runtimemodel.EngineJavaScript:
				return javascript.New()
			default:
				return nil
			}
		}, storageManager.NewPageSession())
		return state
	})
	session.SetOnActiveMutation(window.Invalidate)
	if _, err := session.NewTab(nil); err != nil {
		return err
	}
	defer session.Close()
	browserUI := ui.NewBrowserUIWithTabsAndUpdater(nil, session, window.Invalidate, updater.New(), func() {
		window.Perform(system.ActionClose)
	})
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
