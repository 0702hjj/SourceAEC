// SPDX-License-Identifier: Apache-2.0
package artifact

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func journalStore(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := Open(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close(root) })
	return root
}

func TestRecoveryHonorsLiveProjectionLock(t *testing.T) {
	root := journalStore(t)
	db, err := Database(root)
	if err != nil {
		t.Fatal(err)
	}
	key := "uploads/m_0123456789abcdef.ifc"
	if err = WriteFile(filepath.Join(root, key), []byte("committed"), 0644); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = LockProjectionRecovery(context.Background(), tx, root, true); err != nil {
		t.Fatal(err)
	}
	if err = RecordProjection(root, strings.Repeat("a", 32), key); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, key), []byte("live"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err = Restore(ctx, root); err == nil || ctx.Err() == nil {
		t.Fatalf("recovery bypassed live writer: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(root, key))
	if string(data) != "live" {
		t.Fatal("recovery rewrote a live transaction")
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = Restore(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(root, key))
	if string(data) != "committed" {
		t.Fatal("recovery did not restore authoritative content")
	}
}

func TestRecoveryDropsUncommittedPathsAndTornTail(t *testing.T) {
	root := journalStore(t)
	token := strings.Repeat("b", 32)
	key := "models/m_0123456789abcdef/scripts/v2.py"
	if err := RecordProjection(root, token, key); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, key)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("uncommitted"), 0644); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(root, ".edit-journal", token+".jsonl"), os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString(`"incomplete`)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	if err = Restore(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("uncommitted projection survived: %v", err)
	}
}

func TestProjectionJournalRejectsEscapesAndSymlinks(t *testing.T) {
	root := journalStore(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"../escape", filepath.Join(outside, "file"), "linked/file"} {
		if err := RecordProjection(root, strings.Repeat("c", 32), key); err == nil {
			t.Fatalf("unsafe journal path accepted: %s", key)
		}
	}
}
