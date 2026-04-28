package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/victron-venus/inverter-dashboard-go/internal/config"
	"github.com/victron-venus/inverter-dashboard-go/internal/homeassistant"
	"github.com/victron-venus/inverter-dashboard-go/internal/html"
	"github.com/victron-venus/inverter-dashboard-go/internal/mqtt"
	"github.com/victron-venus/inverter-dashboard-go/internal/version"
	"github.com/victron-venus/inverter-dashboard-go/internal/websocket"
)

var (
	// Version is set during build
	Version string = "dev"
)

func init() {
	// Override version package's version with our build-time version
	if Version != "" && Version != "dev" {
		// Note: version package uses its own internal variable
		// This is just for reference
	}
}

func main() {
	// Command line flags - match Python exactly
	var (
		mqttHost  = flag.String("mqtt-host", "", "MQTT broker host")
		mqttPort  = flag.Int("mqtt-port", 0, "MQTT broker port")
		webPort   = flag.Int("port", 0, "Web server port")
		sslCert   = flag.String("ssl-cert", "", "SSL certificate file")
		sslKey    = flag.String("ssl-key", "", "SSL key file")
		showVersion = flag.Bool("version", false, "Show version and exit")
	)
	flag.Parse()

	// Show version and exit
	if *showVersion {
		fmt.Printf("Inverter Dashboard v%s\n", version.GetCurrent())
		os.Exit(0)
	}

	// Load configuration - Python uses environment only
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
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

	// Print startup info
	log.Printf("Starting Inverter Dashboard v%s", version.GetCurrent())
	log.Printf("MQTT: %s:%d", cfg.MQTT.Host, cfg.MQTT.Port)
	log.Printf("Web: %s://%s:%d", proto, cfg.Web.Host, cfg.Web.Port)

	// Create MQTT client
	mqttClient := mqtt.NewClient(cfg.MQTT.Host, cfg.MQTT.Port)

	// Create Home Assistant client (optional)
	var haClient *homeassistant.Client
	if cfg.HomeAssistant != nil && cfg.HomeAssistant.URL != "" {
		log.Printf("[MAIN DEBUG] HomeAssistant config found, creating HA client")
		log.Printf("[MAIN CONFIG] HomeAssistant URL: %s", cfg.HomeAssistant.URL)
		log.Printf("[MAIN CONFIG] HomeAssistant Token: %s...%s (truncated)", cfg.HomeAssistant.Token[:10], cfg.HomeAssistant.Token[len(cfg.HomeAssistant.Token)-5:])
		log.Printf("[MAIN CONFIG] HA Entities configured:")
		log.Printf("[MAIN CONFIG]   - Boolean entities: %d", len(cfg.HomeAssistant.BooleanEntities))
		log.Printf("[MAIN CONFIG]   - Switch entities: %d", len(cfg.HomeAssistant.SwitchEntities))
		log.Printf("[MAIN CONFIG]   - Water level entity: %s", cfg.HomeAssistant.WaterLevelEntity)
		log.Printf("[MAIN CONFIG]   - Car SoC entity: %s", cfg.HomeAssistant.CarSOCEntity)
		log.Printf("[MAIN CONFIG]   - EV Charging KW entity: %s", cfg.HomeAssistant.EVChargingKWEntity)
		log.Printf("[MAIN CONFIG]   - EV Power entity: %s", cfg.HomeAssistant.EVPowerEntity)
		log.Printf("[MAIN CONFIG]   - Appliance entities: %d", len(cfg.HomeAssistant.ApplianceEntities))

		haClient = homeassistant.NewClient(cfg.HomeAssistant)
		log.Printf("[MAIN DEBUG] HA client created: configured=%v", haClient != nil && haClient.IsDirectMode())
	} else {
		log.Printf("[MAIN DEBUG] HomeAssistant config NOT found (nil or empty URL)")
	}

	// Start MQTT connection
	if err := startMQTT(mqttClient); err != nil {
		log.Fatalf("Failed to start MQTT: %v", err)
	}
	defer mqttClient.Disconnect()

	// Start HA polling if configured
	if haClient != nil {
		log.Printf("[MAIN DEBUG] Checking if HA polling should start: haClient!=nil, IsDirectMode=%v", haClient.IsDirectMode())
	}
	if haClient != nil && haClient.IsDirectMode() {
		log.Printf("[MAIN DEBUG] Starting HA poller goroutine")
		go haPoller(haClient)
	} else {
		log.Printf("[MAIN DEBUG] HA poller NOT started (haClient=nil or !IsDirectMode)")
	}

	// Set state callback for WebSocket broadcasts
	mqttClient.SetMessageHandler(func() {
		var broadcastOverlay homeassistant.Overlay
		if haClient != nil {
			broadcastOverlay = haClient.GetOverlay()
		}
		websocket.BroadcastState(mqttClient, haClient, broadcastOverlay)
	})

	// Check for updates on startup
	go checkVersion(cfg.GitHub.RawURL)

	// Create and configure HTTP server
	server := createServer(mqttClient, haClient)

	// Start server in a goroutine
	go startServer(server, cfg, *sslCert, *sslKey)

	// Wait for shutdown signal
	waitForShutdown(server, mqttClient, haClient)
}

