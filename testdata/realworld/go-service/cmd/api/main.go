package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/example/ledger/internal/config"
	"github.com/example/ledger/internal/handlers"
	"github.com/example/ledger/internal/httpx"
	"github.com/example/ledger/internal/store"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if v := os.Getenv("APP_VERSION"); v != "" {
		version = v
	}

	// Platforms like Heroku and Cloud Run inject PORT.
	if port, ok := os.LookupEnv("PORT"); ok && port != "" {
		cfg.HTTP.Addr = ":" + port
	}

	logger := httpx.NewLogger(cfg.LogLevel, cfg.Env)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(ctx, cfg.DB.URL, cfg.DB.MaxConns)
	if err != nil {
		return err
	}
	defer db.Close()

	srv := &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      handlers.New(db, cfg.JWTSecret, version).Routes(),
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
	}

	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", srv.Addr, "version", version)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
