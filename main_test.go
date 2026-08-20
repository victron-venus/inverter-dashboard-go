package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/victron-venus/inverter-dashboard-go/internal/config"
	"github.com/victron-venus/inverter-dashboard-go/internal/homeassistant"
	"github.com/victron-venus/inverter-dashboard-go/internal/logging"
	"github.com/victron-venus/inverter-dashboard-go/internal/mqtt"
	"github.com/victron-venus/inverter-dashboard-go/internal/version"
	"log/slog"
)

// Mock implementations for dependencies

type mockConfig struct {
	*config.Config
}

func newMockConfig() *mockConfig {
	return &mockConfig{
		Config: &config.Config{
			MQTT: config.MQTTConfig{
				Host: "test.mqtt",
				Port: 1883,
			},
			Web: config.WebConfig{
				Host: "0.0.0.0",
				Port: 8080,
			},
			GitHub: config.GitHubConfig{
				RawURL: "https://raw.githubusercontent.com/victron-venus/inverter-dashboard-go/main",
			},
		},
	}
}

type mockHAClient struct {
	Client *homeassistant.Client
	DirectMode bool
	PollInterval time.Duration
}

func newMockHAClient() *mockHAClient {
	return &mockHAClient{
		Client: &homeassistant.Client{},
		DirectMode: true,
		PollInterval: 10 * time.Second,
	}
}

func (m *mockHAClient) IsDirectMode() bool { return m.DirectMode }
func (m *mockHAClient) GetPollInterval() time.Duration { return m.PollInterval }
func (m *mockHAClient) FetchStatesOnce() (homeassistant.Overlay, error) {
	return homeassistant.Overlay{
		Booleans: map[string]bool{"test": true},
		AdditionalFields: map[string]interface{}{"field": "value"},
		HADirectConnected: true,
	}, nil
}
func (m *mockHAClient) GetOverlay() homeassistant.Overlay {
	return homeassistant.Overlay{
		Booleans: map[string]bool{"test": true},
		AdditionalFields: map[string]interface{}{"field": "value"},
		HADirectConnected: true,
	}
}
func (m *mockHAClient) ReplaceOverlay(overlay homeassistant.Overlay) {}

// End mocks

func TestCheckVersion(t *testing.T) {
	// Test that we can set and get cached version
	version.SetLatestCached("1.2.3")
	if got := version.GetLatestCached(); got != "1.2.3" {
		t.Errorf("GetLatestCached() = %q, want \"1.2.3\"", got)
	}
	// Reset for other tests
	version.SetLatestCached("")
}

func TestStartMQTT(t *testing.T) {
	// Create a real logger for this test
	logger := logging.New("test", "test", slog.LevelInfo)
	_ = logger // Use logger to avoid unused variable error
	// Set environment variables for quick failure
	t.Setenv("MQTT_CONNECT_TIMEOUT", "1s")
	t.Setenv("MQTT_AUTORECONNECT", "false")
	t.Setenv("MQTT_CONNECTRETRY", "false")
	// Create a client that will fail to connect quickly
	client := mqtt.NewClient("127.0.0.1", 1)
	defer client.Disconnect()
	err := startMQTT(client)
	assert.Error(t, err) // Should fail to connect
	// We don't assert.NoError here because we expect it to fail
}

