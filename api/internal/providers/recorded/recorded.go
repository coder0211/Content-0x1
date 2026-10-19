// Package recorded serves recorded vendor responses (testdata fixtures) from
// an httptest server so adapter contract tests run offline (SPEC-02 §4.4).
// Every request is captured for assertions on the outgoing payload.
package recorded

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Reply is one canned response.
type Reply struct {
	Status  int
	Header  map[string]string
	File    string // testdata file name (relative to the test package)
	Body    []byte // used when File is empty
	Handler http.HandlerFunc
}

// Captured is a request received by the server.
type Captured struct {
	Method string
	Path   string
	Query  string
	Header http.Header
	Body   []byte
}

// Server is a fixture server.
type Server struct {
	*httptest.Server
	mu       sync.Mutex
	Requests []Captured
}

// Route matches "METHOD /path" (exact) to a reply.
type Route map[string]Reply

// New starts a fixture server; unknown routes answer 404.
func New(t *testing.T, routes Route) *Server {
	t.Helper()
	s := &Server{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.Requests = append(s.Requests, Captured{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Header: r.Header.Clone(), Body: b})
		s.mu.Unlock()
		rep, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			http.Error(w, `{"error":{"message":"no fixture"}}`, http.StatusNotFound)
			return
		}
		if rep.Handler != nil {
			r.Body = io.NopCloser(bytes.NewReader(b))
			rep.Handler(w, r)
			return
		}
		body := rep.Body
		if rep.File != "" {
			var err error
			body, err = os.ReadFile(filepath.Join("testdata", rep.File))
			if err != nil {
				t.Errorf("fixture %s: %v", rep.File, err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		for k, v := range rep.Header {
			w.Header().Set(k, v)
		}
		st := rep.Status
		if st == 0 {
			st = http.StatusOK
		}
		w.WriteHeader(st)
		_, _ = w.Write(body)
	}))
	t.Cleanup(s.Close)
	return s
}

// Last returns the last captured request.
func (s *Server) Last() Captured {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Requests) == 0 {
		return Captured{}
	}
	return s.Requests[len(s.Requests)-1]
}

// Count returns how many requests were received.
func (s *Server) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Requests)
}
