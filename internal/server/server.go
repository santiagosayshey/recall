// Package server is the HTTP surface: a health check and the webhook
// endpoint the *arr apps post to. It is the only package that wires the
// others together.
package server

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/santiagosayshey/recall/internal/store"
	"github.com/santiagosayshey/recall/internal/webhook"
)

// maxBody bounds a webhook body. The largest seen so far is a few
// kilobytes; a megabyte leaves room for season packs.
const maxBody = 1 << 20

type Options struct {
	Logger *slog.Logger
	Store  *store.Store
}

type Server struct {
	log   *slog.Logger
	store *store.Store
	mux   *http.ServeMux
}

// New returns the handler. Grabs and imports go to the store; everything
// else is logged and dropped.
func New(o Options) *Server {
	s := &Server{log: o.Logger, store: o.Store, mux: http.NewServeMux()}
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

func (s *Server) webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		http.Error(w, "body too large or unreadable", http.StatusRequestEntityTooLarge)
		return
	}
	ev, err := webhook.Parse(body)
	if errors.Is(err, webhook.ErrNotWebhook) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	switch e := ev.(type) {
	case *webhook.Grab:
		err = s.store.AddGrab(*e)
		s.log.Info("grab", "instance", e.Instance, "download", e.DownloadID, "title", e.ReleaseTitle, "score", e.Score)
	case *webhook.Import:
		err = s.store.AddImport(*e)
		s.log.Info("import", "instance", e.Instance, "download", e.DownloadID, "file", e.FileName, "score", e.Score)
	case *webhook.Other:
		s.log.Info("ignored", "instance", e.Instance, "event", e.EventType)
	}
	if err != nil {
		s.log.Error("store", "err", err)
		http.Error(w, "could not store the event", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
