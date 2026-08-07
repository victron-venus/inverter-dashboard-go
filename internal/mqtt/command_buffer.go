package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// Command represents a queued MQTT command
type Command struct {
	Action  string
	Payload interface{}
	Time    time.Time
}

// CommandBuffer is a thread-safe channel-based queue with exponential backoff retry
type CommandBuffer struct {
	client  *Client
	queue   chan Command
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	// Retry configuration
	baseDelay   time.Duration
	maxDelay    time.Duration
	maxRetries  int
	retryJitter float64
}

// maxConcurrentRetries bounds the number of concurrent per-command retry goroutines.
const maxConcurrentRetries = 10

// NewCommandBuffer creates a new command buffer with the given capacity
func NewCommandBuffer(capacity int, client *Client) *CommandBuffer {
	ctx, cancel := context.WithCancel(context.Background())
	cb := &CommandBuffer{
		client:       client,
		queue:        make(chan Command, capacity),
		cancel:       cancel,
		baseDelay:    100 * time.Millisecond,
		maxDelay:     30 * time.Second,
		maxRetries:   10,
		retryJitter:  0.1,
	}

	cb.wg.Add(1)
	go cb.worker(ctx)
	log.Printf("MQTT command buffer worker started (capacity: %d)", capacity)

	return cb
}

// Start begins the background worker (for API compatibility, no-op since worker starts in NewCommandBuffer)
func (cb *CommandBuffer) Start() {
	// Already started in NewCommandBuffer
}

// Stop stops the background worker and drains the queue
func (cb *CommandBuffer) Stop() {
	cb.cancel()
	close(cb.queue)
	cb.wg.Wait()
	log.Printf("MQTT command buffer worker stopped")
}

// Enqueue adds a command to the buffer. If the buffer is full, the oldest
// command is dropped (ring-buffer behavior). Always returns nil.
func (cb *CommandBuffer) Enqueue(action string, payload interface{}) error {
	cmd := Command{
		Action:  action,
		Payload: payload,
		Time:    time.Now(),
	}

	select {
	case cb.queue <- cmd:
	default:
		// Buffer full - drop oldest command (ring buffer behavior)
		select {
		case old := <-cb.queue:
			log.Printf("Command buffer full, dropping oldest command: %s", old.Action)
		default:
		}
		// Try to enqueue the new command
		select {
		case cb.queue <- cmd:
		default:
			// Still full (unlikely), drop this one
			log.Printf("Command buffer still full, dropping new command: %s", cmd.Action)
		}
	}

	return nil
}

// worker processes commands from the queue with exponential backoff
func (cb *CommandBuffer) worker(ctx context.Context) {
	defer cb.wg.Done()

	sem := make(chan struct{}, maxConcurrentRetries)

	for cmd := range cb.queue {
		// Process command with retry in the background
		cb.wg.Add(1)
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			cb.wg.Done()
			return
		}
		go func(cmd Command) {
			defer cb.wg.Done()
			defer func() { <-sem }()
			if err := cb.publishWithRetry(ctx, cmd); err != nil {
				log.Printf("Failed to publish command after retries: action=%s, error=%v", cmd.Action, err)
			}
		}(cmd)
	}

	// Drain remaining semaphore waiters on shutdown
	cb.drain(sem)
}

// drain waits for in-flight retries to complete on shutdown
func (cb *CommandBuffer) drain(sem chan struct{}) {
	for i := 0; i < maxConcurrentRetries; i++ {
		select {
		case sem <- struct{}{}:
		default:
			return
		}
	}
}

// publishWithRetry attempts to publish with exponential backoff
func (cb *CommandBuffer) publishWithRetry(ctx context.Context, cmd Command) error {
	delay := cb.baseDelay

	for attempt := 0; attempt <= cb.maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return fmt.Errorf("worker context cancelled")
		default:
		}

		err := cb.publishOnce(cmd)
		if err == nil {
			return nil
		}

		// Check if we should retry
		if !cb.shouldRetry(err) || attempt == cb.maxRetries {
			return fmt.Errorf("max retries exceeded: %w", err)
		}

		// Add jitter
		jitter := time.Duration(float64(delay) * cb.retryJitter * (0.5 - float64(time.Now().UnixNano()%1000)/1000))
		sleepTime := delay + jitter

		log.Printf("Publish failed (attempt %d/%d), retrying in %v: action=%s, error=%v",
			attempt+1, cb.maxRetries+1, sleepTime, cmd.Action, err)

		select {
		case <-ctx.Done():
			return fmt.Errorf("worker context cancelled")
		case <-time.After(sleepTime):
		}

		// Exponential backoff
		delay *= 2
		if delay > cb.maxDelay {
			delay = cb.maxDelay
		}
	}

	return fmt.Errorf("max retries exceeded")
}

// publishOnce attempts to publish a single command
func (cb *CommandBuffer) publishOnce(cmd Command) error {
	if cb.client == nil || cb.client.client == nil {
		return fmt.Errorf("MQTT client not initialized")
	}

	if !cb.client.client.IsConnected() {
		return fmt.Errorf("MQTT client not connected")
	}

	topic := fmt.Sprintf("inverter/cmd/%s", cmd.Action)
	var message string

	if cmd.Payload != nil {
		data, err := json.Marshal(cmd.Payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}
		message = string(data)
	}

	token := cb.client.client.Publish(topic, 0, false, message)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to publish to %s: %w", topic, token.Error())
	}

	log.Printf("Published buffered command to %s", topic)
	return nil
}

// shouldRetry determines if an error is retryable
func (cb *CommandBuffer) shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	retryableErrors := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"not connected",
		"tls",
		"broken pipe",
		"closed",
	}
	for _, re := range retryableErrors {
		if strings.Contains(errStr, re) {
			return true
		}
	}
	return false
}

// Stats returns buffer statistics
func (cb *CommandBuffer) Stats() map[string]interface{} {
	return map[string]interface{}{
		"capacity":    cap(cb.queue),
		"count":       len(cb.queue),
		"utilization": float64(len(cb.queue)) / float64(cap(cb.queue)) * 100,
	}
}
