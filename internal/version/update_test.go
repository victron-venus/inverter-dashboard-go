package version

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAssetName(t *testing.T) {
	got := AssetName()
	want := ""
	switch {
	case runtime.GOOS == "linux" && runtime.GOARCH == "amd64":
		want = "inverter-dashboard-linux-amd64"
	case runtime.GOOS == "linux" && runtime.GOARCH == "arm64":
		want = "inverter-dashboard-linux-arm64"
	case runtime.GOOS == "linux" && runtime.GOARCH == "arm":
		want = "inverter-dashboard-raspberry-pi3"
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
		want = "inverter-dashboard-macos-silicon"
	case runtime.GOOS == "darwin" && runtime.GOARCH == "amd64":
		want = "inverter-dashboard-macos-intel"
	}
	if got != want {
		t.Errorf("AssetName() = %q, want %q (GOOS=%s GOARCH=%s)", got, want, runtime.GOOS, runtime.GOARCH)
	}
}

func TestSelfUpdateReplacesBinary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte("NEWBINARY")); err != nil {
			t.Errorf("write failed: %v", err)
		}
	}))
	defer srv.Close()

	oldURL, oldResolve := releaseBaseURL, resolveExecPath
	releaseBaseURL = srv.URL
	target := filepath.Join(t.TempDir(), "app")
	if err := os.WriteFile(target, []byte("OLDBINARY"), 0o755); err != nil {
		t.Fatal(err)
	}
	resolveExecPath = func() (string, error) { return target, nil }
	defer func() { releaseBaseURL, resolveExecPath = oldURL, oldResolve }()

	if err := SelfUpdate(); err != nil {
		t.Fatalf("SelfUpdate() error = %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "NEWBINARY" {
		t.Errorf("binary not replaced: got %q", got)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("perm = %v, want 0755", info.Mode().Perm())
	}
	// No leftover temp file.
	if _, err := os.Stat(target + ".new"); !os.IsNotExist(err) {
		t.Errorf("temp file left behind")
	}
}

func TestSelfUpdateHTTPErrorKeepsOldBinary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	oldURL, oldResolve := releaseBaseURL, resolveExecPath
	releaseBaseURL = srv.URL
	target := filepath.Join(t.TempDir(), "app")
	if err := os.WriteFile(target, []byte("OLDBINARY"), 0o755); err != nil {
		t.Fatal(err)
	}
	resolveExecPath = func() (string, error) { return target, nil }
	defer func() { releaseBaseURL, resolveExecPath = oldURL, oldResolve }()

	err := SelfUpdate()
	if err == nil {
		t.Fatal("expected error on 404")
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil || string(got) != "OLDBINARY" {
		t.Errorf("old binary must survive failed update: got %q err %v", got, readErr)
	}
}
