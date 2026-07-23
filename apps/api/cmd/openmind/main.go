package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/riverqueue/river"

	"github.com/rohithgilla12/openmind/api/internal/ai"
	"github.com/rohithgilla12/openmind/api/internal/api"
	"github.com/rohithgilla12/openmind/api/internal/assets"
	"github.com/rohithgilla12/openmind/api/internal/auth"
	"github.com/rohithgilla12/openmind/api/internal/enrich"
	"github.com/rohithgilla12/openmind/api/internal/feeds"
	"github.com/rohithgilla12/openmind/api/internal/geo"
	"github.com/rohithgilla12/openmind/api/internal/jobs"
	"github.com/rohithgilla12/openmind/api/internal/mailer"
	appmcp "github.com/rohithgilla12/openmind/api/internal/mcp"
	"github.com/rohithgilla12/openmind/api/internal/pdftext"
	"github.com/rohithgilla12/openmind/api/internal/reelmedia"
	"github.com/rohithgilla12/openmind/api/internal/store"
)

// errStdioSingleUser is returned when `openmind mcp` starts in Clerk
// multi-user mode: stdio has no per-request credential, so it can only ever
// act as the single auto-provisioned account.
var errStdioSingleUser = errors.New("stdio transport is single-user only — use the HTTP transport at /mcp with a device key")

// checkStdioAuthMode refuses the stdio transport outside single-user token
// mode. It only inspects AUTH_MODE — a bad mode string fails later in
// authConfigFromEnv as usual.
func checkStdioAuthMode() error {
	if os.Getenv("AUTH_MODE") == api.AuthModeClerk {
		return errStdioSingleUser
	}
	return nil
}

