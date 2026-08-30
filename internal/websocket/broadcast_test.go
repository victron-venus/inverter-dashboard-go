package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/victron-venus/inverter-dashboard-go/internal/homeassistant"
	"github.com/victron-venus/inverter-dashboard-go/internal/state"
	"github.com/victron-venus/inverter-dashboard-go/internal/websocket/mockmqtt"
)

// fakeConn simulates a websocket.Conn for BroadcastState tests.
// Embeds the real websocket.Conn via a server upgrade to satisfy the
// concrete *websocket.Conn type used by the broadcast loop.
type fakeConn struct {
	conn *websocket.Conn
	ws   *httptest.Server
	msgs chan []byte
}

func newFakeConn(t *testing.T) *fakeConn {
	t.Helper()
	upgrader := websocket.Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}
	done := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		done <- c
		// Block reading so connection stays open
		for {
			if _, _, err := c.NextReader(); err != nil {
				return
			}
		}
	}))

	// Connect client side
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.DialContext(t.Context(), wsURL, nil)
	if err != nil {
		srv.Close()
		t.Fatalf("dial: %v", err)
	}

	// Wait for server-side conn
	var serverConn *websocket.Conn
	select {
	case serverConn = <-done:
	case <-time.After(2 * time.Second):
		_ = conn.Close()
		srv.Close()
		t.Fatal("server upgrade timeout")
	}

	fc := &fakeConn{
		conn: serverConn,
		ws:   srv,
		msgs: make(chan []byte, 16),
	}
	go fc.readLoop(conn)
	return fc
}

func (f *fakeConn) readLoop(c *websocket.Conn) {
	defer close(f.msgs)
	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			return
		}
		cp := make([]byte, len(data))
		copy(cp, data)
		select {
		case f.msgs <- cp:
		default:
		}
	}
}

func (f *fakeConn) addToBroadcast() {
	clientsMu.Lock()
	clients[f.conn] = true
	clientsMu.Unlock()
}

func (f *fakeConn) removeFromBroadcast() {
	clientsMu.Lock()
	delete(clients, f.conn)
	clientsMu.Unlock()
}

func (f *fakeConn) cleanup() {
	clientsMu.Lock()
	delete(clients, f.conn)
	clientsMu.Unlock()
	_ = f.conn.Close()
	f.ws.Close()
}

func TestBroadcastState_NoClients(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	mc := mockmqtt.NewClient()
	mc.SetState(&state.State{SolarTotal: 1000})
	err := BroadcastState(mc, nil, homeassistant.Overlay{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBroadcastState_SendsToClient(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	fc := newFakeConn(t)
	defer fc.cleanup()

	fc.addToBroadcast()

	mc := mockmqtt.NewClient()
	mc.SetState(&state.State{SolarTotal: 1500, BatterySOC: 80})
	if err := BroadcastState(mc, nil, homeassistant.Overlay{}); err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-fc.msgs:
		var payload map[string]interface{}
		if err := json.Unmarshal(msg, &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if payload["solar_total"] != 1500.0 {
			t.Errorf("solar_total=%v, want 1500", payload["solar_total"])
		}
		if payload["battery_soc"] != 80.0 {
			t.Errorf("battery_soc=%v, want 80", payload["battery_soc"])
		}
		if payload["dashboard_version"] == nil {
			t.Error("dashboard_version missing")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no message received")
	}
}

func TestBroadcastState_MultipleClients(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	fc1 := newFakeConn(t)
	defer fc1.cleanup()
	fc2 := newFakeConn(t)
	defer fc2.cleanup()
	fc3 := newFakeConn(t)
	defer fc3.cleanup()

	fc1.addToBroadcast()
	fc2.addToBroadcast()
	fc3.addToBroadcast()

	mc := mockmqtt.NewClient()
	if err := BroadcastState(mc, nil, homeassistant.Overlay{}); err != nil {
		t.Fatal(err)
	}

	for i, fc := range []*fakeConn{fc1, fc2, fc3} {
		select {
		case <-fc.msgs:
			// got message
		case <-time.After(2 * time.Second):
			t.Errorf("client %d: no message", i)
		}
	}
}

func TestBroadcastState_RemovesDeadClient(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	fc := newFakeConn(t)
	fc.addToBroadcast()

	// Force-close the server-side conn; broadcast will fail to write
	_ = fc.conn.Close()
	fc.ws.Close()

	mc := mockmqtt.NewClient()
	if err := BroadcastState(mc, nil, homeassistant.Overlay{}); err != nil {
		t.Fatal(err)
	}

	// Client should be removed from the map
	clientsMu.RLock()
	_, present := clients[fc.conn]
	clientsMu.RUnlock()
	if present {
		t.Error("dead client not removed from broadcast map")
	}
}

func TestGetConnectedCount_WithClients(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	if got := GetConnectedCount(); got != 0 {
		t.Fatalf("initial count = %d, want 0", got)
	}

	fc := newFakeConn(t)
	defer fc.cleanup()
	fc.addToBroadcast()

	if got := GetConnectedCount(); got != 1 {
		t.Errorf("after add: count = %d, want 1", got)
	}

	fc.removeFromBroadcast()
	if got := GetConnectedCount(); got != 0 {
		t.Errorf("after remove: count = %d, want 0", got)
	}
}

func TestCloseAll(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	fc := newFakeConn(t)
	defer fc.cleanup()
	fc.addToBroadcast()

	if GetConnectedCount() != 1 {
		t.Fatal("setup failed")
	}

	CloseAll()

	if GetConnectedCount() != 0 {
		t.Errorf("after CloseAll: count = %d, want 0", GetConnectedCount())
	}
}
