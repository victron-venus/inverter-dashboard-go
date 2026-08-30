package websocket

import (
	"sync"
	"testing"
)

func TestGetConnectedCount_Empty(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	if got := GetConnectedCount(); got != 0 {
		t.Errorf("GetConnectedCount() = %d, want 0", got)
	}
}

func TestSetAndGetLatestVersion(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	SetLatestVersion("v1.2.3")
	if got := getLatestVersion(); got != "v1.2.3" {
		t.Errorf("getLatestVersion() = %q, want %q", got, "v1.2.3")
	}

	SetLatestVersion("v2.0.0")
	if got := getLatestVersion(); got != "v2.0.0" {
		t.Errorf("getLatestVersion() = %q, want %q", got, "v2.0.0")
	}
}

func TestGetLatestVersion_Concurrent(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(v string) {
			defer wg.Done()
			SetLatestVersion(v)
		}("v" + string(rune('0'+i)))
	}
	wg.Wait()
	// No panic = pass
}
