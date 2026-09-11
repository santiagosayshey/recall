// Package server is the HTTP surface: a health check and the webhook
// endpoint the *arr apps post to. It is the only package that wires the
// others together.
package server

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/santiagosayshey/recall/internal/decide"
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

// New returns the handler. A grab goes to the store. An import goes to the
// store, is weighed against its grab, and the decision goes to the store
// and to the log. Everything else is logged and dropped.
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
		s.log.Info("grab", "instance", e.Instance, "media", e.Media.String(), "release", e.ReleaseTitle, "score", e.Score, "download", e.DownloadID)
	case *webhook.Import:
		err = s.imported(*e)
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

// imported stores the import, decides it against its grab, stores the
// decision, and logs it. The log line is the alert: result first so a
// watcher can match on it, then everything a person needs to act.
func (s *Server) imported(i webhook.Import) error {
	if err := s.store.AddImport(i); err != nil {
		return err
	}
	var grab *webhook.Grab
	if g, ok := s.store.Grab(i.DownloadID); ok {
		grab = &g
	}
	d := decide.Decide(grab, i)
	if err := s.store.AddDecision(d); err != nil {
		return err
	}
	s.log.Info("decision",
		"result", d.Result,
		"instance", d.Instance,
		"media", d.Media.String(),
		"release", d.ReleaseTitle,
		"file", d.FileName,
		"grabScore", d.GrabScore,
		"importScore", d.ImportScore,
		"delta", d.Delta,
		"lost", d.Lost,
		"gained", d.Gained,
		"path", d.Path,
		"download", d.DownloadID,
	)
	return nil
}
