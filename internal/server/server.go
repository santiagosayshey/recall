// Package server is the HTTP surface: a health check and the webhook
// endpoint the *arr apps post to. It is the only package that wires the
// others together.
package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
)

// maxBody bounds a webhook body. The largest seen so far is a few
// kilobytes; a megabyte leaves room for season packs.
const maxBody = 1 << 20

type Options struct {
	Logger *slog.Logger
}

type Server struct {
	log *slog.Logger
	mux *http.ServeMux
}

// New returns the handler. It accepts every webhook and logs it; storing and
// deciding come in later milestones.
func New(o Options) *Server {
	s := &Server{log: o.Logger, mux: http.NewServeMux()}
	if s.log == nil {
		s.log = slog.New(slog.DiscardHandler)
	}
	s.mux.HandleFunc("GET /healthz", s.health)
	s.mux.HandleFunc("POST /webhook", s.webhook)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, "ok\n")
}

// envelope is the little every *arr webhook shares.
type envelope struct {
	EventType    string `json:"eventType"`
	InstanceName string `json:"instanceName"`
}

func (s *Server) webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		http.Error(w, "body too large or unreadable", http.StatusRequestEntityTooLarge)
		return
	}
	var e envelope
	if err := json.Unmarshal(body, &e); err != nil || e.EventType == "" {
		http.Error(w, "not an *arr webhook", http.StatusBadRequest)
		return
	}
	s.log.Info("webhook", "event", e.EventType, "instance", e.InstanceName, "bytes", len(body))
	w.WriteHeader(http.StatusAccepted)
}
