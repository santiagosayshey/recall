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
)

// runServe receives webhooks until told to stop. Each flag falls back to
// RECALL_<NAME>.
func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	listen := fs.String("listen", envOr("RECALL_LISTEN", ":8471"), "address to serve HTTP on")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	l, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
		return exitFailed
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := &http.Server{
		Handler:           server.New(server.Options{Logger: logger}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	logger.Info("recall serving", "version", version, "listen", l.Addr().String())
	if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, "serve:", err)
		return exitFailed
	}
	return exitClean
}
