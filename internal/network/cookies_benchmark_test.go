package network

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

func BenchmarkMatch1000Cookies(b *testing.B) {
	jar := newPolicyCookieJarWithLimits(cookieLimits{maxCookies: 1200, maxCookiesPerDomain: 1200, maxCookieBytes: 4096})
	target, err := url.Parse("https://benchmark.example.test/app/data")
	if err != nil {
		b.Fatal(err)
	}
	cookies := make([]*http.Cookie, 1000)
	for index := range cookies {
		cookies[index] = &http.Cookie{Name: fmt.Sprintf("cookie_%04d", index), Value: "value", Path: "/app"}
	}
	jar.SetCookies(target, cookies)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		_ = jar.cookiesForRequest(target, target, RequestFetch, http.MethodGet)
	}
}
