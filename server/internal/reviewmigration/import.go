// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
// Package reviewmigration moves legacy JSON review records into the existing
// native PostgreSQL tables without changing IDs, timestamps or provenance.
package reviewmigration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/change"
	"ifcviewer/server/internal/issue"
)

func read(path string, value any) error {
	data, err := artifact.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
func Import(pool *pgxpool.Pool, root string) error {
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(710363)"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS sourceaec_review_imports(namespace TEXT PRIMARY KEY)"); err != nil {
		return err
	}
	namespace, err := artifact.Namespace(root)
	if err != nil {
		return err
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM sourceaec_review_imports WHERE namespace=$1", namespace).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	models, err := os.ReadDir(filepath.Join(root, "models"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, model := range models {
		if !model.IsDir() {
			continue
		}
		directory := filepath.Join(root, "models", model.Name())
		var issues []issue.Issue
		if err = read(filepath.Join(directory, "issues.json"), &issues); err != nil {
			return err
		}
		for _, entry := range issues {
			data, err := json.Marshal(entry)
			if err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, "INSERT INTO issues(id,model_id,data,created_at) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING", entry.ID, model.Name(), data, entry.CreatedAt); err != nil {
				return err
			}
		}
		var changes []change.Entry
		if err = read(filepath.Join(directory, "changes.json"), &changes); err != nil {
			return err
		}
		for _, entry := range changes {
			if err = importChange(ctx, tx, model.Name(), entry); err != nil {
				return err
			}
		}
		var overrides map[string]map[string]string
		if err = read(filepath.Join(directory, "overrides.json"), &overrides); err != nil {
			return err
		}
		for entity, fields := range overrides {
			for field, value := range fields {
				if _, err = tx.Exec(ctx, "INSERT INTO overrides(model_id,entity_id,field,value) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING", model.Name(), entity, field, value); err != nil {
					return err
				}
			}
		}
	}
	if _, err = tx.Exec(ctx, "INSERT INTO sourceaec_review_imports(namespace) VALUES($1)", namespace); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func importChange(ctx context.Context, tx pgx.Tx, model string, entry change.Entry) error {
	provenance, err := json.Marshal(entry.Provenance)
	if err != nil {
		return err
	}
	if entry.Operation == "" {
		entry.Operation = "update"
	}
	var diff any
	if entry.Diff != nil {
		diff = []byte(entry.Diff)
	}
	_, err = tx.Exec(ctx, `INSERT INTO changes(id,model_id,entity_id,entity_name,field,old_value,new_value,author,provenance,operation,diff,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT DO NOTHING`, entry.ID, model, entry.EntityID, entry.EntityName, entry.Field, entry.OldValue, entry.NewValue, entry.Author, provenance, entry.Operation, diff, entry.CreatedAt)
	return err
}
