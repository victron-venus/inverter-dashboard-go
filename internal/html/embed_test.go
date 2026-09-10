package html

import (
	"io"
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
	f, err := fsys.Open("index-CmYnOZ-q.js")
	if err != nil {
		t.Fatalf("open asset: %v", err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	if err != nil || len(data) < 1000 {
		t.Fatalf("asset too small: %v len=%d", err, len(data))
	}
}
