package auth

import (
	"net/http"
	"net/url"
	"strings"
)

// SameOrigin accepts browser requests only for the ingress-preserved Host.
// Native clients may omit Origin; all callers retain their normal authentication.
// Forwarded headers are intentionally not an independent source of authority.
func SameOrigin(r *http.Request) bool {
	site := r.Header.Get("Sec-Fetch-Site")
	if site != "" && site != "same-origin" && site != "none" {
		return false
	}
	origins := r.Header.Values("Origin")
	if len(origins) == 0 {
		return true
	}
	if len(origins) != 1 {
		return false
	}
	origin, err := url.Parse(origins[0])
	if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.User != nil || origin.Host == "" || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return false
	}
	target, err := url.Parse(origin.Scheme + "://" + r.Host)
	if err != nil || target.User != nil || target.Host == "" || target.Path != "" || target.RawQuery != "" || target.Fragment != "" {
		return false
	}
	return strings.EqualFold(origin.Hostname(), target.Hostname()) && effectivePort(origin) == effectivePort(target)
}
func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}
