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

// UpdateFiles downloads and updates files from GitHub
type UpdateResult struct {
	Success bool
	Version string
	Error   error
}

func UpdateFiles(rawURL string) UpdateResult {
	files := []string{
		"main.go",
		"internal/config/config.go",
		"internal/version/version.go",
		"internal/mqtt/client.go",
		"internal/websocket/handler.go",
		"internal/homeassistant/client.go",
		"internal/html/template.go",
		"VERSION",
	}

	result := UpdateResult{
		Version: "unknown",
	}

	execPath, err := os.Executable()
	if err != nil {
		result.Error = fmt.Errorf("failed to get executable path: %w", err)
		return result
	}

	appDir := filepath.Dir(execPath)
	ctx, cancel := context.WithTimeout(context.Background(), updateTimeout)
	defer cancel()

	client := &http.Client{
		Timeout: updateTimeout,
	}

	for _, filename := range files {
		url := fmt.Sprintf("%s/%s", rawURL, filename)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			log.Printf("Failed to create request for %s: %v", filename, err)
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Failed to download %s: %v", filename, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			log.Printf("Failed to download %s: status %d", filename, resp.StatusCode)
			resp.Body.Close()
			continue
		}

		content, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("Failed to read %s: %v", filename, err)
			continue
		}

		// Write to file
		filePath := filepath.Join(appDir, filename)
		dir := filepath.Dir(filePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Printf("Failed to create directory for %s: %v", filename, err)
			continue
		}

		if err := os.WriteFile(filePath, content, 0644); err != nil {
			log.Printf("Failed to write %s: %v", filename, err)
			continue
		}

		log.Printf("Updated %s", filename)

		if filename == versionFile {
			result.Version = strings.TrimSpace(string(content))
		}
	}

	result.Success = true
	log.Printf("Updated to v%s", result.Version)
	return result
}

// ScheduleRestart schedules an application restart
func ScheduleRestart(delay time.Duration) {
	log.Printf("Scheduling restart in %v", delay)
	time.Sleep(delay)
	os.Exit(0)
}
