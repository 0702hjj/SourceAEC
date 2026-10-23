// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package editruntime

import (
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"ifcviewer/server/internal/artifact"
)

type fileEntry struct {
	Path    string `json:"path"`
	Content []byte `json:"content"`
}

func (r *Runtime) track(req Request, key string) error {
	t := r.transactions[req.Tx]
	if t == nil {
		return nil
	}
	if _, ok := t.originals[key]; ok {
		return nil
	}
	path, err := safePath(req.Root, key)
	if err != nil {
		return err
	}

	if artifact.Durable(key) {
		var data []byte
		var mode uint32
		err := t.tx.QueryRow("SELECT content,mode FROM artifacts WHERE path=$1", key).Scan(&data, &mode)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		t.originals[key] = original{Content: data, Exists: err == nil, Mode: os.FileMode(mode)}
	} else {
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		mode := os.FileMode(0644)
		if info, e := os.Stat(path); e == nil {
			mode = info.Mode().Perm()
		}
		t.originals[key] = original{Content: data, Exists: err == nil, Mode: mode}
	}
	return nil
}
func restore(t *transaction) error {
	var failures []error
	for key, old := range t.originals {
		path, err := safePath(t.root, key)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if old.Exists {
			err = os.MkdirAll(filepath.Dir(path), 0755)
			if err == nil {
				err = os.WriteFile(path, old.Content, old.Mode)
			}
		} else {
			err = os.Remove(path)
			if os.IsNotExist(err) {
				err = nil
			}
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
func (r *Runtime) restoreWorkspace(root string) error {
	db, err := artifact.Database(root)
	if err != nil {
		return err
	}
	rows, err := db.Query("SELECT path,content,mode FROM artifacts")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var data []byte
		var mode uint32
		if err = rows.Scan(&key, &data, &mode); err != nil {
			return err
		}
		path, e := safePath(root, key)
		if e != nil {
			return e
		}
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err = os.WriteFile(path, data, fs.FileMode(mode)); err != nil {
			return err
		}
	}
	return rows.Err()
}
func (r *Runtime) artifactOperation(req Request) (any, error) {
	db, err := r.database(req)
	if err != nil {
		return nil, err
	}
	switch req.Op {
	case "publish":
		var files []fileEntry
		if err = decode(req.Args, &files); err != nil {
			return nil, err
		}
		// A batch is atomic even when the caller did not open a checkpoint.
		if req.Tx == "" {
			pool, e := artifact.Database(req.Root)
			if e != nil {
				return nil, e
			}
			tx, e := pool.Begin()
			if e != nil {
				return nil, e
			}
			defer tx.Rollback()
			db = tx
			if e = r.publish(req, db, files); e != nil {
				return nil, e
			}
			return nil, tx.Commit()
		}
		return nil, r.publish(req, db, files)
	case "read":
		var key string
		if err = decode(req.Args, &key); err != nil {
			return nil, err
		}
		path, e := safePath(req.Root, key)
		if e != nil {
			return nil, e
		}
		var content []byte
		err = db.QueryRow("SELECT content FROM artifacts WHERE path=$1", key).Scan(&content)
		if errors.Is(err, sql.ErrNoRows) {
			content, err = os.ReadFile(path)
		}
		return content, err
	case "remove":
		var key string
		if err = decode(req.Args, &key); err != nil {
			return nil, err
		}
		if err = r.track(req, key); err != nil {
			return nil, err
		}
		_, err = db.Exec("DELETE FROM artifacts WHERE path=$1", key)
		return nil, err
	case "capture_tree":
		var key string
		if err = decode(req.Args, &key); err != nil {
			return nil, err
		}
		base, e := safePath(req.Root, key)
		if e != nil {
			return nil, e
		}
		files := []fileEntry{}
		err = filepath.WalkDir(base, func(path string, d fs.DirEntry, e error) error {
			if os.IsNotExist(e) {
				return nil
			}
			if e != nil {
				return e
			}
			rel, e := filepath.Rel(req.Root, path)
			if e != nil {
				return e
			}
			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if d.IsDir() {
				if !artifact.Durable(rel + "/file") {
					return filepath.SkipDir
				}
				return nil
			}
			if !artifact.Durable(rel) {
				return nil
			}
			data, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			files = append(files, fileEntry{filepath.ToSlash(rel), data})
			return nil
		})
		if err != nil {
			return nil, err
		}
		return nil, r.publish(req, db, files)
	default:
		return nil, errors.New("unknown runtime operation")
	}
}
func (r *Runtime) publish(req Request, db queryer, files []fileEntry) error {
	for _, file := range files {
		if !artifact.Durable(file.Path) {
			continue
		}
		if _, err := safePath(req.Root, file.Path); err != nil {
			return err
		}
		if err := r.track(req, file.Path); err != nil {
			return err
		}
		if _, err := db.Exec("INSERT INTO artifacts(path,content) VALUES($1,$2) ON CONFLICT(path) DO UPDATE SET content=excluded.content", file.Path, file.Content); err != nil {
			return err
		}
	}
	return nil
}
