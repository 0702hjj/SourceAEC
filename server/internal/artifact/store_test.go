// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package artifact

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportWriteRestoreDelete(t *testing.T) {
	if os.Getenv("VIEWER_PG_DSN") == "" {
		t.Skip("VIEWER_PG_DSN not set")
	}
	root := t.TempDir()
	path := filepath.Join(root, "models", "m_0000000000000001", "model.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("legacy"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Open(root); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	data, err := ReadFile(path)
	if err != nil || string(data) != "new" {
		t.Fatalf("read %q %v", data, err)
	}
	if err := Close(root); err != nil {
		t.Fatal(err)
	}
	if err := Open(root); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil || string(data) != "new" {
		t.Fatalf("restore %q %v", data, err)
	}
	if err := RemoveAll(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if err := Close(root); err != nil {
		t.Fatal(err)
	}
	if err := Open(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("deleted artifact restored: %v", err)
	}
}

func TestMissingPostgresFailsClosed(t *testing.T) {
	t.Setenv("VIEWER_PG_DSN", "")
	if err := Open(t.TempDir()); err == nil {
		t.Fatal("opened storage without PostgreSQL")
	}
}

func TestLegacySQLiteRequiresExplicitMigration(t *testing.T) {
	if os.Getenv("VIEWER_PG_DSN") == "" {
		t.Skip("VIEWER_PG_DSN not set")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sourceaec.sqlite3"), []byte("legacy"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Open(root); err == nil {
		t.Fatal("silently ignored legacy SQLite history")
	}
}

func TestExplicitNamespaceRestoresAfterWorkspaceMove(t *testing.T) {
	if os.Getenv("VIEWER_PG_DSN") == "" {
		t.Skip("VIEWER_PG_DSN not set")
	}
	first, second := t.TempDir(), t.TempDir()
	t.Setenv("VIEWER_STORAGE_NAMESPACE", first)
	if err := Open(first); err != nil {
		t.Fatal(err)
	}
	key := "uploads/m_0000000000000001.ifc"
	if err := WriteFile(filepath.Join(first, key), []byte("synthetic IFC"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Close(first); err != nil {
		t.Fatal(err)
	}
	if err := Open(second); err != nil {
		t.Fatal(err)
	}
	defer Close(second)
	content, err := os.ReadFile(filepath.Join(second, key))
	if err != nil || string(content) != "synthetic IFC" {
		t.Fatal(string(content), err)
	}
}
