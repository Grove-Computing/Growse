package browser

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/serviceworker"
)

type serviceWorkerLoader struct {
	ResourceLoader
	manager *serviceworker.Manager
}

func (loader serviceWorkerLoader) Get(ctx context.Context, target *url.URL) (*network.Response, error) {
	return loader.Do(ctx, &network.Request{Method: http.MethodGet, URL: target, Kind: network.RequestSubresource})
}

func (loader serviceWorkerLoader) Do(ctx context.Context, request *network.Request) (*network.Response, error) {
	fallback := func(fallbackContext context.Context, fallbackRequest *network.Request) (*network.Response, error) {
		if requestClient, ok := loader.ResourceLoader.(requestLoader); ok {
			return requestClient.Do(fallbackContext, fallbackRequest)
		}
		if loader.ResourceLoader == nil || fallbackRequest == nil || fallbackRequest.Method != "" && fallbackRequest.Method != http.MethodGet {
			return nil, errors.New("service worker network fallback is unavailable")
		}
		return loader.ResourceLoader.Get(fallbackContext, fallbackRequest.URL)
	}
	if loader.manager == nil {
		return fallback(ctx, request)
	}
	return loader.manager.DispatchFetch(ctx, request, fallback)
}

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
