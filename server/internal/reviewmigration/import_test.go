// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package reviewmigration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/change"
	"ifcviewer/server/internal/issue"
	"ifcviewer/server/internal/override"
)

func TestImportPreservesRecordsAndDoesNotResurrectDeletedRows(t *testing.T) {
	dsn := os.Getenv("VIEWER_PG_DSN")
	if dsn == "" {
		t.Skip("VIEWER_PG_DSN not set")
	}
	root := t.TempDir()
	db, err := artifact.Connect(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	schema, _ := artifact.Namespace(root)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	issues, err := issue.NewPgStore(pool, root)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := change.NewPgStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	overrides, err := override.NewPgStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	model := "m_0000000000000001"
	directory := filepath.Join(root, "models", model)
	os.MkdirAll(directory, 0755)
	write := func(name string, value any) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(directory, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	write("issues.json", []issue.Issue{{ID: "i_000000000001", Title: "Synthetic issue", Status: "open", CreatedAt: created, UpdatedAt: created}})
	write("changes.json", []change.Entry{{ID: "c_000000000001", Field: "Name", NewValue: "Synthetic wall", CreatedAt: created, Provenance: change.Provenance{Source: "UI"}}})
	write("overrides.json", map[string]map[string]string{"entity": {"Name": "Synthetic wall"}})
	if err = Import(pool, root); err != nil {
		t.Fatal(err)
	}
	gotIssues, err := issues.List(model)
	if err != nil || len(gotIssues) != 1 || gotIssues[0].ID != "i_000000000001" {
		t.Fatal(gotIssues, err)
	}
	gotChanges, err := changes.List(model)
	if err != nil || len(gotChanges) != 1 || gotChanges[0].ID != "c_000000000001" || !gotChanges[0].CreatedAt.Equal(created) {
		t.Fatal(gotChanges, err)
	}
	gotOverrides, err := overrides.GetAll(model)
	if err != nil || gotOverrides["entity"]["Name"] != "Synthetic wall" {
		t.Fatal(gotOverrides, err)
	}
	if err = issues.Delete(model, "i_000000000001"); err != nil {
		t.Fatal(err)
	}
	if err = Import(pool, root); err != nil {
		t.Fatal(err)
	}
	gotIssues, err = issues.List(model)
	if err != nil || len(gotIssues) != 0 {
		t.Fatal("deleted row resurrected", gotIssues, err)
	}
}
