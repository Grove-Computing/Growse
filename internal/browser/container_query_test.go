package browser

import (
	"context"
	"testing"

	"github.com/Grove-Computing/Growse/internal/network"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestNavigationStabilizesContainerQueriesAgainstLayout(t *testing.T) {
	pageURL := mustParseURL(t, "https://example.com/container.html")
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {
			URL: pageURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<style>
.card { container-type: inline-size; container-name: card; width: 400px }
.title { width: 25cqw; color: red }
@container card (min-width: 350px) { .title { width: 50cqw; color: green } }
</style><section class="card"><h1 class="title">Title</h1></section>`),
		},
	}}
	page, err := New(loader).Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	title, ok := page.Document.QuerySelector(".title")
	if !ok {
		t.Fatal("title not found")
	}
	computed, _ := page.ComputedStyles.For(title)
	if computed.Color != 0x008000ff || computed.Width.Kind != style.SizeLength || computed.Width.Value.Pixels != 200 {
		t.Fatalf("stable container style = %#v", computed)
	}
}
