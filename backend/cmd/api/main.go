// Command api is the entry point of the service. The dependency graph itself
// lives in internal/app so that the integration tests boot exactly the same
// wiring; what remains here is process-level concern only.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/app"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/config"
)

func main() {
	if err := run(); err != nil {
		log := zerolog.New(os.Stderr).With().Timestamp().Logger()
		log.Fatal().Err(err).Msg("startup failed")
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := newLogger(cfg)
	log.Info().Str("env", cfg.Env).Str("addr", cfg.HTTPAddr).Msg("starting")

	startupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	application, err := app.New(startupCtx, cfg, log)
	if err != nil {
		return err
	}
	defer func() {
		if err := application.Close(); err != nil {
			log.Warn().Err(err).Msg("closing the database pool")
		}
	}()

	return serve(application.Server(), log)
}

// serve runs the HTTP server and shuts it down gracefully on SIGINT/SIGTERM so
// in-flight postings finish rather than being cut off mid-transaction.
func serve(server *http.Server, log zerolog.Logger) error {
	errCh := make(chan error, 1)
	go func() {
		log.Info().Str("addr", server.Addr).Msg("listening")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case sig := <-stop:
		log.Info().Str("signal", sig.String()).Msg("shutting down")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}

func newLogger(cfg *config.Config) zerolog.Logger {
	level, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.TimeFieldFormat = time.RFC3339Nano

	// Structured JSON everywhere except a developer's terminal, where a
	// human-readable writer is far more useful.
	if cfg.Env == "DEV" {
		return zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
			Level(level).With().Timestamp().Logger()
	}
	return zerolog.New(os.Stdout).Level(level).With().Timestamp().
		Str("service", "sugar-planning").Str("env", cfg.Env).Logger()
}
