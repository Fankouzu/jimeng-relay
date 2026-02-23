package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

var migrationStatements = []string{
	`CREATE TABLE IF NOT EXISTS api_keys (
		id TEXT PRIMARY KEY,
		access_key TEXT NOT NULL,
		secret_key_hash TEXT NOT NULL,
		secret_key_ciphertext TEXT NOT NULL,
		description TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		expires_at TEXT,
		revoked_at TEXT,
		rotation_of TEXT,
		status TEXT NOT NULL
	);`,
	`ALTER TABLE api_keys ADD COLUMN secret_key_ciphertext TEXT NOT NULL DEFAULT '';`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_access_key ON api_keys(access_key);`,

	`CREATE TABLE IF NOT EXISTS downstream_requests (
		id TEXT PRIMARY KEY,
		request_id TEXT NOT NULL,
		api_key_id TEXT NOT NULL,
		action TEXT NOT NULL,
		method TEXT NOT NULL,
		path TEXT NOT NULL,
		query_string TEXT,
		headers TEXT,
		body TEXT,
		client_ip TEXT,
		received_at TEXT NOT NULL
	);`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_downstream_requests_request_id ON downstream_requests(request_id);`,

	`CREATE TABLE IF NOT EXISTS upstream_attempts (
		id TEXT PRIMARY KEY,
		request_id TEXT NOT NULL,
		attempt_number INTEGER NOT NULL,
		upstream_action TEXT NOT NULL,
		request_headers TEXT,
		request_body TEXT,
		response_status INTEGER NOT NULL,
		response_headers TEXT,
		response_body TEXT,
		latency_ms INTEGER NOT NULL,
		error TEXT,
		sent_at TEXT NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_upstream_attempts_request_id ON upstream_attempts(request_id);`,

	`CREATE TABLE IF NOT EXISTS audit_events (
		id TEXT PRIMARY KEY,
		request_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		actor TEXT,
		action TEXT NOT NULL,
		resource TEXT NOT NULL,
		metadata TEXT,
		created_at TEXT NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_audit_events_request_id_created_at ON audit_events(request_id, created_at);`,

	`CREATE TABLE IF NOT EXISTS idempotency_records (
		id TEXT PRIMARY KEY,
		idempotency_key TEXT NOT NULL,
		request_hash TEXT NOT NULL,
		response_status INTEGER NOT NULL,
		response_body TEXT,
		created_at TEXT NOT NULL,
		expires_at TEXT NOT NULL
	);`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_idempotency_records_idempotency_key ON idempotency_records(idempotency_key);`,
}

func ApplyMigrations(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return
		}
	}()

	for i, stmt := range migrationStatements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			if isSQLiteDuplicateColumn(err) {
				continue
			}
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func isSQLiteDuplicateColumn(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "duplicate column name")
}
