package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jimeng-relay/server/internal/config"
	apikeyhandler "github.com/jimeng-relay/server/internal/handler/apikey"
	relayhandler "github.com/jimeng-relay/server/internal/handler/relay"
	"github.com/jimeng-relay/server/internal/logging"
	"github.com/jimeng-relay/server/internal/middleware/observability"
	"github.com/jimeng-relay/server/internal/middleware/sigv4"
	"github.com/jimeng-relay/server/internal/relay/upstream"
	"github.com/jimeng-relay/server/internal/repository"
	"github.com/jimeng-relay/server/internal/repository/postgres"
	"github.com/jimeng-relay/server/internal/repository/sqlite"
	"github.com/jimeng-relay/server/internal/secretcrypto"
	apikeyservice "github.com/jimeng-relay/server/internal/service/apikey"
	auditservice "github.com/jimeng-relay/server/internal/service/audit"
	idempotencyservice "github.com/jimeng-relay/server/internal/service/idempotency"
)

func main() {
	if err := runServer(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func runServer() error {
	cfg, err := config.Load(config.Options{})
	if err != nil {
		return fmt.Errorf("missing required configuration: %w", err)
	}

	logger := logging.NewLogger(slog.LevelInfo)

	ctx := context.Background()
	repos, cleanup, err := openRepositories(ctx, cfg)
	if err != nil {
		return err
	}
	defer cleanup()
	secretCipher, err := newSecretCipher(cfg.APIKeyEncryptionKey)
	if err != nil {
		return err
	}

	apikeySvc := apikeyservice.NewService(repos.APIKeys, apikeyservice.Config{SecretCipher: secretCipher})
	auditSvc := auditservice.NewService(repos.DownstreamRequests, repos.UpstreamAttempts, repos.AuditEvents, auditservice.Config{})
	idempotencySvc := idempotencyservice.NewService(repos.IdempotencyRecords, idempotencyservice.Config{})
	upstreamClient, err := upstream.NewClient(cfg, upstream.Options{})
	if err != nil {
		return fmt.Errorf("init upstream client: %w", err)
	}
	authn := sigv4.New(repos.APIKeys, sigv4.Config{SecretCipher: secretCipher, ExpectedRegion: cfg.Region, ExpectedService: "cv"})
	app := http.NewServeMux()
	apikeyRoutes := apikeyhandler.NewHandler(apikeySvc, logger).Routes()
	submitRoutes := relayhandler.NewSubmitHandler(upstreamClient, auditSvc, idempotencySvc, repos.IdempotencyRecords, logger).Routes()
	getResultRoutes := relayhandler.NewGetResultHandler(upstreamClient, auditSvc, logger).Routes()
	app.Handle("/v1/keys", apikeyRoutes)
	app.Handle("/v1/keys/", apikeyRoutes)
	app.Handle("/v1/submit", submitRoutes)
	app.Handle("/v1/get-result", getResultRoutes)
	app.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("Action")
		switch action {
		case "CVSync2AsyncSubmitTask":
			submitRoutes.ServeHTTP(w, r)
		case "CVSync2AsyncGetResult":
			getResultRoutes.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})

	obs := observability.Middleware(logger)
	mux := http.NewServeMux()
	mux.Handle("/", obs(authn(app)))

	log.Printf("Starting jimeng-relay server on port %s...", cfg.ServerPort)
	log.Printf("Registered API Key routes: POST/GET /v1/keys, POST /v1/keys/{id}/revoke, POST /v1/keys/{id}/rotate")
	log.Printf("Registered relay submit routes: POST /v1/submit, POST /?Action=CVSync2AsyncSubmitTask")
	log.Printf("Registered relay get-result routes: POST /v1/get-result, POST /?Action=CVSync2AsyncGetResult")
	if err := http.ListenAndServe(":"+cfg.ServerPort, mux); err != nil {
		return fmt.Errorf("listen on :%s: %w", cfg.ServerPort, err)
	}
	return nil
}

type repositories struct {
	APIKeys            repository.APIKeyRepository
	DownstreamRequests repository.DownstreamRequestRepository
	UpstreamAttempts   repository.UpstreamAttemptRepository
	AuditEvents        repository.AuditEventRepository
	IdempotencyRecords repository.IdempotencyRecordRepository
}

func openRepositories(ctx context.Context, cfg config.Config) (repositories, func(), error) {
	dbType := strings.ToLower(strings.TrimSpace(cfg.DatabaseType))
	switch dbType {
	case "", "sqlite":
		repos, err := sqlite.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			return repositories{}, nil, fmt.Errorf("open sqlite repository: %w", err)
		}
		cleanup := func() { _ = repos.Close() }
		return repositories{APIKeys: repos.APIKeys, DownstreamRequests: repos.DownstreamRequests, UpstreamAttempts: repos.UpstreamAttempts, AuditEvents: repos.AuditEvents, IdempotencyRecords: repos.IdempotencyRecords}, cleanup, nil
	case "postgres", "postgresql":
		db, err := postgres.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			return repositories{}, nil, fmt.Errorf("open postgres repository: %w", err)
		}
		return repositories{APIKeys: db.APIKeys(), DownstreamRequests: db.DownstreamRequests(), UpstreamAttempts: db.UpstreamAttempts(), AuditEvents: db.AuditEvents(), IdempotencyRecords: db.IdempotencyRecords()}, db.Close, nil
	default:
		return repositories{}, nil, fmt.Errorf("unsupported database_type: %s", cfg.DatabaseType)
	}
}

func newSecretCipher(encodedKey string) (secretcrypto.Cipher, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encodedKey))
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", config.EnvAPIKeyEncryptionKey, err)
	}
	c, err := secretcrypto.NewAESCipher(raw)
	if err != nil {
		return nil, fmt.Errorf("init api key secret cipher: %w", err)
	}
	return c, nil
}
