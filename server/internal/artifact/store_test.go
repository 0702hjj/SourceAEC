// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package artifact

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportWriteRestoreDelete(t *testing.T) {
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
	roots.Delete(root)
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
	roots.Delete(root)
	if err := Open(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("deleted artifact restored: %v", err)
	}
}
