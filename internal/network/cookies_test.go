package network

import (
	"net/http"
	"net/url"
	"reflect"
	"testing"
)

func TestCookiePolicyAppliesSecureHTTPOnlyAndSameSite(t *testing.T) {
	jar := newPolicyCookieJar()
	target := parseCookieURL(t, "https://app.example.com/data")
	jar.SetCookies(target, []*http.Cookie{
		{Name: "secure", Value: "yes", Path: "/", Secure: true},
		{Name: "http_only", Value: "secret", Path: "/", HttpOnly: true},
		{Name: "strict", Value: "yes", Path: "/", SameSite: http.SameSiteStrictMode},
		{Name: "lax", Value: "yes", Path: "/", SameSite: http.SameSiteLaxMode},
		{Name: "none", Value: "yes", Path: "/", SameSite: http.SameSiteNoneMode, Secure: true},
		{Name: "insecure_none", Value: "blocked", Path: "/", SameSite: http.SameSiteNoneMode},
	})

	sameSiteFetch := cookieValueMap(jar.cookiesForRequest(target, parseCookieURL(t, "https://www.example.com/page"), RequestFetch, http.MethodGet))
	if _, exists := sameSiteFetch["insecure_none"]; exists || len(sameSiteFetch) != 5 {
		t.Fatalf("same-site Fetch Cookies = %v", sameSiteFetch)
	}
	crossSiteFetch := cookieValueMap(jar.cookiesForRequest(target, parseCookieURL(t, "https://other.test/page"), RequestFetch, http.MethodGet))
	if !reflect.DeepEqual(crossSiteFetch, map[string]string{"none": "yes"}) {
		t.Fatalf("cross-site Fetch Cookies = %v", crossSiteFetch)
	}
	crossSiteNavigation := cookieValueMap(jar.cookiesForRequest(target, parseCookieURL(t, "https://other.test/page"), RequestNavigation, http.MethodGet))
	if !reflect.DeepEqual(crossSiteNavigation, map[string]string{
		"secure": "yes", "http_only": "secret", "lax": "yes", "none": "yes",
	}) {
		t.Fatalf("cross-site safe Navigation Cookies = %v", crossSiteNavigation)
	}
	plainHTTP := cookieValueMap(jar.cookiesForRequest(parseCookieURL(t, "http://app.example.com/data"), parseCookieURL(t, "http://app.example.com/page"), RequestFetch, http.MethodGet))
	if _, exists := plainHTTP["secure"]; exists {
		t.Fatalf("plain HTTP received Secure Cookie: %v", plainHTTP)
	}
	visible := cookieValueMap(jar.visibleCookies(target))
	if _, exists := visible["http_only"]; exists {
		t.Fatalf("WebGo-visible Cookies contain HttpOnly value: %v", visible)
	}
}

func TestCookieJarEnforcesSizeTotalAndPerDomainLimits(t *testing.T) {
	jar := newPolicyCookieJarWithLimits(cookieLimits{maxCookies: 2, maxCookiesPerDomain: 1, maxCookieBytes: 8})
	first := parseCookieURL(t, "https://first.example/path")
	second := parseCookieURL(t, "https://second.test/path")
	third := parseCookieURL(t, "https://third.invalid/path")
	jar.SetCookies(first, []*http.Cookie{
		{Name: "oversized", Value: "value", Path: "/"},
		{Name: "one", Value: "1", Path: "/"},
		{Name: "two", Value: "2", Path: "/"},
	})
	if got := cookieValueMap(jar.Cookies(first)); !reflect.DeepEqual(got, map[string]string{"one": "1"}) {
		t.Fatalf("first domain Cookies = %v", got)
	}
	jar.SetCookies(first, []*http.Cookie{{Name: "one", Value: "new", Path: "/"}})
	if got := cookieValueMap(jar.Cookies(first))["one"]; got != "new" {
		t.Fatalf("overwrite at per-domain limit = %q", got)
	}
	jar.SetCookies(second, []*http.Cookie{{Name: "two", Value: "2", Path: "/"}})
	jar.SetCookies(third, []*http.Cookie{{Name: "three", Value: "3", Path: "/"}})
	if got := cookieValueMap(jar.Cookies(third)); len(got) != 0 {
		t.Fatalf("Cookies beyond total limit = %v", got)
	}
	jar.SetCookies(first, []*http.Cookie{{Name: "one", Path: "/", MaxAge: -1}})
	jar.SetCookies(third, []*http.Cookie{{Name: "three", Value: "3", Path: "/"}})
	if got := cookieValueMap(jar.Cookies(third))["three"]; got != "3" {
		t.Fatalf("Cookie after deletion freed capacity = %q", got)
	}
}

func parseCookieURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", rawURL, err)
	}
	return parsed
}

func cookieValueMap(cookies []*http.Cookie) map[string]string {
	values := make(map[string]string, len(cookies))
	for _, cookie := range cookies {
		values[cookie.Name] = cookie.Value
	}
	return values
}
