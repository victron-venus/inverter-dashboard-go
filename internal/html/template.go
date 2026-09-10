package html

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// Go embedded dashboard files (original Go implementation)

//go:embed templates/dashboard.html
var dashboardHTML string

//go:embed templates/dashboard_body.html
var dashboardBodyHTML string

//go:embed static/css/dashboard.css
var dashboardCSS string

//go:embed static/js/dashboard.js
var dashboardJS string

// Vue UI (index + hashed assets) — single-binary Docker/deploy.
//
//go:embed all:vue-ui
var vueUIFS embed.FS

// GetVueUIHTML returns Vue UI index.html if available
func GetVueUIHTML() ([]byte, bool) {
	b, err := vueUIFS.ReadFile("vue-ui/index.html")
	if err != nil || len(b) == 0 || bytes.Contains(b, []byte("Vue UI not")) || bytes.Contains(b, []byte("Dev placeholder")) {
		return nil, false
	}
	return b, true
}

// HasVueUI returns true if Vue UI is embedded
func HasVueUI() bool {
	_, ok := GetVueUIHTML()
	return ok
}

// VueAssetsFS returns an http.FileSystem rooted at vue-ui/assets for /assets/*.
func VueAssetsFS() (http.FileSystem, error) {
	sub, err := fs.Sub(vueUIFS, "vue-ui/assets")
	if err != nil {
		return nil, err
	}
	return http.FS(sub), nil
}

// GetDashboardHTML returns combined HTML with embedded CSS and JS
func GetDashboardHTML() string {
	html := strings.Replace(dashboardHTML, "<!--CSS_PLACEHOLDER-->", dashboardCSS, 1)
	html = strings.Replace(html, "<!--HTML_CONTENT-->", dashboardBodyHTML, 1)
	html = strings.Replace(html, "<!--JS_PLACEHOLDER-->", dashboardJS, 1)
	return html
}
