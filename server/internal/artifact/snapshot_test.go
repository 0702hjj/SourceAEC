// SPDX-License-Identifier: Apache-2.0
package artifact

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotDoesNotHoldConnectionOrMutableProjection(t *testing.T) {
	if os.Getenv("VIEWER_PG_DSN") == "" {
		t.Skip("VIEWER_PG_DSN not set")
	}
	root := t.TempDir()
	if err := Open(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close(root) })
	path := filepath.Join(root, "uploads", "m_0000000000000001.ifc")
	data := bytes.Repeat([]byte("source"), 200000)
	if err := WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := OpenSnapshot(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	db, _ := Database(root)
	if db.Stats().InUse != 0 {
		t.Fatal("snapshot retains a database connection")
	}
	if err := Remove(path); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(s)
	if err != nil || !bytes.Equal(data, got) {
		t.Fatalf("deleted source broke snapshot: %v", err)
	}
	name := s.Name()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Fatalf("temporary snapshot leaked: %v", err)
	}
}

type mutatingWriter struct {
	bytes.Buffer
	change func()
}

func (w *mutatingWriter) Write(p []byte) (int, error) {
	if w.change != nil {
		change := w.change
		w.change = nil
		change()
	}
	return w.Buffer.Write(p)
}

func TestSnapshotChunksUseOneCommittedRevision(t *testing.T) {
	if os.Getenv("VIEWER_PG_DSN") == "" {
		t.Skip("VIEWER_PG_DSN not set")
	}
	root := t.TempDir()
	if err := Open(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close(root) })
	db, _ := Database(root)
	data := bytes.Repeat([]byte("a"), 2<<20+3)
	key := "uploads/m_0000000000000001.ifc"
	if _, err := db.Exec("INSERT INTO artifacts(path,content) VALUES($1,$2)", key, data); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	tx, err := conn.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	w := &mutatingWriter{change: func() {
		if _, err := db.Exec("UPDATE artifacts SET content=$1 WHERE path=$2", bytes.Repeat([]byte("b"), len(data)), key); err != nil {
			t.Error(err)
		}
	}}
	if err := copySnapshot(context.Background(), conn, key, w, int64(len(data))); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(w.Bytes(), data) {
		t.Fatal("mixed committed revisions")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := OpenSnapshot(ctx, filepath.Join(root, key)); err == nil {
		t.Fatal("ignored cancellation")
	}
}
