package javascript

import (
	"context"
	"net/url"
	"strings"
	"testing"

	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

func TestBrowserGlobalsSupportHydrationFixtureOperations(t *testing.T) {
	pageURL, _ := url.Parse("https://app.example.test/root/page?old=1")
	var message string
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `
		const url = new URL("../items?a=1&a=2#top", location.href);
		url.searchParams.append("space", "a b");
		url.searchParams.set("a", "3");
		url.pathname = "/next";
		const params = new URLSearchParams([["x", "1"], ["x", "2"], ["empty", ""]]);
		params.delete("x", "1");
		const encoded = new TextEncoder().encode("A✓");
		const target = new Uint8Array(4);
		const into = new TextEncoder().encodeInto("éx", target);
		const random = new Uint8Array(16);
		const sameRandom = crypto.getRandomValues(random) === random;
		console.log([
			url.origin, url.pathname, url.search, url.hash, url.searchParams.get("a"), url.searchParams.getAll("a").length,
			params.toString(), params.size, Array.from(params).length, URL.canParse("/ok", url), URL.canParse("/no-base"),
			Array.from(encoded).join(","), new TextDecoder().decode(encoded), into.read, into.written, Array.from(target).join(","),
			btoa("\x00\xff"), atob("AP8=").charCodeAt(1), typeof performance.now(), performance.now() >= 0,
			sameRandom, random.some(function (value) { return value !== 0; }), typeof process, typeof require
		].join("|"));`
	environment := runtimemodel.Environment{BaseURL: pageURL, ConsoleRecord: func(_, value string) { message = value }}
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := "https://app.example.test|/next|?a=3&space=a+b|#top|3|1|x=2&empty=|2|2|true|false|65,226,156,147|A✓|2|3|195,169,120,0|AP8=|255|number|true|true|true|undefined|undefined"
	if message != want {
		t.Fatalf("browser globals = %q, want %q", message, want)
	}
}

func TestBrowserGlobalsRejectInvalidAndOversizedInputs(t *testing.T) {
	var message string
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `
		const errors = [];
		try { new URL("relative"); } catch (error) { errors.push("url"); }
		try { new URLSearchParams("x=" + "a".repeat(8193)); } catch (error) { errors.push("params"); }
		try { new TextDecoder("utf-16"); } catch (error) { errors.push("label"); }
		try { new TextDecoder("utf-8", { fatal: true }).decode(new Uint8Array([255])); } catch (error) { errors.push("decode"); }
		try { btoa("✓"); } catch (error) { errors.push("btoa"); }
		try { atob("***"); } catch (error) { errors.push("atob"); }
		try { crypto.getRandomValues(new Float32Array(1)); } catch (error) { errors.push("random-type"); }
		try { crypto.getRandomValues(new Uint8Array(65537)); } catch (error) { errors.push("random-size"); }
		console.log(errors.join(","));`
	environment := runtimemodel.Environment{ConsoleRecord: func(_, value string) { message = value }}
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := "url,params,label,decode,btoa,atob,random-type,random-size"
	if message != want || strings.Contains(message, "panic") {
		t.Fatalf("browser global errors = %q, want %q", message, want)
	}
}