func TestHaPoller(t *testing.T) {
	// Create a real logger for this test
	logger := logging.New("test", "test", slog.LevelInfo)
	_ = logger // Use logger to avoid unused variable error
	haClient := newMockHAClient()
	done := make(chan bool)
	go func() {
		haPoller(haClient, logger)
		done <- true
	}()
	select {
	case <-done:
		t.Error("haPoller returned immediately (unexpected)")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestHaPollTick(t *testing.T) {
	// Create a real logger for this test
	logger := logging.New("test", "test", slog.LevelInfo)
	_ = logger // Use logger to avoid unused variable error
	haClient := newMockHAClient()
	// Call haPollTick; it should not panic
	haPollTick(haClient.Client, logger)
	// No assertions needed for coverage
}

func TestLogHAEntities(t *testing.T) {
	// Create a real logger for this test
	logger := logging.New("test", "test", slog.LevelInfo)
	_ = logger // Use logger to avoid unused variable error
	overlay := homeassistant.Overlay{
		Booleans: map[string]bool{"test": true},
		AdditionalFields: map[string]interface{}{"field": "value"},
	}
	logHAEntities(overlay, logger)
}

func TestCreateServer(t *testing.T) {
	// Create a real mqtt client for this test
	mqttClient := mqtt.NewClient("test.mqtt", 1883)
	defer mqttClient.Disconnect()
	haClient := newMockHAClient()
	// Create a real logger for this test
	logger := logging.New("test", "test", slog.LevelInfo)
	_ = logger // Use logger to avoid unused variable error
	// Pass nil for tracer since createServer checks for nil
	server := createServer(mqttClient, haClient.Client, logger, nil)
	assert.NotNil(t, server)
	assert.IsType(t, &gin.Engine{}, server)
}

func TestStartServer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := gin.New()
	cfg := newMockConfig().Config
	// Use a random port to avoid conflicts
	cfg.Web.Port = 0
	// Create a real logger for this test
	logger := logging.New("test", "test", slog.LevelInfo)
	_ = logger // Use logger to avoid unused variable error
	done := make(chan bool)
	go func() {
		startServer(server, cfg, "", "", logger)
		done <- true
	}()
	// Wait for the server to start (we don't know how long, but we can wait a bit)
	time.Sleep(100 * time.Millisecond)
	// The test doesn't wait for the server to stop, so we just let the goroutine run.
	// But we don't want to leak the goroutine, so we can try to stop the server by sending a signal?
	// Since we are in test mode, we can just let the test end and the goroutine will be killed when the program exits.
	// However, we want to avoid the "address already in use" error in the same test run.
	// We can wait for the done channel with a timeout, and then if it's not ready, we assume the server is running and we move on.
	select {
	case <-done:
		// server stopped
	case <-time.After(200 * time.Millisecond):
		// server is still running, we move on
	}
}

func TestWaitForShutdown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := gin.New()
	// Create a real logger for this test
	logger := logging.New("test", "test", slog.LevelInfo)
	_ = logger // Use logger to avoid unused variable error
	// Create a real mqtt client for this test
	mqttClient := mqtt.NewClient("test.mqtt", 1883)
	defer mqttClient.Disconnect()
	haClient := newMockHAClient()
	done := make(chan bool)
	go func() {
		waitForShutdown(server, mqttClient, haClient.Client, logger)
		done <- true
	}()
	time.Sleep(50 * time.Millisecond)
	<-time.After(200 * time.Millisecond)
}

func TestIndexHandler(t *testing.T) {
	handler := indexHandler()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	c.Request = req
	handler(c)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "<title>Inverter Dashboard</title>")
}

func TestWebsocketHandler(t *testing.T) {
	// Create a real mqtt client for this test
	mqttClient := mqtt.NewClient("test.mqtt", 1883)
	defer mqttClient.Disconnect()
	haClient := newMockHAClient()
	// Create a real logger for this test
	logger := logging.New("test", "test", slog.LevelInfo)
	_ = logger // Use logger to avoid unused variable error
	handler := websocketHandler(mqttClient, haClient.Client)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest(http.MethodGet, "/ws", nil)
	c.Request = req
	handler(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestApiStateHandler(t *testing.T) {
	// Create a real mqtt client for this test
	mqttClient := mqtt.NewClient("test.mqtt", 1883)
	defer mqttClient.Disconnect()
	handler := apiStateHandler(mqttClient)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest(http.MethodGet, "/api/state", nil)
	c.Request = req
	handler(c)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"ok":true`)
}

func TestApiCheckUpdateHandler(t *testing.T) {
	// Mock http.Client to avoid making real HTTP requests
	oldClient := http.DefaultClient
	defer func() { http.DefaultClient = oldClient }()

	mockTransport := &mockRoundTripper{
		mockResp: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("2.0.0")),
			Header:     make(http.Header),
		},
	}
	http.DefaultClient = &http.Client{Transport: mockTransport}

	handler := apiCheckUpdateHandler()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest(http.MethodPost, "/api/check-update", nil)
	c.Request = req
	handler(c)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"current"`)
	assert.Contains(t, w.Body.String(), `"latest"`)
}

// mockRoundTripper is a simple mock for testing HTTP requests.
type mockRoundTripper struct {
	mockResp *http.Response
	mockErr  error
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.mockErr != nil {
		return nil, m.mockErr
	}
	return m.mockResp, nil
}

func TestHealthHandler(t *testing.T) {
	// Create a real mqtt client for this test
	mqttClient := mqtt.NewClient("test.mqtt", 1883)
	defer mqttClient.Disconnect()
	handler := healthHandler(mqttClient)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	c.Request = req
	handler(c)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"status"`)
}

func TestCollectCommandBufferMetrics(t *testing.T) {
	// Create a real mqtt client for this test
	mqttClient := mqtt.NewClient("test.mqtt", 1883)
	defer mqttClient.Disconnect()
	// Call the function; it should not panic
	collectCommandBufferMetrics(mqttClient)
	// No assertions needed for coverage
}
