package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	name := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", name)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	return db
}

func requireSQLiteObjectExists(t *testing.T, db *sql.DB, typ, name string) {
	t.Helper()

	var got string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = ? AND name = ?`, typ, name).Scan(&got)
	if err != nil {
		t.Fatalf("expected %s %q to exist: %v", typ, name, err)
	}
}

func TestApplyMigrations_Idempotent(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("ApplyMigrations (first): %v", err)
	}
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("ApplyMigrations (second): %v", err)
	}

	requireSQLiteObjectExists(t, db, "table", "api_keys")
	requireSQLiteObjectExists(t, db, "table", "downstream_requests")
	requireSQLiteObjectExists(t, db, "table", "upstream_attempts")
	requireSQLiteObjectExists(t, db, "table", "audit_events")
	requireSQLiteObjectExists(t, db, "table", "idempotency_records")

	requireSQLiteObjectExists(t, db, "index", "idx_api_keys_access_key")
	requireSQLiteObjectExists(t, db, "index", "idx_downstream_requests_request_id")
	requireSQLiteObjectExists(t, db, "index", "idx_upstream_attempts_request_id")
	requireSQLiteObjectExists(t, db, "index", "idx_audit_events_request_id_created_at")
	requireSQLiteObjectExists(t, db, "index", "idx_idempotency_records_idempotency_key")
}
