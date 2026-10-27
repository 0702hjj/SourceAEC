// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package editruntime

import (
	"context"
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
	if _, ok := t.tracked[key]; ok {
		return nil
	}
	if !t.journalLocked {
		if err := artifact.LockProjectionRecovery(runtimeContext(req), t.tx, req.Root, true); err != nil {
			return err
		}
		t.journalLocked = true
	}
	if err := artifact.RecordProjection(req.Root, req.Tx, key); err != nil {
		return err
	}
	t.tracked[key] = struct{}{}
	return nil
}
func restore(t *transaction) error {
	if !t.journalLocked {
		return nil
	}
	return artifact.RecoverProjectionTransaction(t.root, t.token, t.model)
}
func (r *Runtime) restoreWorkspace(root string) error {
	return artifact.Restore(context.Background(), root)
}
func (r *Runtime) artifactOperation(req Request) (any, error) {
	db, err := r.database(req)
	if err != nil {
		return nil, err
	}
	switch req.Op {
	case "list":
		var prefix string
		if err = decode(req.Args, &prefix); err != nil {
			return nil, err
		}
		if _, err = safePath(req.Root, prefix); err != nil {
			return nil, err
		}
		rows, err := db.Query("SELECT path FROM artifacts WHERE starts_with(path,$1) ORDER BY path", prefix)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		paths := []string{}
		for rows.Next() {
			var path string
			if err = rows.Scan(&path); err != nil {
				return nil, err
			}
			paths = append(paths, path)
		}
		return paths, rows.Err()
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
			if t := r.transactions[req.Tx]; t != nil && t.readOnly {
				return nil, os.ErrNotExist
			}
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
