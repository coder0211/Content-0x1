package sse

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestPumpStopsOnDone(t *testing.T) {
	rec := httptest.NewRecorder()
	w, err := Start(rec)
	if err != nil {
		t.Fatal(err)
	}
	ch := make(chan Event, 2)
	ch <- Event{Name: "status", Data: []byte(`{"status":"running"}`)}
	ch <- Event{Name: "done", Data: []byte(`{"status":"succeeded"}`)}
	if err := w.Pump(context.Background(), ch, func(e Event) bool { return e.Name == "done" }); err != nil {
		t.Fatal(err)
	}
	want := "event: status\ndata: {\"status\":\"running\"}\n\nevent: done\ndata: {\"status\":\"succeeded\"}\n\n"
	if rec.Body.String() != want {
		t.Fatalf("body = %q", rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatal("content type")
	}
}
