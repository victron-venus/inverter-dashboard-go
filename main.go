package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/victron-venus/inverter-dashboard-go/internal/auth"
	"github.com/victron-venus/inverter-dashboard-go/internal/config"
	"github.com/victron-venus/inverter-dashboard-go/internal/homeassistant"
	"github.com/victron-venus/inverter-dashboard-go/internal/html"
	"github.com/victron-venus/inverter-dashboard-go/internal/logging"
	"github.com/victron-venus/inverter-dashboard-go/internal/metrics"
	"github.com/victron-venus/inverter-dashboard-go/internal/mqtt"
	"github.com/victron-venus/inverter-dashboard-go/internal/tracing"
	"github.com/victron-venus/inverter-dashboard-go/internal/version"
	"github.com/victron-venus/inverter-dashboard-go/internal/websocket"
	"go.opentelemetry.io/otel/trace"
	"log/slog"
)

const serviceName = "inverter-dashboard-go"

var (
	// Version is set during build
	Version string = "dev"
)

func main() {
	// Command line flags - match Python exactly
	var (
		mqttHost    = flag.String("mqtt-host", "", "MQTT broker host")
		mqttPort    = flag.Int("mqtt-port", 0, "MQTT broker port")
		webPort     = flag.Int("port", 0, "Web server port")
		sslCert     = flag.String("ssl-cert", "", "SSL certificate file")
		sslKey      = flag.String("ssl-key", "", "SSL key file")
		showVersion = flag.Bool("version", false, "Show version and exit")
	)
	flag.Parse()

	// Show version and exit
	if *showVersion {
		fmt.Printf("Inverter Dashboard v%s\n", version.GetCurrent())
		os.Exit(0)
	}

	// Docker HEALTHCHECK mode: the runtime image ships no wget/curl, so the
	// container probes itself with this subcommand instead.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		port := os.Getenv("WEB_PORT")
		if port == "" {
			port = "8080"
		}
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/health", port))
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		_ = resp.Body.Close() // best-effort; status already checked
		os.Exit(0)
	}

	// Initialize OpenTelemetry tracing
	traceCfg := tracing.DefaultConfig()
	traceCfg.ServiceVersion = version.GetCurrent()
	tp, tracer, err := tracing.InitTracer(traceCfg)
	if err != nil {
		slog.Error("Warning: failed to initialize tracing", "error", err)
	} else {
		defer func() { _ = tracing.Shutdown(context.Background(), tp) }()
	}

	// Initialize structured logger
	logger := logging.New(serviceName, version.GetCurrent(), slog.LevelInfo)

	// Load configuration - Python uses environment only
	cfg, err := config.Load("")
	if err != nil {
		logger.Error(logging.DefaultContext(), "Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Override with command line flags if provided
	if *mqttHost != "" {
		cfg.MQTT.Host = *mqttHost
	}
	if *mqttPort > 0 {
		cfg.MQTT.Port = *mqttPort
	}
	if *webPort > 0 {
		cfg.Web.Port = *webPort
	}

	// Determine protocol
	proto := "http"
	if *sslCert != "" && *sslKey != "" {
		proto = "https"
	}

	// Print startup info using structured logger
	logger.Info(logging.DefaultContext(), "Starting Inverter Dashboard",
		"version", version.GetCurrent(),
		"mqtt_host", cfg.MQTT.Host,
		"mqtt_port", cfg.MQTT.Port,
		"web_proto", proto,
		"web_host", cfg.Web.Host,
		"web_port", cfg.Web.Port,
	)
	if cfg.DashboardSecret == "" {
		logger.Warn(logging.DefaultContext(), "DASHBOARD_SECRET is not set — dashboard is unprotected. "+
			"Set DASHBOARD_SECRET env var for production use.")
	}

	// Create MQTT client
	mqttClient := mqtt.NewClient(cfg.MQTT.Host, cfg.MQTT.Port)
	// Water topics (dbus-pump on the Cerbo); empty portal ID disables
	mqttClient.SetWaterConfig(cfg.Cerbo.PortalID, cfg.Cerbo.TankInstance, cfg.Cerbo.PumpInstance, cfg.Cerbo.ValveInstance)
	if cfg.CameraTopic != "" {
		mqttClient.SetCameraTopic(cfg.CameraTopic)
	}

	// Create Home Assistant client (optional)
	var haClient *homeassistant.Client
	if cfg.HomeAssistant != nil && cfg.HomeAssistant.URL != "" {
		logger.Info(logging.DefaultContext().With("component", "homeassistant"), "HomeAssistant config found, creating HA client")
		logger.Info(logging.DefaultContext().With("component", "homeassistant"), "HomeAssistant URL configured", "url", cfg.HomeAssistant.URL)
		logger.Info(logging.DefaultContext().With("component", "homeassistant"), "HA Entities configured",
			"boolean_entities", len(cfg.HomeAssistant.BooleanEntities),
			"switch_entities", len(cfg.HomeAssistant.SwitchEntities),
			"car_soc_entity", cfg.HomeAssistant.CarSOCEntity,
			"ev_charging_kw_entity", cfg.HomeAssistant.EVChargingKWEntity,
			"ev_power_entity", cfg.HomeAssistant.EVPowerEntity,
			"appliance_entities", len(cfg.HomeAssistant.ApplianceEntities),
		)

		haClient = homeassistant.NewClient(cfg.HomeAssistant)
		logger.Info(logging.DefaultContext().With("component", "homeassistant"), "HA client created",
			"configured", haClient != nil && haClient.IsDirectMode(),
			"direct_mode", haClient.IsDirectMode(),
		)
	} else {
		logger.Info(logging.DefaultContext().With("component", "homeassistant"), "HomeAssistant config NOT found (nil or empty URL)")
	}

	// Start MQTT connection (retry on failure instead of fatal)
	var mqttConnected bool
	for attempt := 1; attempt <= 10; attempt++ {
		if err := startMQTT(mqttClient); err != nil {
			logger.Warn(logging.DefaultContext().With("component", "mqtt"), "MQTT connection attempt failed",
				"attempt", attempt,
				"error", err,
			)
			if attempt < 10 {
				time.Sleep(5 * time.Second)
			}
		} else {
			mqttConnected = true
			break
		}
	}
	if !mqttConnected {
		logger.Error(logging.DefaultContext().With("component", "mqtt"), "MQTT connection failed after 10 attempts")
		os.Exit(1)
	}
	defer mqttClient.Disconnect()

	// Start HA polling if configured
	if haClient != nil {
		logger.Debug(logging.DefaultContext().With("component", "homeassistant"), "Checking if HA polling should start",
			"ha_client", haClient != nil,
			"direct_mode", haClient.IsDirectMode(),
		)
	}
	if haClient != nil && haClient.IsDirectMode() {
		logger.Info(logging.DefaultContext().With("component", "homeassistant"), "Starting HA poller goroutine")
		go haPoller(haClient, logger)
	} else {
		logger.Debug(logging.DefaultContext().With("component", "homeassistant"), "HA poller NOT started",
			"ha_client", haClient != nil,
			"direct_mode", haClient != nil && haClient.IsDirectMode(),
		)
	}

	// Set state callback for WebSocket broadcasts and metrics updates
	mqttClient.SetMessageHandler(func() {
		var broadcastOverlay homeassistant.Overlay
		if haClient != nil {
			broadcastOverlay = haClient.GetOverlay()
		}
		_ = websocket.BroadcastState(mqttClient, haClient, broadcastOverlay) // best-effort: state already logged inside

		// Update Prometheus metrics from current state
		state := mqttClient.GetState()
		metrics.DefaultCollector.UpdateFromState(state)
		metrics.DefaultCollector.UpdateWebsocketClients()
	})

	// Start command buffer metrics collector
	go collectCommandBufferMetrics(mqttClient)

	// Check for updates on startup
	go checkVersion(cfg.GitHub.RawURL, logger)

	// Create and configure HTTP server with tracing middleware
	server := createServer(mqttClient, haClient, cfg, logger, tracer)

	// Start server in a goroutine
	go startServer(server, cfg, *sslCert, *sslKey, logger)

	// Wait for shutdown signal
	waitForShutdown(server, mqttClient, haClient, logger)
}

func checkVersion(rawURL string, logger *logging.Logger) {
	if rawURL == "" {
		return
	}
	logger.Info(logging.DefaultContext().With("component", "version"), "Checking for updates...")
	latest, err := version.CheckLatest(rawURL)
	if err != nil {
		logger.Error(logging.DefaultContext().With("component", "version"), "Version check failed", "error", err)
		return
	}
	if latest != "" {
		version.SetLatestCached(latest)
		logger.Info(logging.DefaultContext().With("component", "version"), "Latest version found", "latest", latest)
	} else {
		logger.Info(logging.DefaultContext().With("component", "version"), "Already on latest version")
	}
}

func startMQTT(client *mqtt.Client) error {
	// Connect to MQTT broker
	if err := client.Connect(); err != nil {
		return fmt.Errorf("failed to connect to MQTT: %w", err)
	}

	// Subscribe to topics
	if err := client.Subscribe(); err != nil {
		return fmt.Errorf("failed to subscribe to topics: %w", err)
	}

	slog.Info("MQTT client started and connected", "host", client.GetIP(), "port", client.GetPort())
	return nil
}

func haPoller(haClient *homeassistant.Client, logger *logging.Logger) {
	logger.Info(logging.DefaultContext().With("component", "homeassistant"), "Starting Home Assistant poller")
	defer logger.Info(logging.DefaultContext().With("component", "homeassistant"), "Home Assistant poller stopped")

	ticker := time.NewTicker(haClient.GetPollInterval())
	defer ticker.Stop()

	for range ticker.C {
		haPollTick(haClient, logger)
	}
}

// haPollTick performs a single Home Assistant poll iteration: fetching the
// latest overlay state, logging diagnostic details, and updating the client.
func haPollTick(haClient *homeassistant.Client, logger *logging.Logger) {
	logger.Debug(logging.DefaultContext().With("component", "homeassistant"), "Poll tick received, calling FetchStatesOnce()")
	overlay, err := haClient.FetchStatesOnce()
	if err != nil {
		logger.Error(logging.DefaultContext().With("component", "homeassistant"), "HA poll failed", "error", err)
		return
	}
	logger.Debug(logging.DefaultContext().With("component", "homeassistant"), "FetchStatesOnce() completed",
		"ha_direct_connected", overlay.HADirectConnected,
	)
	if !overlay.HADirectConnected {
		logger.Debug(logging.DefaultContext().With("component", "homeassistant"), "HADirectConnected=false, ReplaceOverlay NOT called")
		return
	}

	logger.Info(logging.DefaultContext().With("component", "homeassistant"), "Successfully fetched entities",
		"count", len(overlay.AdditionalFields),
	)
	logHAEntities(overlay, logger)
	haClient.ReplaceOverlay(overlay)
}

// logHAEntities logs diagnostic details about the boolean and additional
// entities contained in a Home Assistant overlay.
func logHAEntities(overlay homeassistant.Overlay, logger *logging.Logger) {
	if len(overlay.AdditionalFields) == 0 {
		return
	}

	logger.Debug(logging.DefaultContext().With("component", "homeassistant"), "Values", "fields", overlay.AdditionalFields)

	// Log all collected boolean entities with their current states
	logger.Info(logging.DefaultContext().With("component", "homeassistant"), "Boolean Entities",
		"configured", len(overlay.Booleans),
	)
	for name, state := range overlay.Booleans {
		status := "OFF"
		if state {
			status = "ON"
		}
		logger.Debug(logging.DefaultContext().With("component", "homeassistant"), "Boolean entity",
			"name", name,
			"status", status,
		)
	}

	// Log all entities from overlay.AdditionalFields
	logger.Info(logging.DefaultContext().With("component", "homeassistant"), "Additional Fields",
		"entities", len(overlay.AdditionalFields),
	)
	for entity, value := range overlay.AdditionalFields {
		logger.Debug(logging.DefaultContext().With("component", "homeassistant"), "Entity value",
			"entity", entity,
			"value", value,
		)
	}
}

func createServer(mqttClient *mqtt.Client, haClient *homeassistant.Client, cfg *config.Config, logger *logging.Logger, tracer trace.Tracer) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	// Add tracing middleware before the logging middleware so that trace/span
	// IDs are already present in the request context when requests are logged.
	if tracer != nil {
		router.Use(tracing.GinMiddleware(tracer, serviceName))
	}
	router.Use(logging.NewStructuredMiddleware(logger))

	// Bearer-token auth for the dashboard page, WebSocket and API routes.
	// No-op while DASHBOARD_SECRET is unset. /health and /metrics stay open
	// for Docker healthchecks and Prometheus scraping.
	router.Use(auth.Middleware(cfg.DashboardSecret))

	// Serve Vue UI static assets (JS/CSS) from dist directory
	distDir := "internal/html/dist"
	if _, err := os.Stat(distDir); err == nil {
		router.Static("/assets", distDir+"/assets")
		logger.Info(logging.DefaultContext().With("component", "http"), "Serving Vue UI assets", "dir", distDir)
	}

	// Prometheus metrics endpoint
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))
	logger.Info(logging.DefaultContext().With("component", "http"), "Prometheus metrics endpoint enabled at /metrics")

	// Routes
	router.GET("/", indexHandler())
	router.GET("/ws", websocketHandler(mqttClient, haClient))
	router.GET("/api/state", apiStateHandler(mqttClient))
	router.GET("/health", healthHandler(mqttClient))
	router.POST("/api/check-update", apiCheckUpdateHandler())
	router.POST("/api/update", apiUpdateHandler(cfg.SelfUpdateEnabled))

	return router
}

