package httpx

import (
	"encoding/json"
	"net/http"

	"github.com/be/acs/api/internal/shared/kernel"
)

// Meta is the envelope metadata (SPEC-04 §1.1).
type Meta struct {
	RequestID  string  `json:"request_id"`
	NextCursor *string `json:"next_cursor,omitempty"`
}

type envelope struct {
	Data any  `json:"data"`
	Meta Meta `json:"meta"`
}

type listMeta struct {
	RequestID  string  `json:"request_id"`
	NextCursor *string `json:"next_cursor"`
}

type listEnvelope struct {
	Data any      `json:"data"`
	Meta listMeta `json:"meta"`
}

type errorBody struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Details []kernel.Detail `json:"details"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
	Meta  Meta      `json:"meta"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// JSON writes data in a success envelope with status.
func JSON(w http.ResponseWriter, r *http.Request, status int, data any) {
	writeJSON(w, status, envelope{Data: data, Meta: Meta{RequestID: RequestID(r.Context())}})
}

// OK writes 200 with data.
func OK(w http.ResponseWriter, r *http.Request, data any) { JSON(w, r, http.StatusOK, data) }

// Created writes 201 with data.
func Created(w http.ResponseWriter, r *http.Request, data any) {
	JSON(w, r, http.StatusCreated, data)
}

// Accepted writes 202 with data (AI job created).
func Accepted(w http.ResponseWriter, r *http.Request, data any) {
	JSON(w, r, http.StatusAccepted, data)
}

// NoContent writes 204.
func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// List writes a page of items with meta.next_cursor (always present, null
// on the last page). items must be a slice; nil is sent as [].
func List[T any](w http.ResponseWriter, r *http.Request, items []T, next *kernel.Cursor) {
	if items == nil {
		items = []T{}
	}
	var nc *string
	if next != nil {
		s := string(*next)
		nc = &s
	}
	writeJSON(w, http.StatusOK, listEnvelope{Data: items, Meta: listMeta{RequestID: RequestID(r.Context()), NextCursor: nc}})
}
