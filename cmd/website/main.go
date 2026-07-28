// Command website serves itemize.no.
//
// Everything the server needs at runtime — templates, stylesheets, scripts,
// fonts, images and the editable YAML content — is compiled into the binary
// via embed.FS, so deployment is a single file with no adjacent assets.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// Embeds the timezone database. Every date on this site is rendered in
	// Europe/Oslo, and the distroless base image the container is built on
	// carries no system zoneinfo — without this every event would silently
	// display in UTC.
	_ "time/tzdata"

	"github.com/ItemizeNTNU/website/assets"
	"github.com/ItemizeNTNU/website/internal/api"
	"github.com/ItemizeNTNU/website/internal/config"
	"github.com/ItemizeNTNU/website/internal/events"
	"github.com/ItemizeNTNU/website/internal/httpx"
	"github.com/ItemizeNTNU/website/internal/store"
	"github.com/ItemizeNTNU/website/internal/web"
)

// version is overwritten at build time with -ldflags="-X main.version=$GIT_SHA".
var version = "dev"

func main() {
	dev := flag.Bool("dev", false,
		"read templates, stylesheets and content from disk instead of the embedded copies")
	flag.Parse()

	if err := run(*dev); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(devFlag bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	dev := cfg.Dev || devFlag

	log := newLogger(dev)
	slog.SetDefault(log)
	log.Info("starting", "version", version, "config", cfg)

	// Fail loudly rather than silently rendering every date in UTC.
	if _, err := time.LoadLocation("Europe/Oslo"); err != nil {
		return errors.New("timezone database unavailable: " + err.Error())
	}

	fsys := assets.FS(dev)

	assetServer, err := httpx.NewAssets(fsys, dev)
	if err != nil {
		return err
	}

	repo, disconnect, err := openEvents(cfg, log, dev)
	if err != nil {
		return err
	}
	defer disconnect()

	site, err := web.NewServer(fsys, assetServer, repo, log, dev)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	assetServer.Register(mux)
	site.Routes(mux)

	if repo != nil {
		api.NewServer(repo, cfg.BaseURL.String(), log).Routes(mux)
	}

	https := cfg.BaseURL.Scheme == "https"
	handler := httpx.Chain(mux,
		httpx.RequestID,
		httpx.Logger(log),
		httpx.Recover(log, site.ErrorPage),
		httpx.SecurityHeaders(https),
		httpx.Gzip,
		httpx.TrailingSlash,
		httpx.CookieRecipe,
		web.ClearFlash,
	)

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: handler,
		// A request that never finishes should not hold a connection open
		// forever. ReadHeaderTimeout in particular is what closes a slowloris.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	return serve(srv, log)
}

// serve runs the server until a termination signal arrives, then stops
// accepting connections and lets in-flight requests finish.
func serve(srv *http.Server, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// openEvents connects to MongoDB and returns the event repository.
//
// A database that will not answer is fatal in production — the calendar is
// half the reason the site exists, and a deployment that silently comes up
// without it should not be mistaken for a healthy one. In development it is a
// warning instead, so the content pages can be worked on without running a
// database at all.
func openEvents(cfg *config.Config, log *slog.Logger, dev bool) (events.Repository, func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client, db, err := store.Connect(ctx, cfg.Mongo)
	if err != nil {
		if !dev {
			return nil, func() {}, err
		}
		log.Warn("no database; the calendar will be unavailable", "err", err)
		return nil, func() {}, nil
	}

	repo := events.NewMongoRepo(db, log)
	if err := repo.EnsureIndexes(ctx); err != nil {
		return nil, func() {}, err
	}

	disconnect := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := client.Disconnect(shutdownCtx); err != nil {
			log.Warn("disconnecting from MongoDB failed", "err", err)
		}
	}
	return repo, disconnect, nil
}

func healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte("ok " + version + "\n"))
}

func newLogger(dev bool) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if dev {
		opts.Level = slog.LevelDebug
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
