// Command server runs Talent-Bridge: the JSON API and the built frontend.
//
// Settings come from environment variables:
//
//	PORT          port to listen on (Cloud Run sets it; default 8080)
//	DATABASE_URL  Postgres connection string (Neon in production)
//	STATIC_DIR    the Vite build output (default web/dist)
//
// Sign-in (internal/auth, LoadConfig) also reads GOOGLE_CLIENT_ID,
// GOOGLE_CLIENT_SECRET, PUBLIC_URL, ADMIN_EMAILS and, for local development
// only, GOOGLE_ISSUER. Sign-in is off without a database or a Google client.
package main

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/v1d1tt/talent-bridge/internal/auth"
	"github.com/v1d1tt/talent-bridge/internal/database"
	"github.com/v1d1tt/talent-bridge/internal/health"
	"github.com/v1d1tt/talent-bridge/internal/server"
)

// version is set at build time: go build -ldflags "-X main.version=<git sha>".
var version = "dev"

func main() {
	// JSON logs. Cloud Logging reads "severity" and "message", so rename slog's keys.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.LevelKey:
				a.Key = "severity"
				if a.Value.Any() == slog.LevelWarn {
					a.Value = slog.StringValue("WARNING") // Cloud Logging's name for it
				}
			case slog.MessageKey:
				a.Key = "message"
			}
			return a
		},
	})))
	if err := run(); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var probe health.Probe = database.NotConfigured{}
	var signIn *auth.Service
	if url := os.Getenv("DATABASE_URL"); url != "" {
		db, err := database.Open(ctx, url)
		if err != nil {
			return err
		}
		defer db.Close()
		probe = db
		switch cfg, err := auth.LoadConfig(os.Getenv); {
		case errors.Is(err, auth.ErrNotConfigured):
			slog.Warn("sign-in is off: " + err.Error())
		case err != nil:
			return err // a wrong sign-in setting must stop the start, not be ignored
		default:
			signIn = auth.New(cfg, db.Queries)
			slog.Info("sign-in is on", "issuer", cfg.Issuer, "public_url", cfg.PublicURL, "admins", len(cfg.AdminEmails))
		}
		// Migrate in the background, so the first visitor does not wait for it.
		go func() {
			if _, err := db.Probe(ctx); err != nil {
				slog.Warn("startup database probe failed; the next health check retries", "err", err)
			}
		}()
	} else {
		slog.Warn("DATABASE_URL is not set; the health check reports the database as down")
	}

	srv := &http.Server{
		Addr:              ":" + cmp.Or(os.Getenv("PORT"), "8080"),
		Handler:           server.New(server.Config{StaticDir: cmp.Or(os.Getenv("STATIC_DIR"), "web/dist"), Probe: probe, Version: version, Auth: signIn}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", srv.Addr, "version", version)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		// Cloud Run sends SIGTERM and allows 10 s before it kills the container.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}
