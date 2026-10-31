// SPDX-License-Identifier: Apache-2.0
package artifact

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotCapacityAndEmptyArtifact(t *testing.T) {
	if os.Getenv("VIEWER_PG_DSN") == "" {
		t.Skip("VIEWER_PG_DSN not set")
	}
	root := t.TempDir()
	if err := Open(root); err != nil {
		t.Fatal(err)
	}
	defer Close(root)
	path := filepath.Join(root, "uploads/m_0000000000000001.ifc")
	if err := WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	release, ok := snapshotBytes.Try(4 << 30)
	if !ok {
		t.Fatal("disk budget already used")
	}
	_, err := OpenSnapshot(context.Background(), path)
	release()
	if !errors.Is(err, ErrReadCapacity) {
		t.Fatalf("expected byte admission: %v", err)
	}
	var snapshots []*Snapshot
	for range cap(snapshotSlots) {
		s, err := OpenSnapshot(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		snapshots = append(snapshots, s)
		defer s.Close()
	}
	if _, err := OpenSnapshot(context.Background(), path); !errors.Is(err, ErrReadCapacity) {
		t.Fatalf("count admission: %v", err)
	}
	for _, s := range snapshots {
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := WriteFile(path, []byte{}, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := OpenSnapshot(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	info, err := s.Stat()
	if err != nil || info.Size() != 0 {
		t.Fatal(info, err)
	}
}
