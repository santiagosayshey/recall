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

	"github.com/santiagosayshey/recall/internal/config"
	"github.com/santiagosayshey/recall/internal/server"
	"github.com/santiagosayshey/recall/internal/store"
)

// runServe receives webhooks until told to stop. Each flag falls back to
// RECALL_<NAME>. With a config file it also registers itself on every
// instance and refuses events without the secret; without one it only
// receives, which is how the replay tests run it.
func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	listen := fs.String("listen", envOr("RECALL_LISTEN", ":8471"), "address to serve HTTP on")
	data := fs.String("data", envOr("RECALL_DATA", "/data"), "directory the grab, import and decision files live in")
	path := fs.String("config", envOr("RECALL_CONFIG", "/config/config.yml"), "path to config.yml; serve without one if the default is absent")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	var cfg config.Config
	hasConfig := true
	if c, err := config.Load(*path); err == nil {
		cfg = c
	} else if errors.Is(err, os.ErrNotExist) && !flagSet(fs, "config") && os.Getenv("RECALL_CONFIG") == "" {
		hasConfig = false
	} else {
		fmt.Fprintln(os.Stderr, "serve:", err)
		return exitFailed
	}
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
		Handler:           server.New(server.Options{Logger: logger, Store: st, Secret: cfg.Secret}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	logger.Info("recall serving", "version", version, "listen", l.Addr().String(), "data", *data, "grabs", st.Grabs(), "config", hasConfig)
	if hasConfig {
		go registerUntilDone(ctx, cfg, logger)
	}
	if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, "serve:", err)
		return exitFailed
	}
	return exitClean
}

// flagSet reports whether the flag was given on the command line.
func flagSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}
