package websocket

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ws "github.com/gorilla/websocket"
	"github.com/victron-venus/inverter-dashboard-go/internal/homeassistant"
	"github.com/victron-venus/inverter-dashboard-go/internal/websocket/mockmqtt"
)

// A stalled peer must not hold the client registry used by /health.
func TestSlowPeerDoesNotBlockHealthClientCount(t *testing.T) {
	resetClientsForTest()
	upgraded := make(chan *ws.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&ws.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		if tcp, ok := c.UnderlyingConn().(*net.TCPConn); ok {
			_ = tcp.SetWriteBuffer(1024)
		}
		upgraded <- c
	}))
	defer server.Close()
	peer, _, err := ws.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	if tcp, ok := peer.UnderlyingConn().(*net.TCPConn); ok {
		_ = tcp.SetReadBuffer(1024)
	}
	conn := <-upgraded
	defer conn.Close()
	clientsMu.Lock()
	clients[conn] = true
	clientsMu.Unlock()
	mc := mockmqtt.NewClient()
	mc.SetConsole([]string{strings.Repeat("x", 4*1024*1024)})
	broadcastDone := make(chan struct{})
	go func() { _ = BroadcastState(mc, nil, homeassistant.Overlay{}); close(broadcastDone) }()
	// Read only the beginning of the frame, proving the server entered its write,
	// then leave the rest unread so the small TCP buffers force backpressure.
	if err := peer.UnderlyingConn().SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	var prefix [1]byte
	if _, err := peer.UnderlyingConn().Read(prefix[:]); err != nil {
		t.Fatal(err)
	}
	healthDone := make(chan struct{})
	go func() { _ = GetConnectedCount(); close(healthDone) }()
	select {
	case <-healthDone:
		// Registry reads complete even though the socket write is still blocked.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("health blocked behind a stalled peer")
	}
	select {
	case <-broadcastDone:
	case <-time.After(3 * time.Second):
		t.Fatal("write deadline did not remove the stalled peer")
	}
	select {
	case <-healthDone:
		if count := GetConnectedCount(); count != 0 {
			t.Fatalf("stalled peer was not removed: %d clients", count)
		}
	case <-time.After(time.Second):
		t.Fatal("health did not unblock")
	}
	resetClientsForTest()
}
