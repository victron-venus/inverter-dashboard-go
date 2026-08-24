package version

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const updateDownloadTimeout = 60 * time.Second

// releaseBaseURL is the GitHub releases download prefix; overridden in tests.
var releaseBaseURL = "https://github.com/victron-venus/inverter-dashboard-go/releases/latest/download"

// resolveExecPath returns the real path of the current executable; overridden in tests.
var resolveExecPath = func() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(p)
}

// AssetName maps the running platform to its release asset name
// (matrix in .github/workflows/release.yml).
func AssetName() string {
	switch {
	case runtime.GOOS == "linux" && runtime.GOARCH == "amd64":
		return "inverter-dashboard-linux-amd64"
	case runtime.GOOS == "linux" && runtime.GOARCH == "arm64":
		return "inverter-dashboard-linux-arm64"
	case runtime.GOOS == "linux" && runtime.GOARCH == "arm":
		return "inverter-dashboard-raspberry-pi3"
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
		return "inverter-dashboard-macos-silicon"
	case runtime.GOOS == "darwin" && runtime.GOARCH == "amd64":
		return "inverter-dashboard-macos-intel"
	default:
		return ""
	}
}

// SelfUpdate downloads the latest release binary for this platform and
// atomically replaces the current executable. The caller is responsible for
// restarting the process (exit 0 + container restart policy) so the new
// binary takes effect.
func SelfUpdate() error {
	asset := AssetName()
	if asset == "" {
		return fmt.Errorf("no release asset for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	url := releaseBaseURL + "/" + asset

	ctx, cancel := context.WithTimeout(context.Background(), updateDownloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download update: %w", err)
	}
	defer func() { _ = resp.Body.Close() }() // best-effort close; read errors already handled above

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code downloading %s: %d", asset, resp.StatusCode)
	}

	execPath, err := resolveExecPath()
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	tmpPath := execPath + ".new"
	tmp, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	written, err := io.Copy(tmp, resp.Body)
	closeErr := tmp.Close()
	if err != nil || closeErr != nil {
		_ = os.Remove(tmpPath)
		if err != nil {
			return fmt.Errorf("failed to write update: %w", err)
		}
		return fmt.Errorf("failed to close temp file: %w", closeErr)
	}
	if written == 0 {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("downloaded empty file")
	}

	// Atomic swap: rename over the running binary (same filesystem).
	if err := os.Rename(tmpPath, execPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to replace executable: %w", err)
	}

	return nil
}
