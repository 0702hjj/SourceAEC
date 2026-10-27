// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"ifcviewer/server/internal/artifact"
	"os"
	"path/filepath"
	"testing"
)

func TestDisconnectedTransactionRestoresAndCanRetry(t *testing.T) {
	r, root := databaseRuntime(t)
	ctx := context.Background()
	key := "models/m_0000000000000001/scripts/v1.py"
	path := filepath.Join(root, key)
	if err := artifact.WriteFile(path, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	tx := invoke(t, r, "begin", root, "", nil).(string)
	invoke(t, r, "lock", root, tx, "m_0000000000000001")
	invoke(t, r, "publish", root, tx, []fileEntry{{key, []byte("uncommitted")}})
	if err := os.WriteFile(path, []byte("uncommitted"), 0600); err != nil {
		t.Fatal(err)
	}
	var pid int
	if err := r.transactions[tx].tx.QueryRow("SELECT pg_backend_pid()").Scan(&pid); err != nil {
		t.Fatal(err)
	}
	db, err := artifact.Database(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("SELECT pg_terminate_backend($1)", pid); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Handle(Request{Root: root, Tx: tx, Op: "commit"}); err == nil {
		t.Fatal("disconnected transaction committed")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "before" {
		t.Fatal("projection not recovered", string(data), err)
	}
	data, err = r.ReadStoredArtifact(ctx, root, "", key)
	if err != nil || string(data) != "before" {
		t.Fatal("uncommitted database state survived", string(data), err)
	}
	retry := invoke(t, r, "begin", root, "", nil).(string)
	invoke(t, r, "lock", root, retry, "m_0000000000000001")
	invoke(t, r, "publish", root, retry, []fileEntry{{key, []byte("retried")}})
	invoke(t, r, "commit", root, retry, nil)
	data, err = r.ReadStoredArtifact(ctx, root, "", key)
	if err != nil || string(data) != "retried" {
		t.Fatal("retry failed", string(data), err)
	}
}

func TestFailedProjectionRecoveryKeepsJournalForRetry(t *testing.T) {
	r, root := databaseRuntime(t)
	key := "models/m_0000000000000001/scripts/v1.py"
	path := filepath.Join(root, key)
	if err := artifact.WriteFile(path, []byte("committed"), 0600); err != nil {
		t.Fatal(err)
	}
	tx := invoke(t, r, "begin", root, "", nil).(string)
	invoke(t, r, "lock", root, tx, "m_0000000000000001")
	invoke(t, r, "publish", root, tx, []fileEntry{{key, []byte("uncommitted")}})
	outside := filepath.Join(t.TempDir(), "untouched")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Handle(Request{Root: root, Tx: tx, Op: "rollback"}); err == nil {
		t.Fatal("unsafe projection recovery unexpectedly succeeded")
	}
	journal := filepath.Join(root, ".edit-journal", tx+".jsonl")
	if _, err := os.Stat(journal); err != nil {
		t.Fatal("failed recovery discarded journal", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := artifact.Restore(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "committed" {
		t.Fatal("recovery retry failed", string(data), err)
	}
	if _, err = os.Stat(journal); !os.IsNotExist(err) {
		t.Fatal("successful recovery retained journal", err)
	}
	if _, err = os.Stat(outside); !os.IsNotExist(err) {
		t.Fatal("recovery wrote outside workspace", err)
	}
}
