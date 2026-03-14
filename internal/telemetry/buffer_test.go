package telemetry_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/uptimy/uptimy-agent/internal/telemetry"
)

func TestRingBuffer_PushAndDrain(t *testing.T) {
	buf := telemetry.NewRingBuffer(5)

	for i := 0; i < 3; i++ {
		buf.Push(telemetry.Event{
			Type:      "test",
			Timestamp: time.Now(),
		})
	}

	if buf.Len() != 3 {
		t.Errorf("expected len 3, got %d", buf.Len())
	}

	events := buf.Drain()
	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}
	if buf.Len() != 0 {
		t.Errorf("expected len 0 after drain, got %d", buf.Len())
	}
}

func TestRingBuffer_Overflow(t *testing.T) {
	buf := telemetry.NewRingBuffer(3)

	for i := 0; i < 5; i++ {
		buf.Push(telemetry.Event{
			Type:      "test",
			Timestamp: time.Now(),
			Data:      map[string]string{"index": fmt.Sprintf("%d", i)},
		})
	}

	if buf.Len() != 3 {
		t.Errorf("expected len 3 (capped), got %d", buf.Len())
	}

	events := buf.Drain()
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Should contain the last 3 events (indices 2, 3, 4).
	for i, ev := range events {
		expected := fmt.Sprintf("%d", i+2)
		if ev.Data["index"] != expected {
			t.Errorf("event %d: expected index %s, got %s", i, expected, ev.Data["index"])
		}
	}
}

func TestRingBuffer_DrainEmpty(t *testing.T) {
	buf := telemetry.NewRingBuffer(5)

	events := buf.Drain()
	if events != nil {
		t.Errorf("expected nil from empty drain, got %v", events)
	}
}
