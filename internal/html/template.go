package html

import (
	"bytes"
	_ "embed"
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

// Vue UI files (extracted from inverter-dashboard-vue release)

//go:embed vue-ui/index.html
var vueIndexHTML []byte

// GetVueUIHTML returns Vue UI index.html if available
func GetVueUIHTML() ([]byte, bool) {
	if len(vueIndexHTML) == 0 || bytes.Contains(vueIndexHTML, []byte("Vue UI not")) || bytes.Contains(vueIndexHTML, []byte("Dev placeholder")) {
		return nil, false
	}
	return vueIndexHTML, true
}

// HasVueUI returns true if Vue UI is embedded
func HasVueUI() bool {
	return len(vueIndexHTML) > 0
}

// GetDashboardHTML returns combined HTML with embedded CSS and JS
func GetDashboardHTML() string {
	html := strings.Replace(dashboardHTML, "<!--CSS_PLACEHOLDER-->", dashboardCSS, 1)
	html = strings.Replace(html, "<!--HTML_CONTENT-->", dashboardBodyHTML, 1)
	html = strings.Replace(html, "<!--JS_PLACEHOLDER-->", dashboardJS, 1)
	return html
}
