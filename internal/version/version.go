package version

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultTimeout = 10 * time.Second
	updateTimeout  = 30 * time.Second
	versionFile    = "VERSION"
)

var (
	version     string
	latestCache string
	cacheMu     sync.RWMutex
)

func init() {
	version = readVersionFile()
}

// GetCurrent returns the current version
func GetCurrent() string {
	if version != "" {
		return version
	}
	return "dev"
}

// readVersionFile reads version from VERSION file
func readVersionFile() string {
	execPath, err := os.Executable()
	if err != nil {
		log.Printf("Failed to get executable path: %v", err)
		return "dev"
	}

	versionPath := filepath.Join(filepath.Dir(execPath), versionFile)

	file, err := os.Open(versionPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Failed to open VERSION file: %v", err)
		}
		return "dev"
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Failed to read VERSION file: %v", err)
	}

	return "dev"
}

// CheckLatest checks GitHub for the latest version
func CheckLatest(rawURL string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	url := fmt.Sprintf("%s/%s", rawURL, versionFile)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	latest, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	latestStr := strings.TrimSpace(string(latest))
	log.Printf("Latest version: %s, current: %s", latestStr, GetCurrent())

	return latestStr, nil
}

// GetLatestCached returns the cached latest version
func GetLatestCached() string {
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	return latestCache
}

// SetLatestCached caches the latest version
func SetLatestCached(version string) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	latestCache = version
}
