// SPDX-License-Identifier: Apache-2.0
package notebookcache

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
)

func adversarialArchive(t *testing.T, files map[string][]byte, manifest Manifest, method uint16) Snapshot {
	t.Helper()
	var raw bytes.Buffer
	writer := zip.NewWriter(&raw)
	for name, data := range files {
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: method})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	header, err := writer.Create("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = header.Write(encoded); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	payload := raw.Bytes()
	return Snapshot{Identity: manifest.Identity, Generation: Hash(payload), Payload: payload}
}
func TestArchiveRejectsCompressedExpansionAndForgedFileDigest(t *testing.T) {
	name := roots[0] + "/simplecad/values.pkl"
	for _, oversized := range []bool{false, true} {
		contents := []byte("opaque data")
		size := int64(len(contents))
		hash := "wrong-inner-digest"
		if oversized {
			size = MaxBytes + 1
			hash = Hash(contents)
		}
		manifest := Manifest{Policy: Policy, Identity: Hash(nil), Files: []File{{Path: name, Bytes: size, SHA256: hash}}}
		value := adversarialArchive(t, map[string][]byte{name: contents}, manifest, zip.Deflate)
		root := t.TempDir()
		if err := Restore(context.Background(), root, roots, value); err == nil {
			t.Fatal("malformed compressed package restored", oversized)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatal("invalid package wrote files", entries, err)
		}
	}
}
func TestArchiveRejectsActualHighExpansionBeforeDecompression(t *testing.T) {
	var raw bytes.Buffer
	writer := zip.NewWriter(&raw)
	name := roots[0] + "/large.pkl"
	entry, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
	if err != nil {
		t.Fatal(err)
	}
	block := make([]byte, 1<<20)
	for range 65 {
		if _, err = entry.Write(block); err != nil {
			t.Fatal(err)
		}
	}
	manifest := Manifest{Policy: Policy, Identity: Hash(nil), Files: []File{{Path: name, Bytes: 65 << 20, SHA256: Hash(nil)}}}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	header, err := writer.Create("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = header.Write(encoded); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	payload := raw.Bytes()
	if len(payload) > 1<<20 {
		t.Fatal("fixture is not highly compressed", len(payload))
	}
	value := Snapshot{Identity: manifest.Identity, Generation: Hash(payload), Payload: payload}
	if err = Restore(context.Background(), t.TempDir(), roots, value); err == nil {
		t.Fatal("compressed expansion exceeded cache budget")
	}
}
