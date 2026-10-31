// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package artifact

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"ifcviewer/server/internal/versionstore"
)

// Connect creates the schema but neither imports nor restores workspace files.
// An explicit namespace keeps the database identity stable when moving a deployment.
func Connect(root string) (*sql.DB, error) {
	dsn := os.Getenv("VIEWER_PG_DSN")
	if dsn == "" {
		return nil, errors.New("VIEWER_PG_DSN is required for business storage")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("invalid PostgreSQL configuration")
	}
	schema, err := Namespace(root)
	if err != nil {
		return nil, err
	}

	cfg.RuntimeParams["search_path"] = schema
	db := stdlib.OpenDB(*cfg)
	db.SetMaxOpenConns(8)
	// Retain the bounded pool between bursts instead of repeatedly reconnecting
	// once more than database/sql's default two idle connections are returned.
	db.SetMaxIdleConns(8)
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		db.Close()
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("SELECT pg_advisory_xact_lock(hashtext($1))", schema); err == nil {
		_, err = tx.Exec("CREATE SCHEMA IF NOT EXISTS " + pgx.Identifier{schema}.Sanitize())
	}
	if err == nil {
		_, err = tx.Exec(`CREATE TABLE IF NOT EXISTS artifacts (path TEXT PRIMARY KEY, content BYTEA NOT NULL, mode INTEGER NOT NULL DEFAULT 420);
        CREATE TABLE IF NOT EXISTS migrations (name TEXT PRIMARY KEY);
        CREATE TABLE IF NOT EXISTS repositories (model_id TEXT PRIMARY KEY, bundle BYTEA NOT NULL, branch TEXT NOT NULL);
        CREATE TABLE IF NOT EXISTS transaction_receipts(token TEXT PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp());
        CREATE INDEX IF NOT EXISTS transaction_receipts_created ON transaction_receipts(created_at);`)
	}
	if err == nil {
		_, err = tx.Exec("ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS content_hash BYTEA GENERATED ALWAYS AS (sha256(content)) STORED")
	}
	if err == nil {
		err = versionstore.Schema(context.Background(), tx)
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// Database returns the initialized Go-owned store for the edit runtime.
func Database(root string) (*sql.DB, error) {
	if err := Open(root); err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	value, ok := roots.Load(absolute)
	if !ok {
		return nil, errors.New("storage not initialized")
	}
	return value.(*sql.DB), nil
}

func Durable(path string) bool { return durable(path) }

// Close releases a workspace's pool, e.g. when an embedded runtime shuts down.
func Close(root string) error {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	value, ok := roots.LoadAndDelete(absolute)
	if !ok {
		return nil
	}
	return value.(*sql.DB).Close()
}

// Namespace identifies a deployment without storing a machine-specific path.
func Namespace(root string) (string, error) {
	namespace := os.Getenv("VIEWER_STORAGE_NAMESPACE")
	if namespace == "" {
		absolute, err := filepath.Abs(root)
		if err != nil {
			return "", err
		}
		namespace = absolute
	}
	return fmt.Sprintf("sourceaec_%x", sha256.Sum256([]byte(namespace)))[:42], nil
}
