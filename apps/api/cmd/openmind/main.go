package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/rohithgilla12/openmind/api/internal/ai"
	"github.com/rohithgilla12/openmind/api/internal/api"
	"github.com/rohithgilla12/openmind/api/internal/assets"
	"github.com/rohithgilla12/openmind/api/internal/enrich"
	"github.com/rohithgilla12/openmind/api/internal/jobs"
	"github.com/rohithgilla12/openmind/api/internal/store"
)

const (
	defaultAssetsDir      = "/data/assets"
	defaultAssetsMaxBytes = 10 << 20 // 10 MiB
)

// riverClient is the concrete River client type used across this process.
type riverClient = river.Client[pgx.Tx]

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		slog.Error("openmind failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: openmind <serve|work|all|migrate>")
	}
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	cmd := args[0]
	if cmd == "migrate" {
		return store.Migrate(ctx, pool)
	}

	// Auto-migrate on startup for serve|work|all so a fresh `docker compose up`
	// works on an empty volume. Migrate is idempotent and transactional.
	if err := store.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	s := store.New(pool)
	if err := s.Queries.EnsureUser(ctx, api.DevUserID); err != nil {
		return fmt.Errorf("provisioning dev user: %w", err)
	}
	provider, err := ai.FromEnv(ctx)
	if err != nil {
		return fmt.Errorf("building ai provider: %w", err)
	}
	slog.Info("ai provider ready", "provider", provider.Name())
	pipeline := &enrich.Pipeline{Store: s, AI: provider, Extractor: enrich.NewTrafilatura(nil)}

	token := os.Getenv("OPENMIND_TOKEN")
	if token == "" {
		slog.Warn("API is unauthenticated — set OPENMIND_TOKEN before exposing it")
	}

	assetsDir := os.Getenv("ASSETS_DIR")
	if assetsDir == "" {
		assetsDir = defaultAssetsDir
	}
	assetStore, err := assets.NewFSStore(assetsDir)
	if err != nil {
		return fmt.Errorf("initialising asset store: %w", err)
	}
	assetMaxBytes := assetMaxBytesFromEnv()
	slog.Info("asset store ready", "dir", assetsDir, "max_bytes", assetMaxBytes)

	// Give the pipeline read access to uploaded blobs so it can extract a colour
	// palette from lead images during enrichment.
	pipeline.Assets = assetStore

	switch cmd {
	case "serve":
		client, err := jobs.NewRiverClient(pool, pipeline, false)
		if err != nil {
			return err
		}
		return serveHTTP(ctx, s, client, provider, token, assetStore, assetMaxBytes)
	case "work":
		client, err := jobs.NewRiverClient(pool, pipeline, true)
		if err != nil {
			return err
		}
		return work(ctx, client)
	case "all":
		client, err := jobs.NewRiverClient(pool, pipeline, true)
		if err != nil {
			return err
		}
		return all(ctx, s, client, provider, token, assetStore, assetMaxBytes)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func port() string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	return "8080"
}

// assetMaxBytesFromEnv reads ASSETS_MAX_BYTES, falling back to the 10 MiB
// default when unset or invalid.
func assetMaxBytesFromEnv() int64 {
	if v := os.Getenv("ASSETS_MAX_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
		slog.Warn("invalid ASSETS_MAX_BYTES; using default", "value", v)
	}
	return defaultAssetsMaxBytes
}

// serveHTTP runs the API only (insert-only River client), shutting down
// gracefully on SIGINT/SIGTERM.
func serveHTTP(ctx context.Context, s *store.Store, client *riverClient, provider ai.Provider, token string, assetStore *assets.FSStore, assetMaxBytes int64) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{Addr: ":" + port(), Handler: api.NewServer(s, client, provider, token, assetStore, assetMaxBytes)}
	errc := make(chan error, 1)
	go func() {
		slog.Info("http server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	select {
	case err := <-errc:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		slog.Info("shutting down http server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// work runs the River worker only.
func work(ctx context.Context, client *riverClient) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("starting river workers: %w", err)
	}
	slog.Info("river workers started")
	<-ctx.Done()
	slog.Info("stopping river workers")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return client.Stop(shutdownCtx)
}

// all runs both the River workers and the HTTP API in one process.
func all(ctx context.Context, s *store.Store, client *riverClient, provider ai.Provider, token string, assetStore *assets.FSStore, assetMaxBytes int64) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("starting river workers: %w", err)
	}
	slog.Info("river workers started")

	srv := &http.Server{Addr: ":" + port(), Handler: api.NewServer(s, client, provider, token, assetStore, assetMaxBytes)}
	errc := make(chan error, 1)
	go func() {
		slog.Info("http server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	var runErr error
	select {
	case runErr = <-errc:
	case <-ctx.Done():
	}

	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("http shutdown", "err", err)
	}
	if err := client.Stop(shutdownCtx); err != nil {
		slog.Error("river stop", "err", err)
	}
	return runErr
}
