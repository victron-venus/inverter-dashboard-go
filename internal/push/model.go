package push

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

const (
	maxSubscriptions = 64
	maxDedupe        = 4096
	maxPending       = 1024
	maxPayloadBytes  = 3072
	maxRequestBytes  = 8192
	maxEventAge      = 300 * time.Second
	maxFutureSkew    = 30 * time.Second
)

var (
	errConflict  = errors.New("subscription key conflict")
	errCapacity  = errors.New("notification capacity reached")
	errNotFound  = errors.New("subscription not registered")
	errRateLimit = errors.New("notification rate limit reached")
)

type Preferences struct {
	Native     bool `json:"native"`
	EV         bool `json:"ev"`
	Water      bool `json:"water"`
	LowBattery bool `json:"lowBattery"`
}

func DefaultPreferences() Preferences { return Preferences{true, true, true, true} }

func (p *Preferences) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 4 {
		return errors.New("invalid notification preferences")
	}
	for name, field := range map[string]*bool{"native": &p.Native, "ev": &p.EV, "water": &p.Water, "lowBattery": &p.LowBattery} {
		value, ok := fields[name]
		if !ok || (string(value) != "true" && string(value) != "false") || json.Unmarshal(value, field) != nil {
			return errors.New("invalid notification preferences")
		}
	}
	return nil
}

func (p Preferences) allows(kind string) bool {
	switch kind {
	case "native":
		return p.Native
	case "ev":
		return p.EV
	case "water":
		return p.Water
	case "lowBattery":
		return p.LowBattery
	case "test":
		return true
	}
	return false
}

type Payload struct {
	SchemaVersion     int    `json:"schemaVersion"`
	EventKey          string `json:"eventKey"`
	Kind              string `json:"kind"`
	Source            string `json:"source"`
	SourceTimestampMS int64  `json:"sourceTimestampMs"`
	ObservedAtMS      int64  `json:"observedAtMs"`
	Title             string `json:"title"`
	Body              string `json:"body"`
	URL               string `json:"url"`
}

func eventKey(kind, source, id string, timestamp int64) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode([]any{kind, source, id, timestamp})
	sum := sha256.Sum256(compactUTF8JSON(bytes.TrimSuffix(buf.Bytes(), []byte("\n"))))
	return hex.EncodeToString(sum[:])
}

func boundedText(text string, limit int) string {
	runes := []rune(text)
	if len(runes) > limit {
		runes = runes[:limit]
	}
	return string(runes)
}

func makePayload(kind, source, id, title, body string, occurred, observed time.Time) Payload {
	payload := Payload{SchemaVersion: 1, EventKey: eventKey(kind, source, id, occurred.UnixMilli()), Kind: kind, Source: source,
		SourceTimestampMS: occurred.UnixMilli(), ObservedAtMS: observed.UnixMilli(), Title: boundedText(title, 120), Body: boundedText(body, 1000), URL: "/"}
	if payload.Title == "" {
		payload.Title = "Notification"
	}
	runes := []rune(payload.Body)
	for {
		raw, _ := json.Marshal(payload)
		if len(raw) <= maxPayloadBytes || len(runes) == 0 {
			break
		}
		runes = runes[:len(runes)-1]
		payload.Body = string(runes)
	}
	return payload
}

func (p Payload) current(now time.Time) bool {
	at := time.UnixMilli(p.SourceTimestampMS)
	return p.SourceTimestampMS > 0 && now.Sub(at) <= maxEventAge && at.Sub(now) <= maxFutureSkew
}

type storedSubscription struct {
	Subscription webpush.Subscription `json:"subscription"`
	Preferences  Preferences          `json:"preferences"`
	LastTestMS   int64                `json:"lastTestMs"`
}

type delivery struct {
	ID             string  `json:"id"`
	SubscriptionID string  `json:"subscriptionId"`
	Epoch          string  `json:"epoch"`
	Payload        Payload `json:"payload"`
	Attempts       int     `json:"attempts"`
	NextAttemptMS  int64   `json:"nextAttemptMs"`
}

func subscriptionID(endpoint string) string {
	sum := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(sum[:])
}

// encoding/json escapes the two JavaScript line separators even with HTML
// escaping disabled. Compact UTF-8 JSON (Python/JSON.stringify contract) does not.
func compactUTF8JSON(raw []byte) []byte {
	out := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' && i+1 < len(raw) {
			if i+5 < len(raw) && (string(raw[i:i+6]) == "\\u2028" || string(raw[i:i+6]) == "\\u2029") {
				if raw[i+5] == '8' {
					out = append(out, []byte(" ")...)
				} else {
					out = append(out, []byte(" ")...)
				}
				i += 5
				continue
			}
			out = append(out, raw[i], raw[i+1])
			i++
			continue
		}
		out = append(out, raw[i])
	}
	return out
}
