package websocket

import (
	"sync"

	"github.com/gorilla/websocket"
)

// resetClientsForTest empties the package-level clients map and version cache.
// Must be called between tests that mutate either.
func resetClientsForTest() {
	clientsMu.Lock()
	clients = make(map[*websocket.Conn]bool)
	clientsMu.Unlock()

	latestMu.Lock()
	latestVersion = ""
	latestMu.Unlock()
}

var _ = sync.Mutex{} // pull in sync when other helpers land
