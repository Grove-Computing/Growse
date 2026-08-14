package network

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/publicsuffix"
)

const (
	defaultMaxCookies          = 3000
	defaultMaxCookiesPerDomain = 180
	defaultMaxCookieBytes      = 4096
)

type cookieKey struct {
	domain string
	path   string
	name   string
}

type cookiePolicy struct {
	key      cookieKey
	value    string
	hostOnly bool
	secure   bool
	httpOnly bool
	sameSite http.SameSite
	expires  time.Time
}

type policyCookieJar struct {
	jar      http.CookieJar
	mu       sync.Mutex
	policies map[cookieKey]cookiePolicy
	limits   cookieLimits
}

type cookieLimits struct {
	maxCookies          int
	maxCookiesPerDomain int
	maxCookieBytes      int
}

func newPolicyCookieJar() *policyCookieJar {
	return newPolicyCookieJarWithLimits(cookieLimits{
		maxCookies: defaultMaxCookies, maxCookiesPerDomain: defaultMaxCookiesPerDomain, maxCookieBytes: defaultMaxCookieBytes,
	})
}

func newPolicyCookieJarWithLimits(limits cookieLimits) *policyCookieJar {
	jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	return &policyCookieJar{jar: jar, policies: make(map[cookieKey]cookiePolicy), limits: limits}
}

func (jar *policyCookieJar) Cookies(target *url.URL) []*http.Cookie {
	if jar == nil || jar.jar == nil {
		return nil
	}
	return jar.jar.Cookies(target)
}

func (jar *policyCookieJar) SetCookies(target *url.URL, cookies []*http.Cookie) {
	if jar == nil || jar.jar == nil || target == nil {
		return
	}
	accepted := make([]*http.Cookie, 0, len(cookies))
	jar.mu.Lock()
	jar.removeExpiredLocked(time.Now())
	for _, cookie := range cookies {
		if cookie == nil || cookie.Name == "" || cookie.SameSite == http.SameSiteNoneMode && !cookie.Secure {
			continue
		}
		domain := strings.ToLower(strings.TrimPrefix(cookie.Domain, "."))
		hostOnly := domain == ""
		if hostOnly {
			domain = strings.ToLower(target.Hostname())
		}
		path := cookie.Path
		if path == "" || path[0] != '/' {
			path = defaultCookiePath(target.Path)
		}
		key := cookieKey{domain: domain, path: path, name: cookie.Name}
		if cookie.MaxAge < 0 || !cookie.Expires.IsZero() && cookie.Expires.Before(time.Now()) {
			delete(jar.policies, key)
		} else {
			if jar.limits.maxCookieBytes > 0 && len(cookie.Name)+len(cookie.Value) > jar.limits.maxCookieBytes {
				continue
			}
			if _, exists := jar.policies[key]; !exists {
				if jar.limits.maxCookies > 0 && len(jar.policies) >= jar.limits.maxCookies ||
					jar.limits.maxCookiesPerDomain > 0 && jar.domainCountLocked(domain) >= jar.limits.maxCookiesPerDomain {
					continue
				}
			}
			jar.policies[key] = cookiePolicy{
				key: key, value: cookie.Value, hostOnly: hostOnly, secure: cookie.Secure,
				httpOnly: cookie.HttpOnly, sameSite: cookie.SameSite, expires: cookie.Expires,
			}
		}
		accepted = append(accepted, cookie)
	}
	jar.mu.Unlock()
	jar.jar.SetCookies(target, accepted)
}

func (jar *policyCookieJar) domainCountLocked(domain string) int {
	count := 0
	for key := range jar.policies {
		if key.domain == domain {
			count++
		}
	}
	return count
}

func (jar *policyCookieJar) removeExpiredLocked(now time.Time) {
	for key, policy := range jar.policies {
		if !policy.expires.IsZero() && policy.expires.Before(now) {
			delete(jar.policies, key)
		}
	}
}

func (jar *policyCookieJar) cookiesForRequest(target, siteURL *url.URL, kind RequestKind, method string) []*http.Cookie {
	cookies := jar.Cookies(target)
	if siteURL == nil || sameSite(target, siteURL) {
		return cookies
	}
	result := make([]*http.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		policy, ok := jar.policyFor(target, cookie)
		if !ok {
			result = append(result, cookie)
			continue
		}
		switch policy.sameSite {
		case http.SameSiteStrictMode:
			continue
		case http.SameSiteNoneMode:
			result = append(result, cookie)
		default:
			if kind == RequestNavigation && (method == http.MethodGet || method == http.MethodHead) {
				result = append(result, cookie)
			}
		}
	}
	return result
}

func (jar *policyCookieJar) visibleCookies(target *url.URL) []*http.Cookie {
	result := make([]*http.Cookie, 0)
	for _, cookie := range jar.Cookies(target) {
		if policy, ok := jar.policyFor(target, cookie); !ok || !policy.httpOnly {
			result = append(result, cookie)
		}
	}
	return result
}

func (jar *policyCookieJar) policyFor(target *url.URL, cookie *http.Cookie) (cookiePolicy, bool) {
	jar.mu.Lock()
	defer jar.mu.Unlock()
	var best cookiePolicy
	found := false
	for key, policy := range jar.policies {
		if key.name != cookie.Name || policy.value != cookie.Value || !cookieDomainMatches(policy, target.Hostname()) ||
			!cookiePathMatches(key.path, target.Path) || policy.secure && target.Scheme != "https" {
			continue
		}
		if !policy.expires.IsZero() && policy.expires.Before(time.Now()) {
			delete(jar.policies, key)
			continue
		}
		if !found || len(key.path) > len(best.key.path) {
			best = policy
			found = true
		}
	}
	return best, found
}

func cookieDomainMatches(cookie cookiePolicy, host string) bool {
	host = strings.ToLower(host)
	if cookie.hostOnly {
		return host == cookie.key.domain
	}
	return host == cookie.key.domain || strings.HasSuffix(host, "."+cookie.key.domain)
}

func cookiePathMatches(cookiePath, requestPath string) bool {
	if requestPath == "" {
		requestPath = "/"
	}
	return requestPath == cookiePath || strings.HasPrefix(requestPath, cookiePath) &&
		(strings.HasSuffix(cookiePath, "/") || len(requestPath) > len(cookiePath) && requestPath[len(cookiePath)] == '/')
}

func defaultCookiePath(path string) string {
	if path == "" || path[0] != '/' || strings.Count(path, "/") == 1 {
		return "/"
	}
	return path[:strings.LastIndex(path, "/")]
}

func sameSite(target, siteURL *url.URL) bool {
	if target == nil || siteURL == nil || !strings.EqualFold(target.Scheme, siteURL.Scheme) {
		return false
	}
	return registrableDomain(target.Hostname()) == registrableDomain(siteURL.Hostname())
}

func registrableDomain(host string) string {
	host = strings.ToLower(host)
	domain, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return host
	}
	return domain
}
