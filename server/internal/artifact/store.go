// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// Package artifact persists business files in SQLite. Paths are disposable
// projections for Git, sandbox runners and converters, never database keys
// containing machine-specific absolute paths.
package artifact

import (
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

var roots sync.Map // absolute data directory -> *sql.DB

// Open imports legacy files once and restores missing workspace projections.
// The gateway and Python services share the same schema and WAL database.
func Open(root string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if _, ok := roots.Load(root); ok {
		return nil
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return err
	}
	db, err := sql.Open("sqlite3", filepath.Join(root, "sourceaec.sqlite3")+"?_busy_timeout=30000&_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS artifacts (path TEXT PRIMARY KEY, content BLOB NOT NULL, mode INTEGER NOT NULL DEFAULT 420);`); err != nil {
		db.Close()
		return err
	}
	actual, loaded := roots.LoadOrStore(root, db)
	if loaded {
		db.Close()
		db = actual.(*sql.DB)
	}
	if _, err = db.Exec("CREATE TABLE IF NOT EXISTS migrations (name TEXT PRIMARY KEY)"); err != nil {
		return err
	}
	var imported int
	err = db.QueryRow("SELECT count(*) FROM migrations WHERE name='files-v1'").Scan(&imported)
	if err != nil {
		return err
	}
	names := []string{}
	if imported == 0 {
		names = []string{"models", "uploads", "projects", "plans", "chat", "skill-work"}
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
			_, e = db.Exec("INSERT OR IGNORE INTO artifacts(path,content) VALUES(?,?)", filepath.ToSlash(rel), data)
			return e
		})
		if err != nil {
			return err
		}
	}
	if _, err = db.Exec("INSERT OR IGNORE INTO migrations(name) VALUES('files-v1')"); err != nil {
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
		path := filepath.Join(root, filepath.FromSlash(key))
		if !inside(root, path) {
			return errors.New("invalid artifact path")
		}
		if old, e := os.ReadFile(path); e == nil && string(old) == string(data) {
			continue
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

func inside(root, path string) bool {
	rel, e := filepath.Rel(root, path)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
func durable(key string) bool {
	key = filepath.ToSlash(key)
	if strings.HasSuffix(key, ".tmp") || strings.Contains(key, "_cache/") || strings.Contains(key, "/.git/") {
		return false
	}
	base := filepath.Base(key)
	return base != "model.xkt" && base != "metadata.json" && base != "render.json" && !strings.Contains(key, "/versions/")
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
		e := db.QueryRow("SELECT content FROM artifacts WHERE path=?", key).Scan(&data)
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
		if _, e := db.Exec("INSERT INTO artifacts(path,content,mode) VALUES(?,?,?) ON CONFLICT(path) DO UPDATE SET content=excluded.content,mode=excluded.mode", key, data, uint32(mode)); e != nil {
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
		if _, e = db.Exec("INSERT INTO artifacts(path,content) VALUES(?,?) ON CONFLICT(path) DO UPDATE SET content=excluded.content", key, data); e != nil {
			return e
		}
	}
	return os.Rename(old, new)
}
func Remove(path string) error {
	if db, key := lookup(path); db != nil {
		if _, e := db.Exec("DELETE FROM artifacts WHERE path=?", key); e != nil {
			return e
		}
	}
	return os.Remove(path)
}
func RemoveAll(path string) error {
	if db, key := lookup(path); db != nil {
		if _, e := db.Exec("DELETE FROM artifacts WHERE path=? OR substr(path,1,?)=?", key, len(key)+1, key+"/"); e != nil {
			return e
		}
	}
	return os.RemoveAll(path)
}

// Capture publishes a file written by a streaming producer.
func Capture(path string) error {
	data, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	return WriteFile(path, data, 0644)
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
