// SPDX-License-Identifier: Apache-2.0
package notebookcache

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

var roots = []string{"skill-work/part/__marimo__"}

func fixture(t *testing.T) (string, Snapshot) {
	t.Helper()
	root := t.TempDir()
	filename := filepath.Join(root, roots[0], "simplecad", "part.py", "part", "cache", "values.pkl")
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte("opaque SDK bytes; never decoded by Go"), 0600); err != nil {
		t.Fatal(err)
	}
	value, err := Pack(context.Background(), root, roots, Hash([]byte("environment and inputs")))
	if err != nil {
		t.Fatal(err)
	}
	return root, value
}
func TestArchiveRoundTripAndIndependentAttempts(t *testing.T) {
	_, value := fixture(t)
	a, b := t.TempDir(), t.TempDir()
	for _, root := range []string{a, b} {
		if err := Restore(context.Background(), root, roots, value); err != nil {
			t.Fatal(err)
		}
	}
	name := filepath.Join(roots[0], "simplecad", "part.py", "part", "cache", "values.pkl")
	if err := os.WriteFile(filepath.Join(a, name), []byte("attempt A changes cache"), 0600); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(b, name))
	if err != nil || string(original) != "opaque SDK bytes; never decoded by Go" {
		t.Fatal("attempts share mutable state", err)
	}
	next, err := Pack(context.Background(), a, roots, value.Identity)
	if err != nil || next.Generation == value.Generation {
		t.Fatal("changed cache reused generation", err)
	}
}
func TestArchiveRejectsCorruptionAndWrongCompatibility(t *testing.T) {
	_, good := fixture(t)
	for name, mutate := range map[string]func(*Snapshot){
		"payload":  func(v *Snapshot) { v.Payload = append([]byte(nil), v.Payload...); v.Payload[0] ^= 1 },
		"identity": func(v *Snapshot) { v.Identity = Hash([]byte("other environment")) },
	} {
		t.Run(name, func(t *testing.T) {
			value := good
			mutate(&value)
			root := t.TempDir()
			if err := Restore(context.Background(), root, roots, value); err == nil {
				t.Fatal("invalid archive accepted")
			}
			entries, _ := os.ReadDir(root)
			if len(entries) != 0 {
				t.Fatal("corruption wrote partial cache")
			}
		})
	}
	if err := Restore(context.Background(), t.TempDir(), []string{"skill-work/other/__marimo__"}, good); err == nil {
		t.Fatal("cross-notebook path accepted")
	}
}
func TestArchiveRejectsUploadedPathsAndSymlinks(t *testing.T) {
	root, _ := fixture(t)
	if err := os.Symlink("/etc/passwd", filepath.Join(root, roots[0], "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := Pack(context.Background(), root, roots, Hash(nil)); err == nil {
		t.Fatal("symlink accepted")
	}
	for _, name := range []string{"../escape", "/absolute", "skill-work/part/__marimo__/../../escape", "skill-work/part/__marimo__/a\\b"} {
		var raw bytes.Buffer
		writer := zip.NewWriter(&raw)
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = entry.Write([]byte("bad"))
		_ = writer.Close()
		value := Snapshot{Identity: Hash(nil), Payload: raw.Bytes()}
		value.Generation = Hash(value.Payload)
		if err := Restore(context.Background(), t.TempDir(), roots, value); err == nil {
			t.Fatal("unsafe path accepted", name)
		}
	}
}
func TestArchiveBoundsAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root, value := fixture(t)
	if _, err := Pack(ctx, root, roots, value.Identity); err == nil {
		t.Fatal("packing ignored cancellation")
	}
	if err := Restore(ctx, t.TempDir(), roots, value); err == nil {
		t.Fatal("restore ignored cancellation")
	}
	oversized := filepath.Join(root, roots[0], "oversized")
	file, err := os.Create(oversized)
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Truncate(MaxBytes + 1); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	if _, err := Pack(context.Background(), root, roots, value.Identity); err == nil {
		t.Fatal("oversized cache read before rejection")
	}
}

func TestArchiveLimitsTraversalBeforeUnboundedDirectoryRead(t *testing.T) {
	root := t.TempDir()
	prefix := roots[0]
	if err := os.MkdirAll(filepath.Join(root, prefix), 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= MaxFiles; i++ {
		if err := os.WriteFile(filepath.Join(root, prefix, fmt.Sprint(i)), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Pack(context.Background(), root, roots, Hash(nil)); err == nil {
		t.Fatal("file count budget ignored")
	}
}