func startServer(server *gin.Engine, cfg *config.Config, sslCert string, sslKey string, logger *logging.Logger) {
	addr := fmt.Sprintf("%s:%d", cfg.Web.Host, cfg.Web.Port)
	if sslCert != "" && sslKey != "" {
		logger.Info(logging.DefaultContext().With("component", "http"), "Starting HTTPS web server", "addr", addr)
		if err := server.RunTLS(addr, sslCert, sslKey); err != nil {
			logger.Error(logging.DefaultContext().With("component", "http"), "Failed to start HTTPS server", "error", err)
			os.Exit(1)
		}
	} else {
		logger.Info(logging.DefaultContext().With("component", "http"), "Starting HTTP web server", "addr", addr)
		if err := server.Run(addr); err != nil {
			logger.Error(logging.DefaultContext().With("component", "http"), "Failed to start server", "error", err)
			os.Exit(1)
		}
	}
}

func waitForShutdown(server *gin.Engine, mqttClient *mqtt.Client, haClient *homeassistant.Client, logger *logging.Logger) {
	// Create signal channel
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for signal
	<-sigChan
	logger.Info(logging.DefaultContext().With("component", "main"), "Shutting down...")

	// Close WebSocket connections
	websocket.CloseAll()

	// Shutdown server with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a simple HTTP server for shutdown
	tempServer := &http.Server{
		Handler: server,
	}

	if err := tempServer.Shutdown(ctx); err != nil {
		logger.Error(logging.DefaultContext().With("component", "http"), "Server shutdown error", "error", err)
	}

	logger.Info(logging.DefaultContext().With("component", "main"), "Shutdown complete")
}

