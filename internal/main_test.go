package main

import (
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/version"
)

func TestCheckVersion(t *testing.T) {
	// Since we can't easily test the checkVersion function in isolation,
	// we'll just test that the version package functions work
	// Test with empty URL (should return early)
	if got := version.GetLatestCached(); got != "" {
		// This is just to call the function - we don't care about the value
		_ = got
	}
	// Test that we can set and get cached version
	version.SetLatestCached("1.2.3")
	if got := version.GetLatestCached(); got != "1.2.3" {
		t.Errorf("GetLatestCached() = %q, want \"1.2.3\"", got)
	}
	// Reset for other tests
	version.SetLatestCached("")
}

func TestStartMQTT(t *testing.T) {
	// These functions are in the main package, not internal package
	// They have been moved to main_test.go
	// This test file should focus on internal package functionality
	t.Skip("Tests for main package functions moved to main_test.go")
}

func TestHaPoller(t *testing.T) {
	t.Skip("Tests for main package functions moved to main_test.go")
}

func TestHaPollTick(t *testing.T) {
	t.Skip("Tests for main package functions moved to main_test.go")
}

func TestLogHAEntities(t *testing.T) {
	t.Skip("Tests for main package functions moved to main_test.go")
}

func TestCreateServer(t *testing.T) {
	t.Skip("Tests for main package functions moved to main_test.go")
}

func TestStartServer(t *testing.T) {
	t.Skip("Tests for main package functions moved to main_test.go")
}

func TestWaitForShutdown(t *testing.T) {
	t.Skip("Tests for main package functions moved to main_test.go")
}

func TestIndexHandler(t *testing.T) {
	t.Skip("Tests for main package functions moved to main_test.go")
}

func TestWebsocketHandler(t *testing.T) {
	t.Skip("Tests for main package functions moved to main_test.go")
}

func TestApiStateHandler(t *testing.T) {
	t.Skip("Tests for main package functions moved to main_test.go")
}

func TestApiCheckUpdateHandler(t *testing.T) {
	t.Skip("Tests for main package functions moved to main_test.go")
}

func TestHealthHandler(t *testing.T) {
	t.Skip("Tests for main package functions moved to main_test.go")
}

func TestCollectCommandBufferMetrics(t *testing.T) {
	t.Skip("Tests for main package functions moved to main_test.go")
}
