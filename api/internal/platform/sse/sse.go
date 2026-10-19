// Package sse writes Server-Sent Events streams (SPEC-04 §3).
package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// PingInterval is the keep-alive comment period.
const PingInterval = 15 * time.Second

// Event is one SSE message.
type Event struct {
	Name string
	Data json.RawMessage
}

// Writer streams events to one client.
type Writer struct {
	w http.ResponseWriter
	f http.Flusher
}

// Start sets SSE headers and returns a Writer, or an error when the
// ResponseWriter cannot flush.
func Start(w http.ResponseWriter) (*Writer, error) {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("sse: response writer does not support flushing")
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no") // disable nginx buffering
	w.WriteHeader(http.StatusOK)
	f.Flush()
	return &Writer{w: w, f: f}, nil
}

// Send writes one event.
func (s *Writer) Send(ev Event) error {
	var b strings.Builder
	if ev.Name != "" {
		b.WriteString("event: " + ev.Name + "\n")
	}
	for _, line := range strings.Split(string(ev.Data), "\n") {
		b.WriteString("data: " + line + "\n")
	}
	b.WriteString("\n")
	if _, err := s.w.Write([]byte(b.String())); err != nil {
		return err
	}
	s.f.Flush()
	return nil
}

// Ping writes a keep-alive comment.
func (s *Writer) Ping() error {
	if _, err := s.w.Write([]byte(": ping\n\n")); err != nil {
		return err
	}
	s.f.Flush()
	return nil
}

// Pump forwards events until ctx ends, events closes, or stop returns true
// for a sent event (e.g. the terminal "done" event). It pings every
// PingInterval.
func (s *Writer) Pump(ctx context.Context, events <-chan Event, stop func(Event) bool) error {
	t := time.NewTicker(PingInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if err := s.Ping(); err != nil {
				return err
			}
		case ev, ok := <-events:
			if !ok {
				return nil
			}
			if err := s.Send(ev); err != nil {
				return err
			}
			if stop != nil && stop(ev) {
				return nil
			}
		}
	}
}