const (
	defaultAssetsDir      = "/data/assets"
	defaultAssetsMaxBytes = 10 << 20 // 10 MiB
	defaultSMTPPort       = 587
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
		return fmt.Errorf("usage: openmind <serve|work|all|migrate|mcp>")
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

	// PDF support degrades gracefully: a wasm init failure logs and leaves
	// pipeline.PDF nil, so PDF items simply fall through to normal handling
	// instead of failing the process.
	if pdfx, err := pdftext.New(); err != nil {
		slog.Error("initialising pdf text extractor; pdf support disabled", "err", err)
	} else {
		pipeline.PDF = pdfx
	}

	authCfg, err := authConfigFromEnv()
	if err != nil {
		return err
	}

	trustedProxies, err := api.ParseTrustedProxies(os.Getenv("TRUSTED_PROXIES"))
	if err != nil {
		return fmt.Errorf("parsing TRUSTED_PROXIES: %w", err)
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

	// The feed service and River client are mutually dependent: the poll worker
	// drives the service, and the service enqueues enrichment through the client.
	// Build the service first, construct the client with it, then set the client
	// back on the service (River is a settable field) so enqueue works.
	feedSvc := feeds.NewService(s)

	kindleDeps := kindleDepsFromEnv()
	if kindleDeps.Configured {
		if kindleDeps.To != "" {
			slog.Info("send-to-kindle configured", "to", kindleDeps.To)
		} else {
			slog.Info("send-to-kindle configured — no server-wide KINDLE_EMAIL fallback; each user must set their own Kindle address in Settings")
		}
	} else {
		slog.Info("send-to-kindle not configured — set SMTP_HOST and SMTP_FROM to enable")
	}

	// Geocoding for extracted places is optional (GEOCODER env). Only the
	// worker processes use it, but building it up front keeps config errors
	// loud on every command.
	geocoder, err := geo.FromEnv()
	if err != nil {
		return err
	}
	if geocoder != nil {
		slog.Info("geocoder ready", "geocoder", geocoder.Name())
	}

	// REEL_MEDIA controls the vision ladder for place extraction from social
	// videos: off (caption + location only), thumbnail (default, adds
	// thumbnail vision), or video (adds a deep-media rung sampling frames via
	// yt-dlp+ffmpeg). ModeVideo downgrades to thumbnail if those binaries are
	// missing, so the deep-media rung is only ever wired up when it can
	// actually run.
	reelMode := reelmedia.ModeFromEnv()
	var reelExtractor *reelmedia.Extractor
	if reelMode == reelmedia.ModeVideo {
		ext, ok := reelmedia.Detect()
		if !ok {
			slog.Warn("REEL_MEDIA=video but yt-dlp/ffmpeg not found on PATH; downgrading to thumbnail")
			reelMode = reelmedia.ModeThumbnail
		} else {
			reelExtractor = ext
		}
	}
	slog.Info("reel media", "mode", reelMode.String(), "deep_media", reelExtractor != nil)

	switch cmd {
	case "serve":
		client, err := jobs.NewRiverClient(pool, pipeline, feedSvc, kindleDeps, geocoder, reelMode, reelExtractor, false)
		if err != nil {
			return err
		}
		feedSvc.River = client
		return serveHTTP(ctx, s, client, provider, authCfg, assetStore, assetMaxBytes, feedSvc, kindleConfigFromDeps(kindleDeps), trustedProxies)
	case "work":
		client, err := jobs.NewRiverClient(pool, pipeline, feedSvc, kindleDeps, geocoder, reelMode, reelExtractor, true)
		if err != nil {
			return err
		}
		feedSvc.River = client
		return work(ctx, client)
	case "all":
		client, err := jobs.NewRiverClient(pool, pipeline, feedSvc, kindleDeps, geocoder, reelMode, reelExtractor, true)
		if err != nil {
			return err
		}
		feedSvc.River = client
		return all(ctx, s, client, provider, authCfg, assetStore, assetMaxBytes, feedSvc, kindleConfigFromDeps(kindleDeps), trustedProxies)
	case "mcp":
		if err := checkStdioAuthMode(); err != nil {
			return err
		}
		client, err := jobs.NewRiverClient(pool, pipeline, feedSvc, kindleDeps, geocoder, reelMode, reelExtractor, false)
		if err != nil {
			return err
		}
		feedSvc.River = client
		return serveStdio(ctx, s, client, provider)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

// kindleDepsFromEnv reads the Send-to-Kindle SMTP configuration from the
// environment. Configured is true when SMTP_HOST and SMTP_FROM (the SMTP
// transport) are both set — without them there's no way to send mail at
// all, so the feature stays off regardless of KINDLE_EMAIL. KINDLE_EMAIL is
// optional: when set it becomes Deps.To, the server-wide fallback recipient
// used when a user hasn't set their own kindle_email setting; when unset,
// Deps.To is empty and every send relies on a per-user setting instead.
// SMTP_PASSWORD is intentionally never logged.
func kindleDepsFromEnv() jobs.KindleDeps {
	host := os.Getenv("SMTP_HOST")
	from := os.Getenv("SMTP_FROM")
	to := os.Getenv("KINDLE_EMAIL")
	if host == "" || from == "" {
		return jobs.KindleDeps{}
	}
	port := defaultSMTPPort
	if v := os.Getenv("SMTP_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			port = n
		} else {
			slog.Warn("invalid SMTP_PORT; using default", "value", v)
		}
	}
	cfg := mailer.SMTPConfig{
		Host:     host,
		Port:     port,
		Username: os.Getenv("SMTP_USERNAME"),
		Password: os.Getenv("SMTP_PASSWORD"),
		From:     from,
	}
	return jobs.KindleDeps{Mailer: mailer.New(cfg), To: to, Configured: true}
}

// kindleConfigFromDeps translates the worker-facing jobs.KindleDeps into the
// api package's KindleConfig: SMTPConfigured mirrors Deps.Configured (the
// SMTP transport), EnvRecipient reports whether a server-wide KINDLE_EMAIL
// fallback recipient is also set.
func kindleConfigFromDeps(d jobs.KindleDeps) api.KindleConfig {
	return api.KindleConfig{SMTPConfigured: d.Configured, EnvRecipient: d.To != ""}
}

// authConfigFromEnv builds the API's AuthConfig from AUTH_MODE, OPENMIND_TOKEN,
// and CLERK_ISSUER. AUTH_MODE defaults to "token". In clerk mode, CLERK_ISSUER
// is required — its absence is a startup error rather than a silent
// fallback, since starting an unauthenticated exchange server by accident
// would be worse than failing fast. In token mode with no OPENMIND_TOKEN set,
// the API stays unauthenticated (single-user self-host), with a warning.
func authConfigFromEnv() (api.AuthConfig, error) {
	mode := os.Getenv("AUTH_MODE")
	if mode == "" {
		mode = api.AuthModeToken
	}

	switch mode {
	case api.AuthModeClerk:
		issuer := os.Getenv("CLERK_ISSUER")
		if issuer == "" {
			return api.AuthConfig{}, fmt.Errorf("AUTH_MODE=clerk requires CLERK_ISSUER to be set")
		}
		return api.AuthConfig{Mode: api.AuthModeClerk, Verifier: auth.NewJWTVerifier(issuer)}, nil
	case api.AuthModeToken:
		token := os.Getenv("OPENMIND_TOKEN")
		if token == "" {
			slog.Warn("API is unauthenticated — set OPENMIND_TOKEN before exposing it")
		}
		return api.AuthConfig{Mode: api.AuthModeToken, LegacyToken: token}, nil
	default:
		return api.AuthConfig{}, fmt.Errorf("unknown AUTH_MODE %q (want %q or %q)", mode, api.AuthModeToken, api.AuthModeClerk)
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
func serveHTTP(ctx context.Context, s *store.Store, client *riverClient, provider ai.Provider, authCfg api.AuthConfig, assetStore *assets.FSStore, assetMaxBytes int64, feedSvc *feeds.Service, kindleCfg api.KindleConfig, trustedProxies []*net.IPNet) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{Addr: ":" + port(), Handler: api.NewServer(s, client, provider, authCfg, assetStore, assetMaxBytes, feedSvc, kindleCfg, trustedProxies)}
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

// serveStdio serves the MCP registry over stdin/stdout as the single dev
// user, until the client closes the pipe or the process is signalled. All
// logging must stay on stderr (slog's default) — stdout is the JSON-RPC
// channel. The embedded River client is insert-only: saves enqueue
// enrichment, but a separate `openmind work` or `all` process must be
// running to process the queue.
func serveStdio(ctx context.Context, s *store.Store, client *riverClient, provider ai.Provider) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	backend := api.NewMCPBackend(s, client, provider)
	server := appmcp.NewServer(backend, func(context.Context) uuid.UUID { return api.DevUserID })
	slog.Info("mcp stdio transport serving", "user", api.DevUserID)
	if err := server.Run(ctx, &sdkmcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("mcp stdio: %w", err)
	}
	return nil
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
func all(ctx context.Context, s *store.Store, client *riverClient, provider ai.Provider, authCfg api.AuthConfig, assetStore *assets.FSStore, assetMaxBytes int64, feedSvc *feeds.Service, kindleCfg api.KindleConfig, trustedProxies []*net.IPNet) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("starting river workers: %w", err)
	}
	slog.Info("river workers started")

	srv := &http.Server{Addr: ":" + port(), Handler: api.NewServer(s, client, provider, authCfg, assetStore, assetMaxBytes, feedSvc, kindleCfg, trustedProxies)}
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
