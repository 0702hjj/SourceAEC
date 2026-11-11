// SPDX-License-Identifier: Apache-2.0
package notebookcache

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
)

type File struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}
type Manifest struct {
	Policy   string `json:"policy"`
	Identity string `json:"identity"`
	Files    []File `json:"files"`
}

func validName(name string) bool {
	return name != "" && len(name) <= 512 && !strings.ContainsAny(name, "\\\x00") && !strings.HasPrefix(name, "/") && path.Clean(name) == name && name != ".." && !strings.HasPrefix(name, "../")
}
func allowedName(name string, roots []string) bool {
	if !validName(name) {
		return false
	}
	for _, root := range roots {
		if validName(root) && strings.HasPrefix(root, "skill-work/") && strings.HasSuffix(root, "/__marimo__") && strings.HasPrefix(name, root+"/") {
			return true
		}
	}
	return false
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > MaxBytes {
		return 0, errors.New("notebook cache archive exceeds byte limit")
	}
	return b.Buffer.Write(p)
}

// Pack reads only completed private SDK state, never arbitrary project inputs.
func Pack(ctx context.Context, root string, allowed []string, identity string) (Snapshot, error) {
	if !hashPattern.MatchString(identity) {
		return Snapshot{}, errors.New("invalid notebook compatibility identity")
	}
	var buffer limitedBuffer
	writer := zip.NewWriter(&buffer)
	manifest := Manifest{Policy: Policy, Identity: identity, Files: []File{}}
	seen := map[string]bool{}
	var total int64
	scratch := make([]byte, 64<<10)
	for _, prefix := range allowed {
		if !validName(prefix) || !strings.HasPrefix(prefix, "skill-work/") || !strings.HasSuffix(prefix, "/__marimo__") {
			return Snapshot{}, errors.New("invalid notebook cache root")
		}
		err := walkCache(filepath.Join(root, filepath.FromSlash(prefix)), func(filename string, entry fs.DirEntry, walkErr error) error {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return errors.New("notebook cache contains a symlink")
			}
			if entry.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(root, filename)
			if err != nil {
				return err
			}
			name := filepath.ToSlash(relative)
			if !allowedName(name, allowed) || seen[name] {
				return errors.New("invalid or duplicate notebook cache path")
			}
			if len(manifest.Files) >= MaxFiles {
				return errors.New("notebook cache file count exceeded")
			}
			fd, err := syscall.Open(filename, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
			if err != nil {
				return err
			}
			file := os.NewFile(uintptr(fd), filename)
			defer file.Close()
			info, err := file.Stat()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > MaxBytes-total {
				return errors.New("notebook cache file exceeds byte/type limits")
			}
			target, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
			if err != nil {
				return err
			}
			h := sha256.New()
			n, err := io.CopyBuffer(io.MultiWriter(target, h), contextReader{ctx, io.LimitReader(file, info.Size()+1)}, scratch)
			if err != nil {
				return err
			}
			if n != info.Size() {
				return errors.New("notebook cache changed while packing")
			}
			total += n
			seen[name] = true
			manifest.Files = append(manifest.Files, File{name, n, hex.EncodeToString(h.Sum(nil))})
			return nil
		})
		if err != nil {
			return Snapshot{}, err
		}
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return Snapshot{}, err
	}
	if len(raw) > 1<<20 {
		return Snapshot{}, errors.New("notebook cache manifest exceeds limit")
	}
	entry, err := writer.CreateHeader(&zip.FileHeader{Name: "manifest.json", Method: zip.Store})
	if err != nil {
		return Snapshot{}, err
	}
	if _, err = entry.Write(raw); err != nil {
		return Snapshot{}, err
	}
	if err = writer.Close(); err != nil {
		return Snapshot{}, err
	}
	payload := buffer.Bytes()
	return Snapshot{Identity: identity, Generation: Hash(payload), Payload: payload}, nil
}

func copyEntry(ctx context.Context, file *zip.File, dst io.Writer, limit int64) (int64, error) {
	input, err := file.Open()
	if err != nil {
		return 0, err
	}
	defer input.Close()
	scratch := make([]byte, 64<<10)
	reader := io.LimitReader(input, limit+1)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, err := reader.Read(scratch)
		if n > 0 {
			total += int64(n)
			if total > limit {
				return total, errors.New("notebook cache entry exceeds limit")
			}
			if _, e := dst.Write(scratch[:n]); e != nil {
				return total, e
			}
		}
		if err == io.EOF {
			return total, nil
		}
		if err != nil {
			return total, err
		}
	}
}

