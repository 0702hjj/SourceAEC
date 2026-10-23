// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package migration

import (
	"database/sql"
	"ifcviewer/server/internal/artifact"
	"os"
	"path/filepath"
	"testing"
)

func TestImportPreservesBytesAndRefusesOverwrite(t *testing.T) {
	if os.Getenv("VIEWER_PG_DSN") == "" {
		t.Skip("VIEWER_PG_DSN not set")
	}
	source := filepath.Join(t.TempDir(), "legacy.sqlite3")
	legacy, err := sql.Open("sqlite3", source)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`CREATE TABLE artifacts(path TEXT PRIMARY KEY,content BLOB,mode INTEGER);CREATE TABLE repositories(model_id TEXT PRIMARY KEY,bundle BLOB,branch TEXT);INSERT INTO artifacts VALUES('models/example/scripts/v1.py',X'000102FF',420);INSERT INTO repositories VALUES('example',X'030405FF','feature');`)
	if err != nil {
		t.Fatal(err)
	}
	legacy.Close()
	before, _ := os.ReadFile(source)
	db, err := artifact.Connect(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	counts, err := Import(source, db)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Artifacts != 1 || counts.Repositories != 1 {
		t.Fatal(counts)
	}
	var content []byte
	var branch string
	db.QueryRow("SELECT bundle,branch FROM repositories WHERE model_id='example'").Scan(&content, &branch)
	if string(content) != string([]byte{3, 4, 5, 255}) || branch != "feature" {
		t.Fatal(content, branch)
	}
	if _, err = Import(source, db); err == nil {
		t.Fatal("allowed overwrite")
	}
	after, _ := os.ReadFile(source)
	if string(before) != string(after) {
		t.Fatal("source modified")
	}
}
func TestInvalidSourceRollsBackAllRows(t *testing.T) {
	if os.Getenv("VIEWER_PG_DSN") == "" {
		t.Skip("VIEWER_PG_DSN not set")
	}
	source := filepath.Join(t.TempDir(), "legacy.sqlite3")
	legacy, _ := sql.Open("sqlite3", source)
	_, err := legacy.Exec(`CREATE TABLE artifacts(path TEXT,content BLOB,mode INTEGER);INSERT INTO artifacts VALUES('valid.py',X'01',420),('../escape',X'02',420);`)
	if err != nil {
		t.Fatal(err)
	}
	legacy.Close()
	db, err := artifact.Connect(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = Import(source, db); err == nil {
		t.Fatal("accepted traversal")
	}
	var count int
	if err = db.QueryRow("SELECT count(*) FROM artifacts").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}
