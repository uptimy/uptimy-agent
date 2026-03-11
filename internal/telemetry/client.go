package telemetry

import (
	"time"

	"go.uber.org/zap"
)

// Client collects and dispatches telemetry events.
// When the control plane connection is unavailable, events are buffered locally.
type Client struct {
	buffer  *RingBuffer
	logger  *zap.SugaredLogger
	metrics *Metrics
}

// NewClient creates a telemetry client backed by the given ring buffer.
func NewClient(bufferSize int, metrics *Metrics, logger *zap.SugaredLogger) *Client {
	return &Client{
		buffer:  NewRingBuffer(bufferSize),
		logger:  logger,
		metrics: metrics,
	}
}

// RecordEvent stores a telemetry event in the buffer.
func (c *Client) RecordEvent(eventType string, data map[string]interface{}) {
	c.buffer.Push(Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      data,
	})
}

// Buffer returns the underlying ring buffer for batch operations.
func (c *Client) Buffer() *RingBuffer {
	return c.buffer
}

// Metrics returns the Prometheus metrics instance.
func (c *Client) Metrics() *Metrics {
	return c.metrics
}
