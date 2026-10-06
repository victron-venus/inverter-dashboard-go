package push

import (
	"time"
)

// Config is separate from telemetry configuration so the notification API can be
// exercised without connecting to any inverter, gateway or Home Assistant.
type Config struct {
	Enabled          bool
	DataDir, Subject string
}
type Service struct {
	enabled    bool
	reason     string
	store      *Store
	processor  *Processor
	dispatcher *Dispatcher
}

func NewService(cfg Config) *Service {
	s := &Service{enabled: cfg.Enabled}
	if !cfg.Enabled {
		s.reason = "Web Push is disabled"
		return s
	}
	if supportedStorePlatform() != nil {
		s.reason = "Persistent Web Push requires Linux or macOS"
		return s
	}
	if cfg.Subject == "" {
		cfg.Subject = "https://github.com/victron-venus/inverter-dashboard-go"
	}
	if validateSubject(cfg.Subject) != nil {
		s.reason = "Web Push contact configuration is invalid"
		return s
	}
	store, err := OpenStore(cfg.DataDir)
	if err != nil {
		s.reason = "Persistent Web Push storage is unavailable"
		return s
	}
	s.store = store
	s.processor = NewProcessor(store)
	if err = s.processor.Reset("startup", false, time.Now()); err != nil {
		_ = store.Close()
		s.reason = "Persistent Web Push storage is unavailable"
		return s
	}
	s.dispatcher = newDispatcher(store, cfg.Subject)
	return s
}
func (s *Service) Available() bool {
	return s != nil && s.store != nil && s.reason == "" && s.store.Available()
}
func (s *Service) Start() {
	if s.Available() {
		s.dispatcher.Start()
	}
}
func (s *Service) Close() {
	if s == nil {
		return
	}
	if s.dispatcher != nil {
		s.dispatcher.Close()
	}
	if s.store != nil {
		_ = s.store.Close()
	}
}
func (s *Service) Reset(source string, connected bool, now time.Time) {
	if s.Available() {
		_ = s.processor.Reset(source, connected, now)
	}
}
func (s *Service) Observe(ob Observation, now time.Time) {
	if s.Available() {
		_ = s.processor.Observe(ob, now)
	}
}
func (s *Service) status() map[string]any {
	var reason, public any
	available := s.Available()
	if available {
		public = s.store.PublicKey()
	} else {
		if s.reason != "" {
			reason = s.reason
		} else {
			reason = "Persistent Web Push storage is unavailable"
		}
	}
	return map[string]any{"enabled": s.enabled, "available": available, "reason": reason, "publicKey": public, "preferencesDefaults": DefaultPreferences(), "maxNotificationAgeSeconds": 300}
}
