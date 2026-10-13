package httpendpoint

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// FuzzNormalizeEndpoint exercises the same boundary used by configured gateway
// and Home Assistant destinations; it never resolves or connects to a host.
func FuzzNormalizeEndpoint(f *testing.F) {
	for _, seed := range []string{
		"http://cerbo.local:8080/v1/", " https://[::1]:443/api/ ",
		"https://host.invalid/path%2Fsegment", "http://user:secret@host.invalid/",
		"http://host.invalid:65536/", "http://host.invalid/?token=secret",
		"http://host.invalid/#fragment", "http://host.invalid:", "//host.invalid", "",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 4096 {
			t.Skip()
		}
		normalized, err := Normalize(raw)
		if err != nil {
			if normalized != "" {
				t.Fatal("rejected endpoint returned a usable destination")
			}
			return
		}
		u, err := url.Parse(normalized)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
			t.Fatalf("accepted endpoint has no absolute HTTP destination: %q", normalized)
		}
		if u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(normalized, "#") || u.Opaque != "" {
			t.Fatalf("accepted endpoint retained forbidden routing or credentials: %q", normalized)
		}
		if port := u.Port(); port != "" {
			n, err := strconv.Atoi(port)
			if err != nil || n < 1 || n > 65535 {
				t.Fatalf("accepted out-of-range port: %q", port)
			}
		}
		again, err := Normalize(normalized)
		if err != nil || again != normalized {
			t.Fatalf("normalization is unstable: %q -> %q (%v)", normalized, again, err)
		}
	})
}
