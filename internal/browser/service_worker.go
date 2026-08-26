package browser

import (
	"context"
	"errors"
	"net/http"

	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/serviceworker"
)

func serviceWorkerHost(ctx context.Context, page *Page, client ResourceLoader) *runtimemodel.ServiceWorkerHost {
	if page == nil || page.URL == nil || page.serviceWorkers == nil || !serviceworker.IsSecureContext(page.URL) {
		return nil
	}
	manager := page.serviceWorkers
	fetch := func(fetchContext context.Context, request *network.Request) (*network.Response, error) {
		if request == nil || request.URL == nil {
			return nil, errors.New("service worker request is invalid")
		}
		if loader, ok := client.(requestLoader); ok {
			copy := *request
			copy.Observer = page.ensureDevTools().ObserveNetwork
			return loader.Do(fetchContext, &copy)
		}
		if client == nil || request.Method != http.MethodGet {
			return nil, errors.New("service worker network loader is unavailable")
		}
		return client.Get(fetchContext, request.URL)
	}
	return &runtimemodel.ServiceWorkerHost{
		Register: func(scriptURL, scope string) (runtimemodel.ServiceWorkerRegistration, error) {
			return manager.Register(ctx, page.URL, scriptURL, scope, fetch)
		},
		Update: func(scope string) (runtimemodel.ServiceWorkerRegistration, error) {
			return manager.Update(ctx, page.URL, scope, fetch)
		},
		Unregister: func(scope string) (bool, error) { return manager.Unregister(page.URL, scope) },
		GetRegistration: func(clientURL string) (*runtimemodel.ServiceWorkerRegistration, error) {
			return manager.GetRegistration(page.URL, clientURL)
		},
		GetRegistrations: func() ([]runtimemodel.ServiceWorkerRegistration, error) {
			return manager.GetRegistrations(page.URL)
		},
		Controller: func() *runtimemodel.ServiceWorkerRegistration { return manager.Controller(page.URL) },
	}
}
