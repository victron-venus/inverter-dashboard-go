package html

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
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

func TestVueUISourceReceipt(t *testing.T) {
	data, err := os.ReadFile("vue-ui-source.json")
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Commit string            `json:"commit"`
		SHA256 map[string]string `json:"sha256"`
	}
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	if len(receipt.Commit) != 40 || len(receipt.SHA256) == 0 {
		t.Fatal("source receipt must identify the exact source commit and embedded assets")
	}
	err = fs.WalkDir(vueUIFS, "vue-ui", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := vueUIFS.ReadFile(path)
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(path, "vue-ui/")
		if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != receipt.SHA256[name] {
			t.Errorf("embedded asset %s does not match its source receipt", name)
		}
		delete(receipt.SHA256, name)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.SHA256) != 0 {
		t.Errorf("source receipt references absent assets: %v", receipt.SHA256)
	}
}
