// SPDX-License-Identifier: Apache-2.0
package artifact

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func exportFixture(t *testing.T) (string, string) {
	t.Helper()
	if os.Getenv("VIEWER_PG_DSN") == "" {
		t.Skip("dedicated PostgreSQL required")
	}
	root := t.TempDir()
	if err := Open(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close(root) })
	path := filepath.Join(root, "uploads", "m_0000000000000001.ifc")
	if err := WriteFile(path, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	return root, path
}

func TestSnapshotExportBudgetIndependentOfLiveReader(t *testing.T) {
	root, path := exportFixture(t)
	first, err := OpenSnapshot(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	// A live download no longer retains export capacity.
	release, ok := snapshotExportBytes.Try(1 << 30)
	if !ok {
		t.Fatal("live reader retains export capacity")
	}
	defer release()
	second, err := OpenSnapshot(context.Background(), path)
	if second != nil {
		second.Close()
	}
	if !errors.Is(err, ErrReadCapacity) {
		t.Fatal("full export budget admitted a copy", err)
	}
	if got, err := io.ReadAll(first); err != nil || string(got) != "synthetic" {
		t.Fatal("live reader interrupted", err)
	}
	db, _ := Database(root)
	if db.Stats().InUse != 0 {
		t.Fatal("rejected copy retains connection", db.Stats())
	}
	release()
	second, err = OpenSnapshot(context.Background(), path)
	if err != nil {
		t.Fatal("export capacity was not reusable", err)
	}
	second.Close()
}

func TestSnapshotExportBudgetReleasedOnFailure(t *testing.T) {
	_, path := exportFixture(t)
	invalid := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(invalid, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", invalid)
	if snapshot, err := OpenSnapshot(context.Background(), path); err == nil {
		snapshot.Close()
		t.Fatal("invalid temporary directory accepted")
	}
	release, ok := snapshotExportBytes.Try(1 << 30)
	if !ok {
		t.Fatal("failed copy retained export bytes")
	}
	release()
}