// HTTP Handlers

func indexHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try Vue UI first, fall back to Go embedded dashboard
		if vueHTML, ok := html.GetVueUIHTML(); ok {
			c.Data(200, "text/html; charset=utf-8", vueHTML)
		} else {
			c.Data(200, "text/html; charset=utf-8", []byte(html.GetDashboardHTML()))
		}
	}
}

func websocketHandler(mqttClient *mqtt.Client, haClient *homeassistant.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		websocket.HandleWebSocket(c, mqttClient, haClient)
	}
}

func apiStateHandler(mqttClient *mqtt.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		state := mqttClient.GetState()

		has_mqtt_state := state != nil
		controlVersion := ""
		if state != nil {
			controlVersion = state.Version
		}

		c.JSON(200, gin.H{
			"ok":                true,
			"dashboard_version": version.GetCurrent(),
			"control_version":   controlVersion,
			"has_mqtt_state":    has_mqtt_state,
		})
	}
}

func apiCheckUpdateHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get latest from GitHub
		githubURL := "https://raw.githubusercontent.com/victron-venus/inverter-dashboard-go/main"
		latest, err := version.CheckLatest(githubURL)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		// Cache the latest version
		if latest != "" {
			version.SetLatestCached(latest)
		}

		current := version.GetCurrent()
		c.JSON(200, gin.H{
			"current": current,
			"latest":  latest,
		})
	}
}

