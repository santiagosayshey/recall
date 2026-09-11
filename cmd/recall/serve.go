package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/santiagosayshey/recall/internal/server"
	"github.com/santiagosayshey/recall/internal/store"
)

// runServe receives webhooks until told to stop. Each flag falls back to
// RECALL_<NAME>.
func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	listen := fs.String("listen", envOr("RECALL_LISTEN", ":8471"), "address to serve HTTP on")
	data := fs.String("data", envOr("RECALL_DATA", "/data"), "directory the grab and import files live in")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	st, err := store.Open(*data, store.Options{})
	if err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
		return exitFailed
	}
	defer st.Close()
	l, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
		return exitFailed
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := &http.Server{
		Handler:           server.New(server.Options{Logger: logger, Store: st}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	logger.Info("recall serving", "version", version, "listen", l.Addr().String(), "data", *data, "grabs", st.Grabs())
	if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, "serve:", err)
		return exitFailed
	}
	return exitClean
}
