// Package httpendpoint validates operator-configured HTTP API destinations.
package httpendpoint

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Normalize accepts absolute HTTP(S) endpoints, including LAN and loopback
// hosts, without credentials or ambiguous URL routing. Errors never include
// the input because it may contain secrets.
func Normalize(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.Opaque != "" {
		return "", fmt.Errorf("URL must be an absolute HTTP(S) URL")
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(raw, "#") {
		return "", fmt.Errorf("URL must not contain userinfo, a query, or a fragment")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("URL has an invalid port")
		}
	} else if strings.HasSuffix(u.Host, ":") {
		return "", fmt.Errorf("URL has an invalid port")
	}
	return strings.TrimRight(u.String(), "/"), nil
}
