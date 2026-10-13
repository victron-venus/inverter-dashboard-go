package auth

import (
	"net/http"
	"testing"
)

// FuzzSameOriginPolicy checks authority separation without serving HTTP. The
// forwarded host is untrusted even when it happens to match an attacking origin.
func FuzzSameOriginPolicy(f *testing.F) {
	for _, seed := range [][3]string{
		{"https://dashboard.invalid", "dashboard.invalid", "same-origin"},
		{"http://dashboard.invalid:80", "dashboard.invalid", "none"},
		{"https://[::1]:443", "[::1]", ""},
		{"https://attacker.invalid", "dashboard.invalid", "cross-site"},
		{"null", "dashboard.invalid", "same-site"},
		{"", "", ""},
	} {
		f.Add(seed[0], seed[1], seed[2])
	}
	f.Fuzz(func(t *testing.T, origin, host, site string) {
		if len(origin)+len(host)+len(site) > 8192 {
			t.Skip()
		}
		r := &http.Request{Host: host, Header: make(http.Header)}
		r.Header.Set("Origin", origin)
		r.Header.Set("Sec-Fetch-Site", site)
		accepted := SameOrigin(r)
		if accepted && site != "" && site != "same-origin" && site != "none" {
			t.Fatal("cross-site metadata bypassed the origin policy")
		}
		r.Header.Set("X-Forwarded-Host", origin)
		r.Header.Set("Forwarded", "host="+origin)
		if SameOrigin(r) != accepted {
			t.Fatal("untrusted forwarded metadata changed authority")
		}
		r.Header.Add("Origin", origin)
		if SameOrigin(r) {
			t.Fatal("multiple Origin values were accepted")
		}
		r.Header.Del("Origin")
		if SameOrigin(r) != (site == "" || site == "same-origin" || site == "none") {
			t.Fatal("native-client origin omission changed the fetch-site policy")
		}
	})
}
