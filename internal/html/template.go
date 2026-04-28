package html

import (
	_ "embed"
	"strings"
)

//go:embed templates/dashboard.html
var dashboardHTML string

//go:embed templates/dashboard_body.html
var dashboardBodyHTML string

//go:embed static/css/dashboard.css
var dashboardCSS string

//go:embed static/js/dashboard.js
var dashboardJS string

// GetDashboardHTML returns combined HTML with embedded CSS and JS
func GetDashboardHTML() string {
	// Replace placeholders with actual CSS, HTML body, and JS
	html := strings.Replace(dashboardHTML, "<!--CSS_PLACEHOLDER-->", dashboardCSS, 1)
	html = strings.Replace(html, "<!--HTML_CONTENT-->", dashboardBodyHTML, 1)
	html = strings.Replace(html, "<!--JS_PLACEHOLDER-->", dashboardJS, 1)
	return html
}