// Restore validates the entire package before creating any files. The caller
// supplies an empty private attempt directory; no pickle is opened by Go.
func Restore(ctx context.Context, root string, allowed []string, value Snapshot) error {
	if len(value.Payload) > MaxBytes || Hash(value.Payload) != value.Generation {
		return errors.New("notebook cache archive digest mismatch")
	}
	archive, err := zip.NewReader(bytes.NewReader(value.Payload), int64(len(value.Payload)))
	if err != nil {
		return err
	}
	if len(archive.File) > MaxFiles+1 {
		return errors.New("notebook cache archive file count exceeded")
	}
	entries := map[string]*zip.File{}
	for _, file := range archive.File {
		if entries[file.Name] != nil || !file.Mode().IsRegular() {
			return errors.New("duplicate or nonregular notebook cache entry")
		}
		if file.Name != "manifest.json" && !allowedName(file.Name, allowed) {
			return errors.New("notebook cache path escapes declared roots")
		}
		entries[file.Name] = file
	}
	header := entries["manifest.json"]
	if header == nil || header.UncompressedSize64 > 1<<20 {
		return errors.New("missing or oversized notebook cache manifest")
	}
	var encoded bytes.Buffer
	if _, err = copyEntry(ctx, header, &encoded, 1<<20); err != nil {
		return err
	}
	var manifest Manifest
	decoder := json.NewDecoder(&encoded)
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&manifest); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF || manifest.Policy != Policy || manifest.Identity != value.Identity || len(manifest.Files)+1 != len(entries) {
		return errors.New("invalid notebook cache manifest identity or entries")
	}
	seen := map[string]bool{}
	var total int64
	for _, entry := range manifest.Files {
		file := entries[entry.Path]
		if file == nil || !allowedName(entry.Path, allowed) || seen[entry.Path] || entry.Bytes < 0 || entry.Bytes > MaxBytes-total || file.UncompressedSize64 != uint64(entry.Bytes) {
			return errors.New("invalid notebook cache file manifest")
		}
		h := sha256.New()
		n, err := copyEntry(ctx, file, h, entry.Bytes)
		if err != nil {
			return err
		}
		if n != entry.Bytes || hex.EncodeToString(h.Sum(nil)) != entry.SHA256 {
			return errors.New("notebook cache entry digest mismatch")
		}
		seen[entry.Path] = true
		total += n
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("notebook restore requires a private directory")
	}
	existing, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	if len(existing) != 0 {
		return errors.New("notebook restore requires an empty directory")
	}
	for _, entry := range manifest.Files {
		filename := filepath.Join(root, filepath.FromSlash(entry.Path))
		if err = os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
			return err
		}
		// O_EXCL also rejects pre-existing symlinks/files in an incorrectly reused workspace.
		file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = copyEntry(ctx, entries[entry.Path], file, entry.Bytes)
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return fmt.Errorf("close restored notebook cache: %w", closeErr)
		}
	}
	return nil
}

// The generation also binds its compatibility identity; metadata repair cannot
// relabel a valid cache package as belonging to another environment/input set.
func validateSnapshotIdentity(ctx context.Context, value Snapshot) error {
	archive, err := zip.NewReader(bytes.NewReader(value.Payload), int64(len(value.Payload)))
	if err != nil {
		return err
	}
	if len(archive.File) > MaxFiles+1 {
		return errors.New("notebook archive file count exceeded")
	}
	var manifest *zip.File
	for _, file := range archive.File {
		if file.Name == "manifest.json" {
			if manifest != nil {
				return errors.New("duplicate notebook manifest")
			}
			manifest = file
		}
	}
	if manifest == nil || manifest.UncompressedSize64 > 1<<20 {
		return errors.New("invalid notebook manifest")
	}
	var buffer bytes.Buffer
	if _, err = copyEntry(ctx, manifest, &buffer, 1<<20); err != nil {
		return err
	}
	var header Manifest
	decoder := json.NewDecoder(&buffer)
	if err = decoder.Decode(&header); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF || header.Policy != Policy || header.Identity != value.Identity {
		return errors.New("notebook generation compatibility mismatch")
	}
	return nil
}
