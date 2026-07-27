package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// Command represents a queued MQTT command
type Command struct {
	Action  string
	Payload interface{}
	Time    time.Time
}

// CommandBuffer is a thread-safe ring buffer with exponential backoff retry
type CommandBuffer struct {
	client       *Client
	buffer       []Command
	capacity     int
	head         int
	tail         int
	count        int
	mu           sync.Mutex
	notEmpty     *sync.Cond
	workerCtx    context.Context
	workerCancel context.CancelFunc
	workerWG     sync.WaitGroup

	// Retry configuration
	baseDelay      time.Duration
	maxDelay       time.Duration
	maxRetries     int
	retryJitter    float64
}

// NewCommandBuffer creates a new command buffer with the given capacity
func NewCommandBuffer(capacity int, client *Client) *CommandBuffer {
	ctx, cancel := context.WithCancel(context.Background())

	cb := &CommandBuffer{
		client:       client,
		buffer:       make([]Command, capacity),
		capacity:     capacity,
		workerCtx:    ctx,
		workerCancel: cancel,
		baseDelay:    100 * time.Millisecond,
		maxDelay:     30 * time.Second,
		maxRetries:   10,
		retryJitter:  0.1,
	}

	cb.notEmpty = sync.NewCond(&cb.mu)

	return cb
}

// Start begins the background worker that processes the command queue
func (cb *CommandBuffer) Start() {
	cb.workerWG.Add(1)
	go cb.worker()
	log.Printf("MQTT command buffer worker started (capacity: %d)", cb.capacity)
}

// Stop stops the background worker and drains the queue
func (cb *CommandBuffer) Stop() {
	cb.mu.Lock()
	cb.workerCancel()
	cb.notEmpty.Broadcast()
	cb.mu.Unlock()
	cb.workerWG.Wait()
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

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.count == cb.capacity {
		// Buffer full - drop oldest command (ring buffer behavior)
		log.Printf("Command buffer full, dropping oldest command")
		cb.head = (cb.head + 1) % cb.capacity
		cb.count--
	}

	cb.buffer[cb.tail] = cmd
	cb.tail = (cb.tail + 1) % cb.capacity
	cb.count++

	// Signal worker
	cb.notEmpty.Signal()

	return nil
}

// worker processes commands from the buffer with exponential backoff
func (cb *CommandBuffer) worker() {
	defer cb.workerWG.Done()

	for {
		cmd, ok := cb.dequeue()
		if !ok {
			// Buffer empty, wait for signal or context cancellation
			cb.mu.Lock()
			for cb.count == 0 {
				select {
				case <-cb.workerCtx.Done():
					cb.mu.Unlock()
					// Drain remaining commands on shutdown
					cb.drain()
					return
				default:
				}
				cb.notEmpty.Wait()
			}
			cb.mu.Unlock()
			continue
		}

		// Process command with retry in the background so a single stuck
		// command (e.g. one that requires many retries with backoff) does
		// not block the worker from dequeuing and processing newer commands.
		cb.workerWG.Add(1)
		go func(cmd Command) {
			defer cb.workerWG.Done()
			if err := cb.publishWithRetry(cmd); err != nil {
				log.Printf("Failed to publish command after retries: action=%s, error=%v", cmd.Action, err)
				// Could implement dead letter queue here if needed
			}
		}(cmd)
	}
}

// dequeue removes and returns the next command from the buffer
func (cb *CommandBuffer) dequeue() (Command, bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.count == 0 {
		return Command{}, false
	}

	cmd := cb.buffer[cb.head]
	cb.buffer[cb.head] = Command{} // Clear for GC
	cb.head = (cb.head + 1) % cb.capacity
	cb.count--

	return cmd, true
}

// drain processes all remaining commands during shutdown
func (cb *CommandBuffer) drain() {
	for {
		cmd, ok := cb.dequeue()
		if !ok {
			break
		}
		// Try once without retry on shutdown
		if err := cb.publishOnce(cmd); err != nil {
			log.Printf("Shutdown drain failed for command %s: %v", cmd.Action, err)
		}
	}
}

// publishWithRetry attempts to publish with exponential backoff
func (cb *CommandBuffer) publishWithRetry(cmd Command) error {
	delay := cb.baseDelay

	for attempt := 0; attempt <= cb.maxRetries; attempt++ {
		select {
		case <-cb.workerCtx.Done():
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
		case <-cb.workerCtx.Done():
			return fmt.Errorf("worker context cancelled")
		case <-time.After(sleepTime):
		}

		// Exponential backoff
		delay = delay * 2
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
	// Retry on connection errors, timeout errors
	errStr := err.Error()
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
		if containsIgnoreCase(errStr, re) {
			return true
		}
	}
	return false
}

// containsIgnoreCase checks if string contains substring (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if lower(s[i+j]) != lower(substr[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func lower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + 'a' - 'A'
	}
	return b
}

// Stats returns buffer statistics
func (cb *CommandBuffer) Stats() map[string]interface{} {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	return map[string]interface{}{
		"capacity":     cb.capacity,
		"count":        cb.count,
		"head":         cb.head,
		"tail":         cb.tail,
		"utilization":  float64(cb.count) / float64(cb.capacity) * 100,
	}
}