func checkVersion(rawURL string) {
	if rawURL == "" {
		return
	}
	log.Printf("Checking for updates...")
	latest, err := version.CheckLatest(rawURL)
	if err != nil {
		log.Printf("Version check failed: %v", err)
		return
	}
	if latest != "" {
		version.SetLatestCached(latest)
		log.Printf("Latest version: %s", latest)
	} else {
		log.Printf("Already on latest version")
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

	log.Printf("MQTT client started and connected to %s:%d", client.GetIP(), client.GetPort())
	return nil
}

func haPoller(haClient *homeassistant.Client) {
	log.Printf("Starting Home Assistant poller")
	defer log.Printf("Home Assistant poller stopped")
	ticker := time.NewTicker(haClient.GetPollInterval())

	for range ticker.C {
		log.Printf("[HA POLL DEBUG] Poll tick received, calling FetchStatesOnce()")
		overlay, err := haClient.FetchStatesOnce()
		if err != nil {
			log.Printf("HA poll failed: %v", err)
			continue
		}
		log.Printf("[HA POLL DEBUG] FetchStatesOnce() completed, HADirectConnected: %v", overlay.HADirectConnected)
			if overlay.HADirectConnected {
			log.Printf("[HA POLL] Successfully fetched %d entitites", len(overlay.AdditionalFields))
			if len(overlay.AdditionalFields) > 0 {
				log.Printf("[HA POLL] Values: %+v", overlay.AdditionalFields)

				// Log all collected boolean entities with their current states
				log.Printf("[HA POLL] Boolean Entities (configured: %d):", len(overlay.Booleans))
				for name, state := range overlay.Booleans {
					status := "OFF"
					if state {
						status = "ON"
					}
					log.Printf("[HA POLL] - %s: %s", name, status)
				}

				// Log all entities from overlay.AdditionalFields
				log.Printf("[HA POLL] Additional Fields (entities: %d):", len(overlay.AdditionalFields))
				for entity, value := range overlay.AdditionalFields {
					log.Printf("[HA POLL] - %s: %v", entity, value)
				}
			}
			haClient.ReplaceOverlay(overlay)
		} else {
			log.Printf("[HA POLL DEBUG] HADirectConnected=false, ReplaceOverlay NOT called")
		}
	}
}

func createServer(mqttClient *mqtt.Client, haClient *homeassistant.Client) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(loggingMiddleware())

	// Routes
	router.GET("/", indexHandler())
	router.GET("/ws", websocketHandler(mqttClient, haClient))
	router.GET("/api/state", apiStateHandler(mqttClient))
	router.POST("/api/check-update", apiCheckUpdateHandler())
	router.POST("/api/update", apiUpdateHandler())

	return router
}

func startServer(server *gin.Engine, cfg *config.Config, sslCert string, sslKey string) {
	addr := fmt.Sprintf("%s:%d", cfg.Web.Host, cfg.Web.Port)
	if sslCert != "" && sslKey != "" {
		log.Printf("Starting HTTPS web server on %s", addr)
		if err := server.RunTLS(addr, sslCert, sslKey); err != nil {
			log.Fatalf("Failed to start HTTPS server: %v", err)
		}
	} else {
		log.Printf("Starting HTTP web server on %s", addr)
		if err := server.Run(addr); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}
}

func waitForShutdown(server *gin.Engine, mqttClient *mqtt.Client, haClient *homeassistant.Client) {
	// Create signal channel
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for signal
	<-sigChan
	log.Println("Shutting down...")

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
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Shutdown complete")
}

// HTTP Handlers

func indexHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Data(200, "text/html; charset=utf-8", []byte(html.GetDashboardHTML()))
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
			"ok": true,
			"dashboard_version": version.GetCurrent(),
			"control_version": controlVersion,
			"has_mqtt_state": has_mqtt_state,
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
			"latest": latest,
		})
	}
}

func apiUpdateHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Println("Update requested...")

		githubURL := "https://raw.githubusercontent.com/victron-venus/inverter-dashboard-go/main"
		result := version.UpdateFiles(githubURL)

		if result.Success {
			// Schedule restart
			go func() {
				time.Sleep(1 * time.Second)
				log.Printf("Restarting after update to v%s", result.Version)
				os.Exit(0)
			}()

			c.JSON(200, gin.H{
				"status": "updated",
				"version": result.Version,
				"message": fmt.Sprintf("Updated to v%s, restarting...", result.Version),
			})
		} else {
			c.JSON(500, gin.H{
				"error": result.Error.Error(),
			})
		}
	}
}

// loggingMiddleware logs HTTP requests
func loggingMiddleware() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return fmt.Sprintf("%s - [%s] \"%s %s %s %d %s \"%s\" %s\"\n",
			param.ClientIP,
			param.TimeStamp.Format("2006/01/02 15:04:05"),
			param.Method,
			param.Path,
			param.Request.Proto,
			param.StatusCode,
			param.Latency,
			param.Request.UserAgent(),
			param.ErrorMessage,
		)
	})
}

// Health check
type healthResponse struct {
	Status    string    `json:"status"`
	Version   string    `json:"version"`
	Timestamp time.Time `json:"timestamp"`
	Clients   int       `json:"websocket_clients"`
}

func healthHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, healthResponse{
			Status:    "ok",
			Version:   version.GetCurrent(),
			Timestamp: time.Now().UTC(),
			Clients:   websocket.GetConnectedCount(),
		})
	}
}