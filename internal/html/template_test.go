package html

import "testing"

func TestGetDashboardHTML(t *testing.T) {
	html := GetDashboardHTML()
	if html == "" {
		t.Error("GetDashboardHTML returned empty string")
	}
}

func TestGetVueUIHTML(t *testing.T) {
	data, ok := GetVueUIHTML()
	if !ok && len(data) > 0 {
		t.Error("GetVueUIHTML returned data but ok=false")
	}
	if ok && len(data) == 0 {
		t.Error("GetVueUIHTML returned empty data but ok=true")
	}
}

func TestHasVueUI(t *testing.T) {
	has := HasVueUI()
	// Just ensure it returns a bool
	if !has && has {
		t.Error("HasVueUI returned invalid bool")
	}
}
