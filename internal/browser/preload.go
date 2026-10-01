package browser

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/Grove-Computing/Growse/internal/network"
	xhtml "golang.org/x/net/html"
)

const maxStreamingPreloads = 32

type preloadFuture struct {
	ready    chan struct{}
	response *network.Response
	err      error
}

// streamingPreloader scans start tags while the main HTML response is still
// arriving. It also coalesces the parser's later request with speculative work.
type streamingPreloader struct {
	ctx      context.Context
	loader   requestLoader
	siteURL  *url.URL
	baseURL  *url.URL
	observer func(network.Observation)
	engine   string
	writer   *io.PipeWriter
	scanDone chan struct{}
	mu       sync.Mutex
	futures  map[string]*preloadFuture
	count    int
}

func newStreamingPreloader(ctx context.Context, loader requestLoader, siteURL *url.URL, engine string, observer func(network.Observation)) *streamingPreloader {
	reader, writer := io.Pipe()
	preloader := &streamingPreloader{
		ctx: ctx, loader: loader, siteURL: cloneURL(siteURL), baseURL: cloneURL(siteURL), observer: observer, engine: engine,
		writer: writer, scanDone: make(chan struct{}), futures: make(map[string]*preloadFuture),
	}
	go preloader.scan(reader)
	return preloader
}

func (p *streamingPreloader) write(chunk []byte) {
	if p == nil || p.writer == nil || len(chunk) == 0 {
		return
	}
	_, _ = p.writer.Write(chunk)
}

func (p *streamingPreloader) finishInput() {
	if p == nil {
		return
	}
	_ = p.writer.Close()
	<-p.scanDone
}

func (p *streamingPreloader) scan(reader io.Reader) {
	defer close(p.scanDone)
	tokenizer := xhtml.NewTokenizer(reader)
	for {
		switch tokenizer.Next() {
		case xhtml.ErrorToken:
			return
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			token := tokenizer.Token()
			attributes := make(map[string]string, len(token.Attr))
			for _, attribute := range token.Attr {
				attributes[strings.ToLower(attribute.Key)] = attribute.Val
			}
			switch strings.ToLower(token.Data) {
			case "base":
				if target := p.resolve(attributes["href"]); target != nil {
					p.baseURL = target
				}
			case "link":
				rel := strings.Fields(strings.ToLower(attributes["rel"]))
				if containsToken(rel, "stylesheet") {
					p.schedule(attributes["href"], network.RequestStylesheet)
				} else if containsToken(rel, "preload") {
					switch strings.ToLower(attributes["as"]) {
					case "style":
						p.schedule(attributes["href"], network.RequestStylesheet)
					case "image":
						p.schedule(attributes["href"], network.RequestImage)
					case "font":
						p.schedule(attributes["href"], network.RequestFont)
					}
				}
			case "img":
				p.schedule(attributes["src"], network.RequestImage)
			}
		}
	}
}

func containsToken(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (p *streamingPreloader) resolve(raw string) *url.URL {
	raw = strings.TrimSpace(raw)
	if raw == "" || p.baseURL == nil {
		return nil
	}
	reference, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	target := p.baseURL.ResolveReference(reference)
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil
	}
	return target
}

func (p *streamingPreloader) schedule(raw string, kind network.RequestKind) {
	target := p.resolve(raw)
	if target == nil || p.loader == nil {
		return
	}
	key := target.String()
	p.mu.Lock()
	if p.count >= maxStreamingPreloads || p.futures[key] != nil {
		p.mu.Unlock()
		return
	}
	future := &preloadFuture{ready: make(chan struct{})}
	p.futures[key] = future
	p.count++
	p.mu.Unlock()
	go func() {
		future.response, future.err = p.loader.Do(p.ctx, &network.Request{
			Method: http.MethodGet, URL: target, SiteURL: cloneURL(p.siteURL), Kind: kind, Engine: p.engine,
			Initiator: "preload-scanner", Schedule: "speculative", Observer: p.observer,
		})
		close(future.ready)
	}()
}

func (p *streamingPreloader) Get(ctx context.Context, target *url.URL) (*network.Response, error) {
	return p.Do(ctx, &network.Request{Method: http.MethodGet, URL: target})
}

func (p *streamingPreloader) Do(ctx context.Context, request *network.Request) (*network.Response, error) {
	if p == nil || request == nil || request.URL == nil {
		return nil, context.Canceled
	}
	method := request.Method
	if method == "" {
		method = http.MethodGet
	}
	if method == http.MethodGet && len(request.Body) == 0 {
		p.mu.Lock()
		future := p.futures[request.URL.String()]
		p.mu.Unlock()
		if future != nil {
			select {
			case <-future.ready:
				return future.response, future.err
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	return p.loader.Do(ctx, request)
}
