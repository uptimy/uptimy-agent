package telemetry

import (
	"encoding/json"
	"sync"
	"time"
)

// Event represents a telemetry event to be sent to the control plane.
type Event struct {
	Type      string            `json:"type"`
	Timestamp time.Time         `json:"timestamp"`
	Data      map[string]string `json:"data"`
}

// RingBuffer is a fixed-capacity circular buffer for telemetry events.
// When full, the oldest events are dropped to make room for new ones.
type RingBuffer struct {
	mu    sync.Mutex
	buf   []Event
	head  int
	count int
	cap   int
}

// NewRingBuffer creates a buffer with the given capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		buf: make([]Event, capacity),
		cap: capacity,
	}
}

// Push adds an event to the buffer. If full, drops the oldest event.
func (rb *RingBuffer) Push(event Event) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	idx := (rb.head + rb.count) % rb.cap
	rb.buf[idx] = event

	if rb.count == rb.cap {
		// Buffer is full; advance head (drops oldest).
		rb.head = (rb.head + 1) % rb.cap
	} else {
		rb.count++
	}
}

// Drain removes and returns all events from the buffer.
func (rb *RingBuffer) Drain() []Event {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.count == 0 {
		return nil
	}

	events := make([]Event, 0, rb.count)
	for i := 0; i < rb.count; i++ {
		idx := (rb.head + i) % rb.cap
		events = append(events, rb.buf[idx])
	}

	rb.head = 0
	rb.count = 0

	return events
}

// Len returns the current number of events in the buffer.
func (rb *RingBuffer) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count
}

// Snapshot returns a copy of all buffered events without removing them.
func (rb *RingBuffer) Snapshot() []Event {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.count == 0 {
		return nil
	}

	events := make([]Event, 0, rb.count)
	for i := 0; i < rb.count; i++ {
		idx := (rb.head + i) % rb.cap
		events = append(events, rb.buf[idx])
	}
	return events
}

// MarshalJSON serializes all buffered events as a JSON array without
// removing them from the buffer.
func (rb *RingBuffer) MarshalJSON() ([]byte, error) {
	events := rb.Snapshot()
	return json.Marshal(events)
}
