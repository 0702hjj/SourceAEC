// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"ifcviewer/server/internal/artifact"
)

func (s *Store) viewerLock(ctx context.Context, id string) (*sql.Tx, func(), error) {
	if !validID(id) {
		return nil, nil, ErrInvalidID
	}
	release, err := artifact.AcquireModel(ctx, s.DataDir, id)
	if err != nil {
		return nil, nil, err
	}
	db, err := artifact.Database(s.DataDir)
	if err != nil {
		release()
		return nil, nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		release()
		return nil, nil, err
	}
	close := func() { _ = tx.Rollback(); release() }
	if err = artifact.LockModel(ctx, tx, s.DataDir, id); err != nil {
		close()
		return nil, nil, err
	}
	return tx, close, nil
}

// OpenViewer resolves and opens under the same model lock as publication,
// deletion and GC. The descriptor remains readable after a later unlink.
func (s *Store) OpenViewer(ctx context.Context, id, name, generation string) (*os.File, error) {
	if name != "model.xkt" && name != "metadata.json" {
		return nil, ErrInvalidID
	}
	tx, close, err := s.viewerLock(ctx, id)
	if err != nil {
		return nil, err
	}
	defer close()
	path, err := s.viewerPath(ctx, tx, id, name, generation)
	if err != nil {
		return nil, err
	}
	if err = cacheDirectories(s.DataDir, filepath.Dir(path)); err != nil {
		return nil, err
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, os.ErrNotExist
	}
	return file, nil
}

// PruneViewerCache removes only unreferenced immutable generations older than
// cutoff. Active private converter directories are outside viewer_cache.
func (s *Store) PruneViewerCache(ctx context.Context, id string, cutoff time.Time) error {
	tx, close, err := s.viewerLock(ctx, id)
	if err != nil {
		return err
	}
	defer close()
	var raw []byte
	err = tx.QueryRowContext(ctx, "SELECT content FROM artifacts WHERE path=$1", "models/"+id+"/viewer.json").Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var current viewerManifest
	if len(raw) > 0 {
		if err = json.Unmarshal(raw, &current); err != nil {
			return err
		}
	}
	parent := filepath.Join(s.ModelDir(id), "viewer_cache")
	if err = cacheDirectories(s.DataDir, parent); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	entries, err := os.ReadDir(parent)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			return err
		}
		if !generationPattern.MatchString(entry.Name()) || entry.Name() == current.Generation {
			continue
		}
		info, e := entry.Info()
		if e != nil {
			return e
		}
		if !info.ModTime().Before(cutoff) {
			continue
		}
		if err = os.RemoveAll(filepath.Join(parent, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func cacheDirectories(root, path string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return err
	}
	for dir := path; dir != root; dir = filepath.Dir(dir) {
		info, e := os.Lstat(dir)
		if e != nil {
			return e
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return os.ErrNotExist
		}
		if filepath.Dir(dir) == dir {
			return os.ErrNotExist
		}
	}
	return nil
}
