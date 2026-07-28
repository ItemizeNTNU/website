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
	"fmt"
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
	"github.com/ItemizeNTNU/website/internal/auth"
	"github.com/ItemizeNTNU/website/internal/config"
	"github.com/ItemizeNTNU/website/internal/discord"
	"github.com/ItemizeNTNU/website/internal/events"
	"github.com/ItemizeNTNU/website/internal/fusionauth"
	"github.com/ItemizeNTNU/website/internal/httpx"
	"github.com/ItemizeNTNU/website/internal/store"
	"github.com/ItemizeNTNU/website/internal/web"
)

// version is overwritten at build time with -ldflags="-X main.version=$GIT_SHA".
var version = "dev"

func main() {
	fromDisk := flag.Bool("dev", false,
		"read templates, stylesheets and content from disk instead of the embedded copies")
	check := flag.Bool("healthcheck", false,
		"probe the running server and exit 0 if healthy (used by the container health check)")
	flag.Parse()

	if *check {
		os.Exit(healthcheck())
	}

	if err := run(*fromDisk); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// run starts the server.
//
// fromDisk is separate from cfg.Dev on purpose. They used to be the same
// switch, which meant a container started without ENV=production — and
// "development" is the default — tried to read templates and content from a
// directory the image does not contain, and died with a confusing error about
// a missing YAML file. They are two different questions:
//
//   - cfg.Dev: is this production? Governs TLS requirements, log format, and
//     whether a .env file is read at all.
//   - fromDisk: should assets be read from the working directory instead of
//     the binary? Only ever true when a developer passes -dev.
func run(fromDisk bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := newLogger(cfg.Dev)
	slog.SetDefault(log)
	log.Info("starting", "version", version, "config", cfg)

	// Fail loudly rather than silently rendering every date in UTC.
	if _, err := time.LoadLocation("Europe/Oslo"); err != nil {
		return errors.New("timezone database unavailable: " + err.Error())
	}

	fsys := assets.FS(fromDisk)

	assetServer, err := httpx.NewAssets(fsys, fromDisk)
	if err != nil {
		return err
	}

	repo, disconnect, err := openEvents(cfg, log, cfg.Dev)
	if err != nil {
		return err
	}
	defer disconnect()

	// A nil client means Discord is not configured: event sync is skipped and
	// the site works without a bot token.
	discordClient := discord.New(cfg.Discord, log)
	if !discordClient.Enabled() {
		log.Warn("Discord is not configured; events will not be announced there")
	}

	var eventSvc *events.Service
	if repo != nil {
		eventSvc = events.NewService(repo, discordClient, log)
	}

	fusionClient := fusionauth.New(cfg.FusionAuth.Host.String(), cfg.FusionAuth.APIToken)
	if !fusionClient.Configured() {
		log.Warn("no FusionAuth API token; registration will be unavailable")
	}

	site, err := web.NewServer(fsys, assetServer, repo, eventSvc, fusionClient,
		cfg.BaseURL.String(), log, fromDisk)
	if err != nil {
		return err
	}

	https := cfg.BaseURL.Scheme == "https"

	sealer, err := auth.NewSealer(cfg.FusionAuth.Secret, https)
	if err != nil {
		return err
	}
	authn, err := auth.New(context.Background(), cfg, sealer, log)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	authn.Routes(mux)
	assetServer.Register(mux)
	site.Routes(mux)

	if repo != nil {
		api.NewServer(repo, fusionClient, cfg.BaseURL.String(), log).Routes(mux)
	}

	handler := httpx.Chain(mux,
		httpx.RequestID,
		httpx.Logger(log),
		httpx.Recover(log, site.ErrorPage),
		httpx.SecurityHeaders(https),
		httpx.Gzip,
		httpx.TrailingSlash,
		httpx.CookieRecipe,
		// Before anything that renders a page, so every handler and template
		// sees the signed-in member.
		authn.Inject,
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

// healthcheck probes the local server and reports whether it is serving.
//
// The container image has no shell and no curl — that is the point of a
// distroless base — so the binary doubles as its own health probe.
func healthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = os.Getenv("LISTEN")
	}
	if port == "" {
		port = "3000"
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
		return 1
	}
	return 0
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
