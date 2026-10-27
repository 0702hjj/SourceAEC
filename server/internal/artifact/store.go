// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// Package artifact persists business files in PostgreSQL. Paths are disposable
// projections for Git, sandbox runners and converters, never database keys
// containing machine-specific absolute paths.
package artifact

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var roots sync.Map // absolute data directory -> *sql.DB
var openMu sync.Mutex

// Open imports legacy files once and restores missing workspace projections.
// The gateway and Go edit runtime share one PostgreSQL namespace.
func Open(root string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if _, ok := roots.Load(root); ok {
		return nil
	}
	openMu.Lock()
	defer openMu.Unlock()
	if _, ok := roots.Load(root); ok {
		return nil
	}
	db, err := Connect(root)
	if err != nil {
		return err
	}
	registered := false
	defer func() {
		if !registered {
			db.Close()
		}
	}()
	if err = os.MkdirAll(root, 0700); err != nil {
		db.Close()
		return err
	}
	// Serialize first import across gateway and edit-runtime processes.
	tx, err := db.Begin()
	if err != nil {
		db.Close()
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("SELECT pg_advisory_xact_lock(710362)"); err != nil {
		db.Close()
		return err
	}
	var imported int
	err = tx.QueryRow("SELECT count(*) FROM migrations WHERE name='files-v1'").Scan(&imported)
	if err != nil {
		return err
	}
	if imported == 0 {
		if _, e := os.Stat(filepath.Join(root, "sourceaec.sqlite3")); e == nil {
			return errors.New("legacy SQLite database found: stop services and run migrate-sqlite into a fresh PostgreSQL namespace first")
		} else if !os.IsNotExist(e) {
			return e
		}
	}
	names := []string{}
	if imported == 0 {
		names = []string{"models", "uploads", "projects", "plans", "chat", "skill-work", "chat-sessions.json"}
	}
	for _, name := range names {
		err = filepath.WalkDir(filepath.Join(root, name), func(path string, d fs.DirEntry, walkErr error) error {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			if walkErr != nil {
				return walkErr
			}
			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if d.IsDir() {
				if strings.HasSuffix(d.Name(), "_cache") || d.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			if !durable(rel) {
				return nil
			}
			data, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			_, e = tx.Exec("INSERT INTO artifacts(path,content) VALUES($1,$2) ON CONFLICT DO NOTHING", filepath.ToSlash(rel), data)
			return e
		})
		if err != nil {
			return err
		}
	}
	if _, err = tx.Exec("INSERT INTO migrations(name) VALUES('files-v1') ON CONFLICT DO NOTHING"); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		db.Close()
		return err
	}
	if err = restoreWorkspace(context.Background(), root, db); err != nil {
		return err
	}
	roots.Store(root, db)
	registered = true
	return nil
}

func inside(root, path string) bool {
	rel, e := filepath.Rel(root, path)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
func durable(key string) bool {
	key = filepath.ToSlash(key)
	if strings.HasSuffix(key, ".tmp") {
		return false
	}
	for _, part := range strings.Split(key, "/") {
		if part == ".git" || part == "versions" || strings.HasSuffix(part, "_cache") {
			return false
		}
	}
	base := filepath.Base(key)
	return base != "model.xkt" && base != "metadata.json" && base != "render.json"
}

func lookup(path string) (*sql.DB, string) {
	abs, e := filepath.Abs(path)
	if e != nil {
		return nil, ""
	}
	var db *sql.DB
	var key string
	roots.Range(func(k, v any) bool {
		root := k.(string)
		if inside(root, abs) {
			rel, _ := filepath.Rel(root, abs)
			if durable(rel) {
				db = v.(*sql.DB)
				key = filepath.ToSlash(rel)
			}
			return false
		}
		return true
	})
	return db, key
}

func ReadFile(path string) ([]byte, error) {
	if db, key := lookup(path); db != nil {
		var data []byte
		e := db.QueryRow("SELECT content FROM artifacts WHERE path=$1", key).Scan(&data)
		if e == nil {
			return data, nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return nil, e
		}
	}
	return os.ReadFile(path)
}
func WriteFile(path string, data []byte, mode fs.FileMode) error {
	if db, key := lookup(path); db != nil {
		if _, e := db.Exec("INSERT INTO artifacts(path,content,mode) VALUES($1,$2,$3) ON CONFLICT(path) DO UPDATE SET content=excluded.content,mode=excluded.mode", key, data, uint32(mode)); e != nil {
			return e
		}
	}
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		return e
	}
	return os.WriteFile(path, data, mode)
}
func Rename(old, new string) error {
	if db, key := lookup(new); db != nil {
		data, e := os.ReadFile(old)
		if e != nil {
			return e
		}
		if _, e = db.Exec("INSERT INTO artifacts(path,content) VALUES($1,$2) ON CONFLICT(path) DO UPDATE SET content=excluded.content", key, data); e != nil {
			return e
		}
	}
	return os.Rename(old, new)
}
func Remove(path string) error {
	if db, key := lookup(path); db != nil {
		if _, e := db.Exec("DELETE FROM artifacts WHERE path=$1", key); e != nil {
			return e
		}
	}
	return os.Remove(path)
}
func RemoveAll(path string) error {
	if db, key := lookup(path); db != nil {
		if _, e := db.Exec("DELETE FROM artifacts WHERE path=$1 OR substr(path,1,$2)=$3", key, len(key)+1, key+"/"); e != nil {
			return e
		}
		parts := strings.Split(key, "/")
		if len(parts) == 2 && parts[0] == "models" {
			if _, err := db.Exec("DELETE FROM repositories WHERE model_id=$1", parts[1]); err != nil {
				return err
			}
		}
	}
	return os.RemoveAll(path)
}

// Capture publishes a file written by a streaming producer.
func Capture(path string) error {
	db, key := lookup(path)
	if db == nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = db.Exec("INSERT INTO artifacts(path,content) VALUES($1,$2) ON CONFLICT(path) DO UPDATE SET content=excluded.content WHERE artifacts.content!=excluded.content", key, data)
	return err
}

// CaptureTree persists CLI-produced inputs/deliverables. Cache directories,
// symlinks and temporary files are excluded by the shared storage policy.
func CaptureTree(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			if strings.HasSuffix(d.Name(), "_cache") || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if db, _ := lookup(path); db == nil {
			return nil
		}
		return Capture(path)
	})
}
