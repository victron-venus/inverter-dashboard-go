package html

import (
	"io"
	"strings"
	"testing"
)

func TestVueUIEmbedded(t *testing.T) {
	b, ok := GetVueUIHTML()
	if !ok || len(b) == 0 {
		t.Fatal("Vue UI index.html not embedded")
	}
	fsys, err := VueAssetsFS()
	if err != nil {
		t.Fatalf("VueAssetsFS: %v", err)
	}
	html := string(b)
	const marker = `src="/assets/`
	i := strings.Index(html, marker)
	if i < 0 {
		t.Fatal("no /assets/ script in index.html")
	}
	rest := html[i+len(marker):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		t.Fatal("malformed script src")
	}
	indexJS := rest[:j]
	f, err := fsys.Open(indexJS)
	if err != nil {
		t.Fatalf("open asset %s: %v", indexJS, err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	if err != nil || len(data) < 1000 {
		t.Fatalf("asset too small: %v len=%d", err, len(data))
	}
	if !strings.Contains(string(data), "acknowledge_all_notifications") {
		t.Fatal("embedded SPA missing acknowledge_all_notifications (banner ack)")
	}
	// prodY regression: DailyStats must bind produced_yesterday (PR #99 left it undeclared).
	if !strings.Contains(string(data), "produced_yesterday") {
		t.Fatal("embedded SPA missing produced_yesterday (DailyStats prodY)")
	}
}
