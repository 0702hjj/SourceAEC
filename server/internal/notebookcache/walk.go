// SPDX-License-Identifier: Apache-2.0
package notebookcache

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(data)
}

// Walk in small batches rather than allocating a complete directory listing
// before discovering that a sandbox produced too many cache entries.
func walkCache(root string, visit func(string, fs.DirEntry, error) error) error {
	count := 0
	var walk func(string, fs.DirEntry) error
	walk = func(name string, entry fs.DirEntry) error {
		count++
		if count > MaxFiles*2 || len(name)-len(root) > 512 {
			return errors.New("notebook cache traversal budget exceeded")
		}
		if err := visit(name, entry, nil); err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		fd, err := syscall.Open(name, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		dir := os.NewFile(uintptr(fd), name)
		defer dir.Close()
		for {
			entries, err := dir.ReadDir(64)
			if err != nil && err != io.EOF {
				return err
			}
			for _, child := range entries {
				if e := walk(filepath.Join(name, child.Name()), child); e != nil {
					return e
				}
			}
			if err == io.EOF {
				return nil
			}
		}
	}
	info, err := os.Lstat(root)
	if err != nil {
		return visit(root, nil, err)
	}
	return walk(root, fs.FileInfoToDirEntry(info))
}