// apiUpdateHandler downloads and swaps in the latest release binary, then
// exits so the container restart policy brings the new version up.
// Opt-in via SELF_UPDATE_ENABLED=true.
func apiUpdateHandler(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enabled {
			c.JSON(403, gin.H{"error": "self-update is disabled (set SELF_UPDATE_ENABLED=true)"})
			return
		}
		githubURL := "https://raw.githubusercontent.com/victron-venus/inverter-dashboard-go/main"
		latest, err := version.CheckLatest(githubURL)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		if latest != "" && latest == version.GetCurrent() {
			c.JSON(200, gin.H{"status": "up-to-date", "version": latest})
			return
		}
		if err := version.SelfUpdate(); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"status": "updated", "version": latest})
		// Flush the response, then exit; restart: unless-stopped restarts us into the new binary.
		go func() {
			time.Sleep(500 * time.Millisecond)
			os.Exit(0)
		}()
	}
}

// Health check
type healthResponse struct {
	Status        string    `json:"status"`
	Version       string    `json:"version"`
	Timestamp     time.Time `json:"timestamp"`
	Clients       int       `json:"websocket_clients"`
	MQTTConnected bool      `json:"mqtt_connected"`
	LastStateAge  string    `json:"last_state_age,omitempty"`
}

func healthHandler(mqttClient *mqtt.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		connected := mqttClient.IsConnected()
		lastState := mqttClient.LastStateTime()
		age := time.Since(lastState)

		status := "ok"
		if !connected || (!lastState.IsZero() && age > 30*time.Second) || lastState.IsZero() {
			status = "degraded"
		}

		c.JSON(200, healthResponse{
			Status:        status,
			Version:       version.GetCurrent(),
			Timestamp:     time.Now().UTC(),
			Clients:       websocket.GetConnectedCount(),
			MQTTConnected: connected,
			LastStateAge:  age.Truncate(time.Second).String(),
		})
	}
}

// collectCommandBufferMetrics periodically updates Prometheus metrics for the MQTT command buffer
func collectCommandBufferMetrics(mqttClient *mqtt.Client) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if mqttClient != nil {
			stats := mqttClient.GetCmdBufferStats()
			if stats != nil {
				metrics.DefaultCollector.UpdateMqttCmdBuffer(stats)
			}
		}
	}
}
