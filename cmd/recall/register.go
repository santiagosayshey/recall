package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/santiagosayshey/recall/internal/config"
	"github.com/santiagosayshey/recall/internal/register"
)

// runRegister registers Recall on every instance in the config once and
// fails if any of them could not be made right.
func runRegister(args []string) int {
	fs := flag.NewFlagSet("register", flag.ContinueOnError)
	path := fs.String("config", envOr("RECALL_CONFIG", "/config/config.yml"), "path to config.yml")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	cfg, err := config.Load(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "register:", err)
		return exitFailed
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if !registerOnce(context.Background(), cfg, logger) {
		return exitFailed
	}
	return exitClean
}

// registerOnce registers every instance and logs each outcome. It reports
// whether all of them succeeded.
func registerOnce(ctx context.Context, cfg config.Config, logger *slog.Logger) bool {
	ok := true
	for _, r := range register.All(ctx, cfg) {
		if r.Err != nil {
			ok = false
			logger.Error("register", "instance", r.Instance, "err", r.Err)
			continue
		}
		logger.Info("registered", "instance", r.Instance, "connection", r.Action)
	}
	return ok
}

// registerUntilDone keeps registering until every instance is right or the
// context ends, backing off between rounds. An app that is still starting
// is the normal case after a reboot, not a failure.
func registerUntilDone(ctx context.Context, cfg config.Config, logger *slog.Logger) {
	delay := 5 * time.Second
	for {
		if registerOnce(ctx, cfg, logger) {
			return
		}
		logger.Info("register retry", "in", delay.String())
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		if delay < time.Minute {
			delay *= 2
		}
	}
}
